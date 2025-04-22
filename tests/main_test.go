package tests

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"examtopics-downloader/internal/fetch"
	"examtopics-downloader/internal/models"
	"examtopics-downloader/internal/utils"
)

func TestGetAllPages(t *testing.T) {
	links := fetch.GetAllPages("lpi", "010-160")
	if len(links) == 0 {
		t.Fatalf("Expected non-empty data for provider 'lpi', but got: %v", links)
	}

	expectedType := reflect.TypeOf(models.QuestionData{})
	for _, link := range links {
		if reflect.TypeOf(link) != expectedType {
			t.Fatalf("Incorrect type for link, expected %v, got %v", expectedType, reflect.TypeOf(link))
		}
	}

	t.Logf("Data len of %d for provider 'lpi'", len(links))
}

func TestValidateExamsOutput(t *testing.T) {
	links := fetch.GetAllPages("lpi", "010-160")
	outputPath := "test.txt"

	utils.SaveLinks(outputPath, links)

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Expected file at %s but got error: %v", outputPath, err)
	}

	content := string(data)
	expectedContent := "https://www.examtopics.com/discussions/lpi/view"
	if !strings.Contains(content, expectedContent) {
		t.Errorf("Expected file content to contain %q but got:\n%s", expectedContent, content)
	}

	err = os.Remove(outputPath)
	if err != nil {
		t.Fatalf("Error when removing file! %v", err)
	}
}

func TestExamProvider(t *testing.T) {
	data := fetch.GetProviderExams("google")
	if len(data) == 0 {
		t.Fatalf("Expected non-empty data for provider 'google', but got: %v", data)
	}

	t.Logf("Got %d exams for provider 'google'", len(data))
}

func TestWriteData(t *testing.T) {
	outputPath := "write_test.md"
	links := fetch.GetAllPages("lpi", "010-160")
	utils.WriteData(links, outputPath, true)

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Expected file at %s but got error: %v", outputPath, err)
	}

	content := string(data)
	if !strings.Contains(content, "Comments:") {
		t.Errorf("Expected file content to contain 'Comments:' but got:\n%s", content)
	}

	err = os.Remove(outputPath)
	if err != nil {
		t.Fatalf("Error when removing file! %v", err)
	}
}
