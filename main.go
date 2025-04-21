package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Shared HTTP client
var client = &http.Client{Timeout: 10 * time.Second}

// Fetches the total number of pages
func getMaxNumPages(url string) int {
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatalf("Failed to parse HTML: %v", err)
	}

	var pageCount int
	doc.Find(".discussion-list-page-indicator strong").Each(func(i int, s *goquery.Selection) {
		if i == 1 {
			text := strings.TrimSpace(s.Text())
			pageCount, _ = strconv.Atoi(text)
		}
	})

	if pageCount == 0 {
		pageCount = 1
	}

	return pageCount
}

// Extracts matching links from a single page
func getLinksFromPage(url string, grepStr string) []string {
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Failed to fetch URL %s: %v", url, err)
		return nil
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("Failed to parse HTML for %s: %v", url, err)
		return nil
	}

	var matchingLinks []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && strings.Contains(strings.ToLower(href), "/discussions") && strings.Contains(strings.ToLower(href), strings.ToLower(grepStr)) {
			matchingLinks = append(matchingLinks, href)
		}
	})

	return matchingLinks
}

// Main concurrent page scraping logic
func getAllPages(providerName string, grepStr string) []string {
	baseURL := fmt.Sprintf("https://www.examtopics.com/discussions/%s/", providerName)
	numPages := getMaxNumPages(baseURL)

	var wg sync.WaitGroup
	concurrencyLimit := make(chan struct{}, 20) // limit to 20 concurrent requests
	results := make(chan []string, numPages)

	for i := 1; i <= numPages; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			concurrencyLimit <- struct{}{} // acquire slot
			defer func() { <-concurrencyLimit }()

			url := fmt.Sprintf("https://www.examtopics.com/discussions/%s/%d", providerName, i)
			links := getLinksFromPage(url, grepStr)
			results <- links
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	uniqueLinks := make(map[string]struct{})
	for res := range results {
		for _, link := range res {
			uniqueLinks[link] = struct{}{}
		}
	}

	var resultLinks []string
	for link := range uniqueLinks {
		resultLinks = append(resultLinks, link)
	}

	sort.Slice(resultLinks, func(i, j int) bool {
		extractQuestionNum := func(url string) int {
			parts := strings.Split(url, "question-")
			if len(parts) < 2 {
				return 0
			}
			numStr := strings.TrimSuffix(parts[1], "/") // remove trailing slash
			num, _ := strconv.Atoi(numStr)
			return num
		}
		return extractQuestionNum(resultLinks[i]) < extractQuestionNum(resultLinks[j])
	})

	fmt.Printf("Found %d unique matching links:\n", len(uniqueLinks))

	return resultLinks
}

func cleanText(raw string) string {
	// Remove excessive whitespace (newlines, tabs, etc.)
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, "\n", " ")
	raw = strings.ReplaceAll(raw, "\t", " ")

	re := regexp.MustCompile(`\s+`)
	cleaned := re.ReplaceAllString(raw, " ")
	cleaned = strings.TrimSpace(cleaned)

	// Add newline before "Suggested Answer"
	cleaned = strings.Replace(cleaned, "Suggested Answer", "\nSuggested Answer", 1)
	cleaned = strings.ReplaceAll(cleaned, "Forgot my password", "")

	return cleaned
}

func getProviderExams(providerName string) []string {
	baseURL := fmt.Sprintf("https://www.examtopics.com/exams/%s/", providerName)
	resp, err := client.Get(baseURL)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatalf("Failed to parse HTML: %v", err)
	}

	var allExams []string
	doc.Find(".popular-exam-link").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			allExams = append(allExams, cleanText(href))
		}
	})

	return allExams
}

