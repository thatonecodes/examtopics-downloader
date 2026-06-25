package fetch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"examtopics-downloader/internal/constants"
	"examtopics-downloader/internal/utils"

	"github.com/PuerkitoBio/goquery"
)

var client = utils.NewHTTPClient()

// requestLimiter paces the concurrent fan-out phases and adapts to the server:
// it speeds up while responses are healthy and brakes hard on throttling. Wired
// for feedback inside FetchURL (the single place that sees status codes).
var requestLimiter = utils.NewAdaptiveLimiter(
	constants.StartRequestsPerSecond,
	constants.MinRequestsPerSecond,
	constants.MaxRequestsPerSecond,
	constants.RateIncreaseStep,
	constants.SuccessStreakForSpeedup,
)

var (
	providerHrefPattern           = regexp.MustCompile(`(?i)^/exams/([a-z0-9-]+)/?$`)
	discussionProviderHrefPattern = regexp.MustCompile(`(?i)^/discussions/([a-z0-9-]+)/?$`)
	discussionViewLinkPattern     = regexp.MustCompile(`(?i)^/discussions/[a-z0-9-]+/view/`)
	examFromDiscussionURLPattern  = regexp.MustCompile(`(?i)-exam-([a-z0-9-]+?)(?:-topic-|-question-|/|$)`)
	digitsPattern                 = regexp.MustCompile(`\D+`)
	oracleVersionedPattern        = regexp.MustCompile(`(?i)^(1z\d-\d{3,4})-\d{1,2}$`)
	oracleBaseCodePattern         = regexp.MustCompile(`(?i)^1z\d-\d{3,4}$`)
	trailingVersionTokenPattern   = regexp.MustCompile(`(?i)^(?:\d{2}|\d{4}|v\d+|ver\d+|rev\d+)$`)
	urlInTextPattern              = regexp.MustCompile(`https?://[^\s"'<>\]]+`)
)

// retryableStatuses are HTTP status codes that justify a retry. Anti-bot
// rate-limiting and gateway hiccups are transient and almost always recover
// on a second pass; they used to be silently dropped.
var retryableStatuses = map[int]struct{}{
	http.StatusTooManyRequests:     {}, // 429
	http.StatusBadGateway:          {}, // 502
	http.StatusServiceUnavailable:  {}, // 503
	http.StatusGatewayTimeout:      {}, // 504
}

func FetchURL(url string, client http.Client) []byte {
	backoff := constants.InitalBackoff
	var lastStatus int

	for attempt := 0; attempt <= constants.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := utils.DelayTime(backoff)
			debugf("Retry attempt %d for URL: %s after waiting %v", attempt, url, delay)
			utils.Sleep(delay)
			backoff = utils.BackoffTime(backoff, constants.BackoffFactor)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			debugf("failed to create request for URL %s: %v", url, err)
			continue
		}
		// Reduce anti-bot 403s by mimicking a normal browser request.
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", "https://www.examtopics.com/")

		resp, err := client.Do(req)
		if err != nil {
			debugf("failed to fetch URL (attempt %d): %v", attempt, err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				debugf("failed to read response body: %v", err)
				return nil
			}
			requestLimiter.OnSuccess()
			return body
		}

		lastStatus = resp.StatusCode
		if _, retryable := retryableStatuses[resp.StatusCode]; retryable {
			// Server is pushing back — slow the whole fan-out, not just this call.
			requestLimiter.OnThrottle()
			// Honour Retry-After when the server provides one (common on 429).
			if delay := parseRetryAfter(resp.Header.Get("Retry-After")); delay > 0 {
				resp.Body.Close()
				debugf("status %d; honouring Retry-After=%v for %s", resp.StatusCode, delay, url)
				utils.Sleep(delay)
				continue
			}
			resp.Body.Close()
			continue
		}

		resp.Body.Close()
		debugf("request failed with non-retryable status code: %d for %s", resp.StatusCode, url)
		fmt.Fprintf(os.Stderr, "[WARN] HTTP %d for %s (not retried)\n", resp.StatusCode, url)
		return nil
	}

	if lastStatus != 0 {
		fmt.Fprintf(os.Stderr, "[WARN] Gave up after %d retries (last status %d) for %s\n", constants.MaxRetries, lastStatus, url)
	} else {
		fmt.Fprintf(os.Stderr, "[WARN] Gave up after %d retries (transport errors) for %s\n", constants.MaxRetries, url)
	}
	debugf("exhausted retries for URL: %s", url)
	return nil
}

