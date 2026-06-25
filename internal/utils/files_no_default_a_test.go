package utils

import (
	"reflect"
	"strings"
	"testing"

	"examtopics-downloader/internal/models"
)

// When no answer can be determined, extractCorrectAnswers must return nothing
// rather than guessing the first option ("A"). A confident-but-wrong answer is
// worse than an honestly-empty one.
func TestExtractCorrectAnswersNoGuessWhenUnknown(t *testing.T) {
	options := []answerOption{
		{Letter: "A", Text: "first"},
		{Letter: "B", Text: "second"},
	}
	if got := extractCorrectAnswers("", options); got != nil {
		t.Fatalf("expected nil for empty answer, got %v", got)
	}
	if got := extractCorrectAnswers("no letters here at all", options); got != nil {
		t.Fatalf("expected nil when no letter resolvable, got %v", got)
	}
}

func TestExtractCorrectAnswersStillResolvesKnown(t *testing.T) {
	options := []answerOption{
		{Letter: "A", Text: "first"},
		{Letter: "B", Text: "second"},
		{Letter: "C", Text: "third"},
	}
	got := extractCorrectAnswers("Correct: C", options)
	if !reflect.DeepEqual(got, []string{"C"}) {
		t.Fatalf("want [C], got %v", got)
	}
}

// A multi-choice card whose answer is unknown must render with an empty
// data-correct (no pre-marked option), never data-correct="A".
func TestRenderCardNeverDefaultsToA(t *testing.T) {
	data := []models.QuestionData{{
		Title:        "Q",
		Content:      "pick the best",
		QuestionLink: "https://www.examtopics.com/discussions/cisco/view/1-exam-200-301-topic-1-question-1-discussion/",
		Questions:    []string{"A. a", "B. b", "C. c", "D. d"},
		Answer:       "", // nothing scraped
	}}
	html := buildQuestionCards(data, false)
	if !strings.Contains(html, `data-correct=""`) {
		t.Fatalf("expected empty data-correct when answer unknown, got:\n%s", html)
	}
	if strings.Contains(html, `data-correct="A"`) {
		t.Fatalf("must not default to data-correct=\"A\":\n%s", html)
	}
}
