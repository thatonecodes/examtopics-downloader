package fetch

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"examtopics-downloader/internal/constants"
	"examtopics-downloader/internal/utils"
	"github.com/PuerkitoBio/goquery"
)

var client = &http.Client{Timeout: constants.HttpTimeout}

func FetchURL(url string, client http.Client) []byte {
	backoff := constants.InitalBackoff

	for attempt := 0; attempt <= constants.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := utils.DelayTime(backoff)
			log.Printf("Retry attempt %d for URL: %s after waiting %v", attempt, url, delay)
			utils.Sleep(delay)
			backoff = utils.BackoffTime(backoff, constants.BackoffFactor)
		}

		resp, err := client.Get(url)
		if err != nil {
			log.Printf("failed to fetch URL (attempt %d): %v", attempt, err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("failed to read response body: %v", err)
				return nil
			}
			return body
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			log.Printf("request failed with status code: %d", resp.StatusCode)
			return nil
		}
	}

	log.Printf("exhausted retries for URL: %s", url)
	return nil
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

// Fetches total number of pages
func getMaxNumPages(url string) int {
	doc, err := ParseHTML(url, *client)
	if err != nil {
		log.Panicf("Failed parsing HTML for number of pages: %v", err)
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

func GetProviderExams(providerName string) []string {
	baseURL := fmt.Sprintf("https://www.examtopics.com/exams/%s/", providerName)
	doc, err := ParseHTML(baseURL, *client)
	if err != nil {
		log.Fatalf("Failed to parse HTML for provider exams: %v", err)
	}

	var allExams []string
	doc.Find(".popular-exam-link").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			allExams = append(allExams, utils.CleanText(href))
		}
	})

	return allExams
}

// Extracts matching links from a single page
func getLinksFromPage(url string, grepStr string) []string {
	doc, err := ParseHTML(url, *client)
	if err != nil {
		log.Printf("Failed to parse HTML for %s: %v", url, err)
		return nil
	}

	var matchingLinks []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && utils.GrepString(href, "/discussions") && utils.GrepString(href, grepStr) {
			matchingLinks = append(matchingLinks, href)
		}
	})

	return matchingLinks
}