// parseRetryAfter accepts either a delay in seconds or an HTTP date and
// returns the duration to wait. Returns 0 when unparseable.
func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0
		}
		if secs > 60 {
			secs = 60 // cap to keep the loop responsive
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		if d > 60*time.Second {
			d = 60 * time.Second
		}
		return d
	}
	return 0
}

func ParseHTML(url string, client http.Client) (*goquery.Document, error) {
	body := FetchURL(url, client)
	if body == nil {
		return nil, fmt.Errorf("empty response body from URL %q", url)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML from URL %q: %w", url, err)
	}

	return doc, nil
}

// ParseHTMLCached behaves like ParseHTML but serves and stores the raw response
// body in the on-disk question-page cache. Re-running the same exam then skips
// the network (and rate limiting) for unchanged question pages. A corrupt or
// unparseable cache entry transparently falls back to a fresh fetch.
func ParseHTMLCached(url string, client http.Client) (*goquery.Document, error) {
	if body, ok := readCachedPage(url); ok {
		if doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body)); err == nil {
			debugf("page-cache: hit for %s", url)
			return doc, nil
		} else {
			debugf("page-cache: corrupt entry for %s, refetching: %v", url, err)
		}
	}

	body := FetchURL(url, client)
	if body == nil {
		return nil, fmt.Errorf("empty response body from URL %q", url)
	}
	writeCachedPage(url, body)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML from URL %q: %w", url, err)
	}
	return doc, nil
}

// Fetches total number of pages
func getMaxNumPages(url string) int {
	doc, err := ParseHTML(url, *client)
	if err != nil {
		debugf("failed parsing HTML for number of pages: %v", err)
		return 1
	}

	var pageCount int
	doc.Find(".discussion-list-page-indicator strong").Each(func(i int, s *goquery.Selection) {
		if i == 1 {
			pageCount, _ = strconv.Atoi(strings.TrimSpace(s.Text()))
		}
	})

	// Handle the null case
	if pageCount == 0 {
		pageCount = 1
	}

	return pageCount
}

func GetAllProviders() []string {
	seen := map[string]struct{}{}
	providers := make([]string, 0, 64)

	for _, provider := range getProvidersFromExams() {
		if _, exists := seen[provider]; exists {
			continue
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}

	for _, provider := range getProvidersFromDiscussions() {
		if _, exists := seen[provider]; exists {
			continue
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}

	sort.Strings(providers)
	return providers
}

func getProvidersFromExams() []string {
	doc, err := ParseHTML("https://www.examtopics.com/exams/", *client)
	if err != nil {
		debugf("failed to parse HTML for providers from exams: %v", err)
		return nil
	}
	return extractProvidersFromExamsDoc(doc)
}

func getProvidersFromDiscussions() []string {
	const (
		maxAttempts                = 3
		minLikelyGoodProviderCount = 150
	)

	var best []string
	expectedCategories := 0

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		doc, err := ParseHTML("https://www.examtopics.com/discussions/", *client)
		if err != nil {
			debugf("failed to parse HTML for providers from discussions (attempt %d/%d): %v", attempt, maxAttempts, err)
			continue
		}

		if catCount := extractDiscussionCategoryCount(doc); catCount > expectedCategories {
			expectedCategories = catCount
		}

		current := extractProvidersFromDiscussionsDoc(doc)
		if len(current) > len(best) {
			best = current
		}

		if expectedCategories > 0 {
			// discussions page includes categories with zero discussions as well;
			// require ~80% coverage to consider result stable enough.
			minTarget := int(float64(expectedCategories) * 0.8)
			if len(best) >= minTarget {
				return best
			}
		} else if len(best) >= minLikelyGoodProviderCount {
			return best
		}

		if attempt < maxAttempts {
			time.Sleep(600 * time.Millisecond)
		}
	}

	return best
}

func extractProvidersFromExamsDoc(doc *goquery.Document) []string {
	seen := map[string]struct{}{}
	providers := make([]string, 0, 32)

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		href = strings.TrimSpace(strings.ToLower(href))
		matches := providerHrefPattern.FindStringSubmatch(href)
		if len(matches) != 2 {
			return
		}

		provider := strings.TrimSpace(matches[1])
		if provider == "" {
			return
		}
		if _, exists := seen[provider]; exists {
			return
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	})

	sort.Strings(providers)
	return providers
}

