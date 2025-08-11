package utils

import (
	"examtopics-downloader/internal/models"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func CleanText(raw string) string {
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

type AutoCloseFile struct {
	*os.File
}

func (f *AutoCloseFile) Close() {
	if f.File != nil {
		f.File.Close()
		f.File = nil
	}
}

func CreateFile(filename string) *AutoCloseFile {
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}

	// Set up finalizer to ensure file is closed if Close() isn't called
	runtime.SetFinalizer(&AutoCloseFile{file}, (*AutoCloseFile).Close)

	return &AutoCloseFile{file}
}

func DeduplicateLinks(links []string) []string {
	seen := make(map[string]struct{})
	var unique []string
	for _, link := range links {
		if _, exists := seen[link]; !exists {
			seen[link] = struct{}{}
			unique = append(unique, link)
		}
	}
	return unique
}

func SortLinksByQuestionNumber(links []string) []string {
	extractQuestionNum := func(url string) int {
		parts := strings.Split(url, "question-")
		if len(parts) < 2 {
			return 0
		}
		numStr := strings.TrimSuffix(parts[1], "/")
		numStr = strings.TrimSuffix(numStr, "-discussion")
		num, _ := strconv.Atoi(numStr)
		return num
	}

	extractTopicNum := func(url string) int {
		parts := strings.Split(url, "topic-")
		if len(parts) < 2 {
			return 0
		}
		subParts := strings.Split(parts[1], "-")
		if len(subParts) < 1 {
			return 0
		}
		num, _ := strconv.Atoi(subParts[0])
		return num
	}

	sort.Slice(links, func(i, j int) bool {
		topicI := extractTopicNum(links[i])
		topicJ := extractTopicNum(links[j])

		if topicI != topicJ {
			return topicI < topicJ
		}
		return extractQuestionNum(links[i]) < extractQuestionNum(links[j])
	})
	return links
}

func normalize(s string) string {
	s = strings.ToLower(s)

	// Replace multiple dashes with a single dash
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")

	// Remove suffix like _xxx.json (if any)
	reSuffix := regexp.MustCompile(`(_\d+)?\.json$`)
	s = reSuffix.ReplaceAllString(s, "")

	return s
}

func GrepString(baseString, searchString string) bool {
	return strings.Contains(
		strings.ToLower(baseString),
		strings.ToLower(searchString),
	)
}

func GrepStringFromCache(baseString, searchString string) bool {
	baseNorm := normalize(baseString)
	searchNorm := normalize(searchString)

	return strings.Contains(
		baseNorm,
		searchNorm,
	)
}

func AddToBaseUrl(addString string) string {
	return fmt.Sprintf("https://www.examtopics.com%s", addString)
}

func CreateRateLimiter(rps float64) *time.Ticker {
	interval := time.Duration(float64(time.Second) / rps)
	return time.NewTicker(interval)
}

func DelayTime(backoff time.Duration) time.Duration {
	return backoff + time.Duration(rand.Intn(500))*time.Millisecond
}

func BackoffTime(backoff time.Duration, backoffFactor float64) time.Duration {
	return time.Duration(float64(backoff) * backoffFactor)
}

func Sleep(seconds time.Duration) {
	time.Sleep(seconds)
}

func SortCachedLinks(linksWithNumbers []models.FileInfo) []string {
	sort.Slice(linksWithNumbers, func(i, j int) bool {
		return linksWithNumbers[i].Number < linksWithNumbers[j].Number
	})

	// Collect sorted links
	var sortedLinks []string
	for _, linkWithNumber := range linksWithNumbers {
		sortedLinks = append(sortedLinks, linkWithNumber.URL)
	}
	return sortedLinks
}

func ExtractNumberFromPath(filename string) int {
	num := -1 // Default if no number found
	parts := strings.Split(filename, "_")
	if len(parts) > 1 {
		numStr := strings.Split(parts[1], ".")[0]
		parsedNum, err := strconv.Atoi(numStr)
		if err == nil {
			num = parsedNum
		}
	}
	return num
}

func FilterOutNilData(results []*models.QuestionData) []models.QuestionData {
	var finalData []models.QuestionData
	for _, entry := range results {
		if entry != nil {
			finalData = append(finalData, *entry)
		}
	}
	return finalData
}

func CapitalizeFirstLetter(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(string(s[0])) + s[1:]
}

func NewGitHubClient(token string) *http.Client {
	return &http.Client{
		Transport: &models.AuthTransport{
			Token:     token,
			Transport: http.DefaultTransport,
		},
	}
}

func GetNameFromLink(link string) string {
	name := strings.TrimSuffix(path.Base(link), ".json")
	name = strings.ReplaceAll(name, "-", " ")
	return strings.Join(strings.Fields(name), " ")
}

func SortQuestionDataByPageNumber(data []models.QuestionData) []models.QuestionData {
	sortedData := make([]models.QuestionData, len(data))
	copy(sortedData, data)

	sort.Slice(sortedData, func(i, j int) bool {
		pageNumI := ExtractNumberFromPath(sortedData[i].Title)
		pageNumJ := ExtractNumberFromPath(sortedData[j].Title)

		return pageNumI < pageNumJ
	})

	return sortedData
}

func StartTime() time.Time {
	return time.Now()
}

func TimeSince(startTime time.Time) string {
	duration := time.Since(startTime)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
