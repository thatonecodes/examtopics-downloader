package fetch

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func TestExtractMostVotedAnswerSingle(t *testing.T) {
	doc := mustDoc(t, `
<div class="question-body">
  <div class="voted-answers-tally d-none">
    <script type="application/json">[{"voted_answers": "D", "vote_count": 12, "is_most_voted": true}]</script>
  </div>
</div>`)
	if got := extractMostVotedAnswer(doc); got != "D" {
		t.Fatalf("want D, got %q", got)
	}
}

func TestExtractMostVotedAnswerMulti(t *testing.T) {
	doc := mustDoc(t, `
<div class="question-body">
  <div class="voted-answers-tally d-none">
    <script type="application/json">[{"voted_answers": "AC", "vote_count": 30, "is_most_voted": true},{"voted_answers": "AD", "vote_count": 5, "is_most_voted": false}]</script>
  </div>
</div>`)
	if got := extractMostVotedAnswer(doc); got != "AC" {
		t.Fatalf("want AC, got %q", got)
	}
}

func TestExtractMostVotedAnswerFallsBackToHighestCount(t *testing.T) {
	// No entry flagged is_most_voted -> pick the highest vote_count.
	doc := mustDoc(t, `
<div class="voted-answers-tally d-none">
  <script type="application/json">[{"voted_answers": "B", "vote_count": 2, "is_most_voted": false},{"voted_answers": "E", "vote_count": 9, "is_most_voted": false}]</script>
</div>`)
	if got := extractMostVotedAnswer(doc); got != "E" {
		t.Fatalf("want E (highest count), got %q", got)
	}
}

func TestExtractMostVotedAnswerEmptyOrMalformed(t *testing.T) {
	cases := map[string]string{
		"no tally":     `<div class="question-body"><p>hi</p></div>`,
		"empty script": `<div class="voted-answers-tally"><script type="application/json"></script></div>`,
		"malformed":    `<div class="voted-answers-tally"><script type="application/json">not json</script></div>`,
		"image vote":   `<div class="voted-answers-tally"><script type="application/json">[{"voted_answers": "", "vote_count": 4, "is_most_voted": true}]</script></div>`,
	}
	for name, html := range cases {
		if got := extractMostVotedAnswer(mustDoc(t, html)); got != "" {
			t.Errorf("%s: want empty, got %q", name, got)
		}
	}
}