func extractProvidersFromDiscussionsDoc(doc *goquery.Document) []string {
	seen := map[string]struct{}{}
	providers := make([]string, 0, 32)

	addProvider := func(provider string) {
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider == "" {
			return
		}
		if _, exists := seen[provider]; exists {
			return
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}

	// Primary strategy: parse provider rows and respect "discussions > 0".
	doc.Find(".discussion-row").Each(func(i int, row *goquery.Selection) {
		provider := ""
		row.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			href, exists := a.Attr("href")
			if !exists {
				return true
			}

			href = strings.TrimSpace(strings.ToLower(href))
			matches := discussionProviderHrefPattern.FindStringSubmatch(href)
			if len(matches) != 2 {
				return true
			}

			provider = strings.TrimSpace(matches[1])
			return false
		})

		if provider == "" {
			return
		}

		// Prefer rows with discussions > 0; if count is missing/unparseable, keep provider.
		if countNode := row.Find(".discussion-stats-replies").First(); countNode.Length() > 0 {
			countText := countNode.Text()
			count := parseDiscussionCount(countText)
			if count == 0 && strings.TrimSpace(countText) != "" {
				return
			}
		}

		addProvider(provider)
	})

	// Fallback: if row parsing yields nothing (markup drift / anti-bot HTML),
	// collect providers from plain provider links.
	if len(providers) == 0 {
		doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists {
				return
			}

			href = strings.TrimSpace(strings.ToLower(href))
			matches := discussionProviderHrefPattern.FindStringSubmatch(href)
			if len(matches) != 2 {
				return
			}

			addProvider(matches[1])
		})
	}

	sort.Strings(providers)
	return providers
}

func parseDiscussionCount(raw string) int {
	clean := digitsPattern.ReplaceAllString(raw, "")
	if clean == "" {
		return 0
	}

	count, err := strconv.Atoi(clean)
	if err != nil {
		return 0
	}
	return count
}

func extractDiscussionCategoryCount(doc *goquery.Document) int {
	if doc == nil {
		return 0
	}

	count := 0
	doc.Find(".discussion-list-page-indicator").First().Find("span").EachWithBreak(func(i int, s *goquery.Selection) bool {
		n := parseDiscussionCount(s.Text())
		if n <= 0 {
			return true
		}
		count = n
		return false
	})

	return count
}

func GetProviderExams(providerName string) []string {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	baseURL := fmt.Sprintf("https://www.examtopics.com/exams/%s/", providerName)
	doc, err := ParseHTML(baseURL, *client)
	if err != nil {
		debugf("failed to parse HTML for provider exams: %v", err)
		return nil
	}

	examHrefPattern := regexp.MustCompile(fmt.Sprintf(`(?i)^/exams/%s/([a-z0-9-]+)/?$`, regexp.QuoteMeta(providerName)))
	seen := map[string]struct{}{}
	allExams := make([]string, 0, 32)

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		cleanHref := strings.TrimSpace(strings.ToLower(href))
		matches := examHrefPattern.FindStringSubmatch(cleanHref)
		if len(matches) != 2 {
			return
		}

		examSlug := strings.TrimSpace(matches[1])
		if examSlug == "" {
			return
		}

		normalized := fmt.Sprintf("/exams/%s/%s/", providerName, examSlug)
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		allExams = append(allExams, normalized)
	})

	sort.Strings(allExams)
	return allExams
}

func GetProviderExamSlugs(providerName string, includeDiscussionExams bool) []string {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	if providerName == "" {
		return nil
	}

	officialExamLinks := GetProviderExams(providerName)
	var inferredFromDiscussions []string
	if includeDiscussionExams {
		// Smart fallback strategy for providers missing /exams/ coverage:
		// infer distinct exam slugs from provider discussion links.
		inferredFromDiscussions = inferExamSlugsFromDiscussionPages(providerName)
	}

	return buildProviderExamSlugs(providerName, officialExamLinks, inferredFromDiscussions, includeDiscussionExams)
}

