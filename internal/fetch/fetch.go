package fetch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"examtopics-downloader/internal/constants"
	"examtopics-downloader/internal/models"
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

func FetchCachedLinks(providerName string, grepStr string, token string) []string {
	baseURL := fmt.Sprintf("https://api.github.com/repos/thatonecodes/examtopics-data/contents/%s", utils.CapitalizeFirstLetter(providerName))
	if token != "" {
		client = utils.NewGitHubClient(token)
	}
	resp := FetchURL(baseURL, *client)

	var content []models.FileInfo

	if resp == nil {
		log.Printf("the response body was nil, %v", resp)
		return nil
	}

	err := json.Unmarshal(resp, &content)
	if err != nil {
		log.Fatalf("error unmarshaling response: %v", err)
	}

	var linksWithNumbers []models.FileInfo
	for _, item := range content {
		link := item.URL
		number := utils.ExtractNumberFromPath(item.Name)
		if utils.GrepString(link, grepStr) {
			linksWithNumbers = append(linksWithNumbers, models.FileInfo{
				URL:    link,
				Name:   item.Name,
				Number: number,
			})
		}
	}

	return utils.SortCachedLinks(linksWithNumbers)
}

func GetCachedPages(providerName string, grepStr string, token string) []models.QuestionData {
	links := FetchCachedLinks(providerName, grepStr, token)
	var allData []models.QuestionData

	var wg sync.WaitGroup
	dataChan := make(chan models.QuestionData)

	for _, link := range links {
		wg.Add(1)
		go func(link string) {
			defer wg.Done()
			dataList := getJSONFromLink(link)
			if dataList == nil {
				return
			}
			for _, data := range dataList {
				dataChan <- *data // send each QuestionData into the channel
			}
		}(link)
	}

	go func() {
		wg.Wait()
		close(dataChan)
	}()

	for data := range dataChan {
		allData = append(allData, data)
	}

	return allData
}
