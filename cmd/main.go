package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"examtopics-downloader/internal/fetch"
	"examtopics-downloader/internal/utils"
)

func main() {
	provider := flag.String("p", "google", "Name of the exam provider (default -> google)")
	grepStr := flag.String("s", "", "String to grep for in discussion links (required)")
	outputPath := flag.String("o", "examtopics_output.md", "Optional path of the file where the data will be outputted")
	commentBool := flag.Bool("c", false, "Optionally include all the comment/discussion text")
	examsFlag := flag.Bool("exams", false, "Optionally show all the possible exams for your selected provider and exit")
	saveUrls := flag.Bool("save-links", false, "Optional argument to save unique links to questions")
	flag.Parse()

	if *examsFlag {
		exams := fetch.GetProviderExams(*provider)
		fmt.Printf("Exams for provider '%s'\n\n", *provider)
		for _, exam := range exams {
			fmt.Println(utils.AddToBaseUrl(exam))
		}
		os.Exit(0)
	}

	if *grepStr == "" {
		log.Println("running without a valid string to search for with -s, (no_grep_str)!")
	}

	links := fetch.GetAllPages(*provider, *grepStr)

	if *saveUrls {
		utils.SaveLinks("saved-links.txt", links)
	}

	utils.WriteData(links, *outputPath, *commentBool)

	fmt.Printf("Successfully saved output to %s.\n", *outputPath)
}
