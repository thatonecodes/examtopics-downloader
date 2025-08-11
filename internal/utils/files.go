package utils

import (
	"examtopics-downloader/internal/models"
	"fmt"
	"log"
)

func writeFile(filename string, content any) {
	file := CreateFile(filename)
	defer file.Close()

	switch v := content.(type) {
	case string:
		fmt.Fprintln(file, v)
	case []string:
		for _, line := range v {
			fmt.Fprintln(file, line)
		}
	default:
		log.Printf("writeFile: unsupported content type %T", v)
	}
}

func WriteData(dataList []models.QuestionData, outputPath string, commentBool bool) {
	file := CreateFile(outputPath)
	defer file.Close()

	fmt.Fprintf(file, "# Exam Topics Questions\n\n")
	fmt.Fprintf(file, "@thatonecodes\n\n")

	for _, data := range dataList {
		if data.Title == "" {
			continue
		}

		fmt.Fprintf(file, "## %s\n\n", data.Title)
		fmt.Fprintf(file, "%s\n\n", data.Header)

		if data.Content != "" {
			fmt.Fprintf(file, "%s\n\n", data.Content)
		}

		for _, question := range data.Questions {
			fmt.Fprintf(file, "%s\n\n", question)
		}

		fmt.Fprintf(file, "**Answer: %s**\n\n", data.Answer)
		fmt.Fprintf(file, "**Timestamp: %s**\n\n", data.Timestamp)
		fmt.Fprintf(file, "[View on ExamTopics](%s)\n\n", data.QuestionLink)

		if commentBool {
			fmt.Fprintf(file, "Comments: %s\n", data.Comments)
		}

		fmt.Fprintf(file, "----------------------------------------\n\n")
	}
}

func SaveLinks(filename string, links []models.QuestionData) {
	var fullLinks []string
	for _, link := range links {
		fullLinks = append(fullLinks, link.QuestionLink)
	}
	writeFile(filename, fullLinks)
}