func buildProviderExamSlugs(providerName string, officialExamLinks, inferredFromDiscussions []string, includeDiscussionExams bool) []string {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	if providerName == "" {
		return nil
	}

	seen := map[string]struct{}{}
	examSlugs := make([]string, 0, 32)
	add := func(raw string) {
		normalized := normalizeExamSlug(providerName, raw)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		examSlugs = append(examSlugs, normalized)
	}

	officialExamSlugs := extractExamSlugsFromExamLinks(providerName, officialExamLinks)
	for _, exam := range officialExamSlugs {
		add(exam)
	}

	if includeDiscussionExams {
		for _, exam := range inferredFromDiscussions {
			add(exam)
		}
	}

	sort.Strings(examSlugs)
	if len(examSlugs) == 0 {
		if includeDiscussionExams {
			// last-resort fallback to still ingest provider content
			return []string{"all-discussions"}
		}
		return nil
	}

	return examSlugs
}

func extractExamSlugsFromExamLinks(providerName string, examLinks []string) []string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?i)^/exams/%s/([a-z0-9-]+)/?$`, regexp.QuoteMeta(strings.ToLower(strings.TrimSpace(providerName)))))
	seen := map[string]struct{}{}
	out := make([]string, 0, len(examLinks))

	for _, link := range examLinks {
		matches := pattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(link)))
		if len(matches) != 2 {
			continue
		}
		examSlug := strings.TrimSpace(matches[1])
		if examSlug == "" {
			continue
		}
		if _, exists := seen[examSlug]; exists {
			continue
		}
		seen[examSlug] = struct{}{}
		out = append(out, examSlug)
	}

	sort.Strings(out)
	return out
}

func inferExamSlugsFromDiscussionPages(providerName string) []string {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	if providerName == "" {
		return nil
	}

	if cached, ok := getCachedDiscussionExamSlugs(providerName); ok {
		debugf("using cached discussion-derived exams for provider %q (%d)", providerName, len(cached))
		return cached
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 32)
	baseURL := fmt.Sprintf("https://www.examtopics.com/discussions/%s/", providerName)
	numPages := getMaxNumPages(baseURL)

	for pageNum := 1; pageNum <= numPages; pageNum++ {
		pageURL := fmt.Sprintf("https://www.examtopics.com/discussions/%s/%d", providerName, pageNum)
		discussionLinks := getDiscussionLinksFromPage(pageURL)
		for _, link := range discussionLinks {
			examSlug := extractExamSlugFromDiscussionURL(link)
			if examSlug == "" {
				continue
			}
			if _, exists := seen[examSlug]; exists {
				continue
			}
			seen[examSlug] = struct{}{}
			out = append(out, examSlug)
		}
	}

	sort.Strings(out)
	setCachedDiscussionExamSlugs(providerName, out)
	return out
}

func normalizeExamSlug(providerName, examSlug string) string {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	examSlug = strings.TrimSpace(strings.ToLower(examSlug))
	if examSlug == "" {
		return ""
	}

	// Oracle version collapsing: 1z0-1042-20 -> 1z0-1042
	if providerName == "oracle" {
		if m := oracleVersionedPattern.FindStringSubmatch(examSlug); len(m) == 2 {
			return strings.TrimSpace(m[1])
		}
	}

	// Generic version collapsing for common vendor variants:
	// <base>-v2, <base>-2024, <base>-23, <base>-rev3
	parts := strings.Split(examSlug, "-")
	if len(parts) >= 3 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if trailingVersionTokenPattern.MatchString(last) {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}

	return examSlug
}

func extractExamSlugFromDiscussionURL(link string) string {
	link = strings.TrimSpace(strings.ToLower(link))
	if link == "" {
		return ""
	}

	matches := examFromDiscussionURLPattern.FindStringSubmatch(link)
	if len(matches) != 2 {
		return ""
	}

	slug := strings.Trim(matches[1], "- ")
	if slug == "" {
		return ""
	}
	return slug
}

func getDiscussionLinksFromPage(url string) []string {
	links, _ := getDiscussionLinksFromPageWithStatus(url)
	return links
}

// getDiscussionLinksFromPageWithStatus is getDiscussionLinksFromPage but also
// reports whether the page was fetched successfully. The ok flag lets callers
// distinguish "fetched, no links here" from "fetch failed" so transient
// failures can be retried instead of silently dropping ~10 questions per page.
func getDiscussionLinksFromPageWithStatus(url string) (links []string, ok bool) {
	doc, err := ParseHTML(url, *client)
	if err != nil {
		debugf("failed to parse HTML for %s: %v", url, err)
		return nil, false
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 64)
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		clean := normalizeDiscussionViewHref(href)
		if clean == "" {
			return
		}
		if _, exists := seen[clean]; exists {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	})

	return out, true
}

func normalizeDiscussionViewHref(rawHref string) string {
	rawHref = strings.TrimSpace(strings.ToLower(rawHref))
	if rawHref == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(rawHref, "https://www.examtopics.com/"):
		rawHref = strings.TrimPrefix(rawHref, "https://www.examtopics.com")
	case strings.HasPrefix(rawHref, "http://www.examtopics.com/"):
		rawHref = strings.TrimPrefix(rawHref, "http://www.examtopics.com")
	case strings.HasPrefix(rawHref, "https://") || strings.HasPrefix(rawHref, "http://"):
		parsed, err := url.Parse(rawHref)
		if err != nil {
			return ""
		}
		host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
		if host != "www.examtopics.com" && host != "examtopics.com" {
			return ""
		}
		rawHref = parsed.EscapedPath()
	}

	if !strings.HasPrefix(rawHref, "/") {
		rawHref = "/" + rawHref
	}

	if cut := strings.IndexAny(rawHref, "?#"); cut >= 0 {
		rawHref = rawHref[:cut]
	}
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return ""
	}

	if !discussionViewLinkPattern.MatchString(rawHref) {
		return ""
	}

	return rawHref
}

func matchesExamSelection(providerName, selectedExam, link string) bool {
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	selectedExam = strings.TrimSpace(strings.ToLower(selectedExam))
	if selectedExam == "" {
		return true
	}
	selectedNormalized := normalizeExamSlug(providerName, selectedExam)

	link = strings.TrimSpace(strings.ToLower(link))
	if link == "" {
		return false
	}

	// Primary strategy: normalize the exam slug extracted from the discussion link
	// and compare with the normalized user selection.
	if linkExamSlug := extractExamSlugFromDiscussionURL(link); linkExamSlug != "" {
		return normalizeExamSlug(providerName, linkExamSlug) == selectedNormalized
	}

	// Oracle fallback: match variant-like URLs even when the "exam-..." segment
	// is missing or formatted unusually in discussion links.
	if providerName == "oracle" && oracleBaseCodePattern.MatchString(selectedNormalized) {
		variantPattern := regexp.MustCompile(`(?i)(?:^|[-/])` + regexp.QuoteMeta(selectedNormalized) + `-\d{1,2}(?:[-/]|$)`)
		if variantPattern.MatchString(link) {
			return true
		}
	}

	// Fallback for unusual URL formats where exam slug extraction fails.
	return utils.GrepString(link, selectedExam) || utils.GrepString(link, selectedNormalized)
}

// Extracts matching links from a single page.
func getLinksFromPage(providerName, url, selectedExam string) []string {
	links, _ := getLinksFromPageWithStatus(providerName, url, selectedExam)
	return links
}

// getLinksFromPageWithStatus is getLinksFromPage but also reports whether the
// underlying listing page was fetched successfully, so the fan-out can retry
// pages that failed rather than silently losing their questions.
func getLinksFromPageWithStatus(providerName, url, selectedExam string) (links []string, ok bool) {
	hrefs, ok := getDiscussionLinksFromPageWithStatus(url)
	if !ok {
		return nil, false
	}
	for _, href := range hrefs {
		if matchesExamSelection(providerName, selectedExam, href) {
			links = append(links, href)
		}
	}
	return links, true
}
