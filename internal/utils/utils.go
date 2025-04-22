package utils

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
		num, _ := strconv.Atoi(numStr)
		return num
	}

	sort.Slice(links, func(i, j int) bool {
		return extractQuestionNum(links[i]) < extractQuestionNum(links[j])
	})
	return links
}

func GrepString(baseString, searchString string) bool {
	return strings.Contains(
		strings.ToLower(baseString),
		strings.ToLower(searchString),
	)
}

func AddToBaseUrl(addString string) string {
	return fmt.Sprintf("https://www.examtopics.com%s", addString)
}