func getDataFromLink(link string) map[string]any {
	resp, err := client.Get(link)
	if err != nil {
		log.Fatalf("Failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		log.Printf("503 error for link: %s", link)
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatalf("Failed to parse HTML: %v", err)
	}

	var allQuestions []string
	doc.Find("li.multi-choice-item").Each(func(i int, s *goquery.Selection) {
		allQuestions = append(allQuestions, cleanText(s.Text()))
	})

	data := map[string]any{
		"title":     cleanText(doc.Find("h1").Text()),
		"header":    strings.ReplaceAll(strings.TrimSpace(doc.Find(".question-discussion-header").Text()), "\t", ""),
		"content":   cleanText(doc.Find(".card-text").Text()),
		"questions": allQuestions,
		"answer": func() string {
			text := doc.Find(".correct-answer").Text()
			text = strings.TrimSpace(text)
			text = strings.ReplaceAll(text, " ", "")
			text = strings.ReplaceAll(text, "\n", "")
			if len(text) > 0 {
				return string(text[0])
			}
			return ""
		}(),
		"timestamp":    cleanText(doc.Find(".discussion-meta-data > i").Text()),
		"questionLink": link,
		"comments":     cleanText(doc.Find(".discussion-container").Text()),
	}

	return data
}

func startSpinner(done <-chan struct{}) {
	chars := `-\|/`
	i := 0
	for {
		select {
		case <-done:
			fmt.Print("\r\n")
			return
		default:
			fmt.Printf("\rScraping... %c ", chars[i%len(chars)])
			time.Sleep(100 * time.Millisecond)
			i++
		}
	}
}

func writeData(links []string, outputPath string, commentBool bool) {
	const MaxConcurrentRequests = 15
	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrentRequests)

	results := make([]map[string]any, len(links))

	fmt.Println("Started writing to files, please wait...")
	for i, link := range links {
		wg.Add(1)
		url := fmt.Sprintf("https://www.examtopics.com%s", link)

		go func(i int, link, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data := getDataFromLink(url)
			if data == nil {
				return
			}
			results[i] = data
		}(i, link, url)
	}

	wg.Wait()

	// Must write in order
	file, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	fmt.Fprintf(file, "# Exam Topics Questions\n\n")
	fmt.Fprintf(file, "@thatonecodes\n\n")

	for _, data := range results {
		if data == nil {
			continue
		}
		fmt.Fprintf(file, "## %s\n\n", data["title"])
		fmt.Fprintf(file, "%s\n\n", data["header"])
		fmt.Fprintf(file, "%s\n\n", data["content"])
		if questions, ok := data["questions"].([]string); ok {
			for _, question := range questions {
				fmt.Fprintf(file, "%v\n\n", question)
			}
		}
		fmt.Fprintf(file, "**Answer: %s**\n\n", data["answer"])
		fmt.Fprintf(file, "**Timestamp: %s**\n\n", data["timestamp"])
		fmt.Fprintf(file, "[View on ExamTopics](%s)\n\n", data["questionLink"])
		if commentBool {
			fmt.Fprintf(file, "Comments: %s\n", data["comments"])
		}
		fmt.Fprintf(file, "----------------------------------------\n\n")
	}
}

func saveLinks(links []string) {
	file, err := os.Create("saved-links.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	for _, link := range links {
		fmt.Fprintf(file, "https://www.examtopics.com%s\n", link)
	}
}

func main() {
	provider := flag.String("p", "google", "Name of the exam provider (default -> google)")
	grepStr := flag.String("s", "", "String to grep for in discussion links (required)")
	outputPath := flag.String("o", "examtopics_output.md", "Optional path of the file where the data will be outputted")
	commentBool := flag.Bool("c", false, "Optionally include all the comment/discussion text")
	examsFlag := flag.Bool("exams", false, "Optionally show all the possible exams for your selected provider and exit")
	saveUrls := flag.Bool("save-links", false, "Optional argument to save unique links to questions")
	flag.Parse()

	if *examsFlag {
		exams := getProviderExams(*provider)
		fmt.Printf("Exams for provider '%s'\n\n", *provider)
		for _, exam := range exams {
			fmt.Printf("https://www.examtopics.com%s\n", exam)
		}
		os.Exit(0)
	}

	done := make(chan struct{})
	go startSpinner(done)

	if *grepStr == "" {
		log.Println("running without a valid string to search for with -s, (no_grep_str)!")
	}

	links := getAllPages(*provider, *grepStr)
	close(done)

	if *saveUrls {
		saveLinks(links)
	}

	writeData(links, *outputPath, *commentBool)

	fmt.Printf("Successfully saved output to %s.\n", *outputPath)
}
