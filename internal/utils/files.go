package utils

import (
	"encoding/json"
	"examtopics-downloader/internal/models"
	"examtopics-downloader/internal/templates"
	"fmt"
	htmlpkg "html"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type answerOption struct {
	Letter string
	Text   string
}

type templateComment struct {
	User   string `json:"user"`
	Answer string `json:"answer"`
	Text   string `json:"text"`
}

type examMeta struct {
	Company  string
	ExamCode string
	Badge    string
}

var (
	titlePattern       = regexp.MustCompile(`(?is)<title>.*?</title>`)
	companyPattern     = regexp.MustCompile(`(?is)(<span class="company-name">).*?(</span>)`)
	headerTitlePattern = regexp.MustCompile(`(?is)(<span class="header-separator">\|</span>\s*)(.*?)(\s*</h1>)`)
	badgePattern       = regexp.MustCompile(`(?is)(<span class="badge">).*?(</span>)`)
	questionsListOpen  = regexp.MustCompile(`(?is)<div[^>]*class="[^"]*\bquestions-list\b[^"]*"[^>]*>`)

	discussionLinkPattern = regexp.MustCompile(`(?i)/discussions/([^/]+)/view/[^/]*-exam-([a-z0-9-]+)-topic-`)
	providerOnlyPattern   = regexp.MustCompile(`(?i)/discussions/([^/]+)/`)
	titleExamPattern      = regexp.MustCompile(`(?i)\bexam\s+(.+?)\s+topic\b`)

	codeWithPrefixPattern  = regexp.MustCompile(`(?i)\b[a-z0-9]{1,6}-\d{2,4}(?:-[a-z0-9]{1,4})*\b`)
	examCodePattern        = regexp.MustCompile(`(?i)\b\d{2,4}(?:-\d{2,4})+\b`)
	answerLetterPattern    = regexp.MustCompile(`(?i)\b([A-F])\b`)
	answerLettersRunes     = regexp.MustCompile(`(?i)[A-F]`)
	answerCompactPattern   = regexp.MustCompile(`(?i)\b([A-F]{2,6})\b`)
	optionDotPattern       = regexp.MustCompile(`(?m)^\s*([A-Fa-f])\s*[\.)]\s*(.+?)\s*$`)
	optionColonPattern     = regexp.MustCompile(`(?m)^\s*([A-Fa-f])\s*:\s*(.+?)\s*$`)
	imageMarkerPattern     = regexp.MustCompile(`\[\[IMG:([^\]]+)\]\]`)
	descURLPattern         = regexp.MustCompile(`https?://[^\s"'<>\]]+`)
	urlPattern             = regexp.MustCompile(`https?://[^\s"'<>]+`)
	imageURLPattern        = regexp.MustCompile(`(?i)^https?://[^\s"'<>]+(\.(png|jpg|jpeg|gif|webp|bmp|svg))?(\?[^\s"'<>]*)?$`)
	invalidCharsPattern    = regexp.MustCompile(`[^a-z0-9._-]+`)
	suggestedAnswerPattern = regexp.MustCompile(`(?is)\bSuggested\s+Answer\s*:\s*[A-F,\s/&+-]+`)
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
		fmt.Printf("writeFile: unsupported content type %T\n", v)
		return
	}
}

func WriteData(dataList []models.QuestionData, outputPath string, commentBool bool) ([]string, error) {
	return WriteDataWithSelection(dataList, outputPath, commentBool, "", "")
}

func WriteDataWithSelection(dataList []models.QuestionData, outputPath string, commentBool bool, selectedProvider string, selectedExam string) ([]string, error) {
	htmlDoc, err := buildTemplateDocument(dataList, commentBool, selectedProvider, selectedExam)
	if err != nil {
		return nil, err
	}

	htmlOutput := getHTMLOutputPath(outputPath)
	if err := os.WriteFile(htmlOutput, htmlDoc, 0644); err != nil {
		return nil, fmt.Errorf("failed to write html file: %w", err)
	}

	return []string{htmlOutput}, nil
}

func buildTemplateDocument(dataList []models.QuestionData, includeComments bool, selectedProvider string, selectedExam string) ([]byte, error) {
	templateShell, err := readTemplateShell()
	if err != nil {
		return nil, err
	}

	meta := deriveExamMeta(dataList, selectedProvider, selectedExam)
	withMeta := applyTemplateMeta(templateShell, meta)

	cardsHTML := buildQuestionCards(dataList, includeComments)
	finalDoc, err := injectQuestionCards(withMeta, cardsHTML)
	if err != nil {
		return nil, err
	}

	return []byte(finalDoc), nil
}

func readTemplateShell() (string, error) {
	candidates := []string{
		filepath.Join("internal", "templates", "template.html"),
		filepath.Join("templates", "template.html"),
		"template.html",
	}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		sourceRelative := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "templates", "template.html"))
		candidates = append(candidates, sourceRelative)
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}

		data, err := os.ReadFile(candidate)
		if err == nil {
			return string(data), nil
		}
	}

	embedded := strings.TrimSpace(templates.EmbeddedTemplate)
	if embedded != "" {
		return templates.EmbeddedTemplate, nil
	}

	return "", fmt.Errorf("could not locate template.html")
}

func deriveExamMeta(dataList []models.QuestionData, selectedProvider string, selectedExam string) examMeta {
	provider := strings.TrimSpace(strings.ToLower(selectedProvider))
	examSlug := sanitizeExamSlug(selectedExam)

	for _, item := range dataList {
		if item.QuestionLink != "" && (provider == "" || examSlug == "") {
			if matches := discussionLinkPattern.FindStringSubmatch(item.QuestionLink); len(matches) == 3 {
				if provider == "" {
					provider = strings.ToLower(strings.TrimSpace(matches[1]))
				}
				if examSlug == "" {
					examSlug = strings.ToLower(strings.TrimSpace(matches[2]))
				}
			}
			if provider == "" {
				if matches := providerOnlyPattern.FindStringSubmatch(item.QuestionLink); len(matches) == 2 {
					provider = strings.ToLower(strings.TrimSpace(matches[1]))
				}
			}
		}
		if examSlug == "" && item.Title != "" {
			if matches := titleExamPattern.FindStringSubmatch(item.Title); len(matches) == 2 {
				examSlug = sanitizeExamSlug(matches[1])
			}
		}
		if provider != "" && examSlug != "" {
			break
		}
	}

	company := providerDisplayName(provider)
	examCode := deriveExamCode(examSlug)
	if examCode == "" {
		examCode = "Exam"
	}

	badge := deriveBadge(provider, examCode, examSlug)
	if badge == "" {
		badge = strings.ToUpper(examCode)
	}

	return examMeta{
		Company:  company,
		ExamCode: examCode,
		Badge:    badge,
	}
}

func providerDisplayName(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return "ExamTopics"
	}

	overrides := map[string]string{
		"aws":              "AWS",
		"ec-council":       "EC-Council",
		"isc2":             "ISC2",
		"isaca":            "ISACA",
		"paloaltonetworks": "Palo Alto Networks",
		"servicenow":       "ServiceNow",
		"vmware":           "VMware",
		"lpi":              "LPI",
	}
	if val, ok := overrides[provider]; ok {
		return val
	}

	provider = strings.ReplaceAll(provider, "-", " ")
	parts := strings.Fields(provider)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	if len(parts) == 0 {
		return "ExamTopics"
	}
	return strings.Join(parts, " ")
}

func sanitizeExamSlug(input string) string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = invalidCharsPattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-._")
	return normalized
}

func deriveExamCode(examSlug string) string {
	examSlug = strings.TrimSpace(strings.ToLower(examSlug))
	if examSlug == "" {
		return ""
	}

	if code := codeWithPrefixPattern.FindString(examSlug); code != "" {
		return strings.ToUpper(code)
	}

	if code := examCodePattern.FindString(examSlug); code != "" {
		return strings.ToUpper(code)
	}

	words := strings.Fields(strings.ReplaceAll(examSlug, "-", " "))
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func deriveBadge(provider, examCode, examSlug string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "cisco" && strings.EqualFold(examCode, "200-301") {
		return "CCNA"
	}

	if examCode != "" {
		return strings.ToUpper(examCode)
	}

	if examSlug == "" {
		return "EXAM"
	}

	parts := strings.Fields(strings.ReplaceAll(examSlug, "-", " "))
	if len(parts) == 0 {
		return "EXAM"
	}

	var initials strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		initials.WriteString(strings.ToUpper(p[:1]))
		if initials.Len() >= 6 {
			break
		}
	}
	if initials.Len() == 0 {
		return "EXAM"
	}
	return initials.String()
}

func applyTemplateMeta(templateHTML string, meta examMeta) string {
	title := fmt.Sprintf("%s %s Exam Simulator", meta.Company, meta.ExamCode)
	headerText := fmt.Sprintf("%s Exam Simulator", meta.ExamCode)
	escapedTitle := htmlpkg.EscapeString(title)
	escapedHeaderText := htmlpkg.EscapeString(headerText)
	escapedCompany := htmlpkg.EscapeString(meta.Company)
	escapedBadge := htmlpkg.EscapeString(meta.Badge)

	updated := titlePattern.ReplaceAllString(templateHTML, fmt.Sprintf("<title>%s</title>", escapedTitle))
	updated = companyPattern.ReplaceAllString(updated, fmt.Sprintf("${1}%s${2}", escapedCompany))
	updated = headerTitlePattern.ReplaceAllString(updated, fmt.Sprintf("${1}%s${3}", escapedHeaderText))
	updated = badgePattern.ReplaceAllString(updated, fmt.Sprintf("${1}%s${2}", escapedBadge))

	return updated
}

func injectQuestionCards(templateHTML, cardsHTML string) (string, error) {
	if injected, found, err := injectIntoQuestionsList(templateHTML, cardsHTML); err != nil {
		return "", err
	} else if found {
		return injected, nil
	}

	noResultsStart := strings.Index(templateHTML, `<div class="no-results" id="noResults">`)
	if noResultsStart == -1 {
		return "", fmt.Errorf("no-results block not found in template")
	}

	noResultsEnd, err := findMatchingDivClose(templateHTML, noResultsStart)
	if err != nil {
		return "", err
	}

	footerStart := strings.Index(templateHTML, "<!-- FOOTER -->")
	if footerStart == -1 {
		footerStart = strings.Index(templateHTML, `<div class="page-footer">`)
	}
	if footerStart == -1 {
		return "", fmt.Errorf("footer anchor not found in template")
	}
	if footerStart <= noResultsEnd {
		return "", fmt.Errorf("invalid template structure for question injection")
	}

	prefix := strings.TrimRight(templateHTML[:noResultsEnd], "\r\n")
	suffix := strings.TrimLeft(templateHTML[footerStart:], "\r\n")
	injectedCards := indentBlock(strings.TrimSpace(cardsHTML), "    ")

	if injectedCards == "" {
		return prefix + "\n\n    " + suffix, nil
	}

	return prefix + "\n\n" + injectedCards + "\n\n    " + suffix, nil
}

func injectIntoQuestionsList(templateHTML, cardsHTML string) (string, bool, error) {
	openMatch := questionsListOpen.FindStringIndex(templateHTML)
	if openMatch == nil {
		return "", false, nil
	}

	openStart := openMatch[0]
	openEnd := openMatch[1]
	closeEnd, err := findMatchingDivClose(templateHTML, openStart)
	if err != nil {
		return "", true, fmt.Errorf("failed to locate questions-list closing tag: %w", err)
	}

	closeStart := closeEnd - len("</div>")
	if closeStart < openEnd {
		return "", true, fmt.Errorf("invalid questions-list structure in template")
	}

	lineStart := strings.LastIndex(templateHTML[:openStart], "\n")
	if lineStart == -1 {
		lineStart = 0
	} else {
		lineStart++
	}
	containerIndent := templateHTML[lineStart:openStart]
	childIndent := containerIndent + "  "

	injectedCards := indentBlock(strings.TrimSpace(cardsHTML), childIndent)
	prefix := strings.TrimRight(templateHTML[:openEnd], "\r\n")
	suffix := templateHTML[closeStart:]

	if injectedCards == "" {
		return prefix + "\n" + containerIndent + suffix, true, nil
	}

	return prefix + "\n" + injectedCards + "\n" + containerIndent + suffix, true, nil
}

func findMatchingDivClose(content string, startIdx int) (int, error) {
	depth := 0
	cursor := startIdx

	for cursor < len(content) {
		nextOpen := strings.Index(content[cursor:], "<div")
		nextClose := strings.Index(content[cursor:], "</div>")

		if nextOpen == -1 && nextClose == -1 {
			break
		}

		if nextOpen != -1 && (nextClose == -1 || nextOpen < nextClose) {
			depth++
			cursor += nextOpen + len("<div")
			continue
		}

		depth--
		cursor += nextClose + len("</div>")
		if depth == 0 {
			return cursor, nil
		}
	}

	return 0, fmt.Errorf("failed to locate closing </div> for no-results block")
}

func indentBlock(content, indent string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ""
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func buildQuestionCards(dataList []models.QuestionData, includeComments bool) string {
	var b strings.Builder
	questionNumber := 0
	droppedEmpty := 0
	renderedWithoutOptions := 0

	for _, data := range dataList {
		options := parseOptions(data.Questions)

		if len(options) == 0 {
			// Only drop when the entire question record is empty — otherwise
			// render it so image-only questions or pages with markup drift
			// still reach the output instead of silently disappearing.
			if !questionHasAnyContent(data) {
				droppedEmpty++
				continue
			}
			renderedWithoutOptions++
		}

		questionNumber++
		qid := fmt.Sprintf("q%d", questionNumber)
		isOpen := questionNumber == 1

		// Prefer the canonical Solution.Letter (from /view/) when available —
		// it's authoritative, whereas the discussion-page answer is a
		// best-effort guess.
		answerSource := data.Answer
		if data.Solution != nil && strings.TrimSpace(data.Solution.Letter) != "" {
			answerSource = data.Solution.Letter
		}
		// Skip the default "A" extractCorrectAnswers fallback for HOTSPOT /
		// image-only questions: there are no letter options and no letter
		// answer to commit to. data-correct stays empty so the interactive
		// UI doesn't claim a bogus correct option.
		correct := ""
		if len(options) > 0 || strings.TrimSpace(answerSource) != "" {
			correct = strings.Join(extractCorrectAnswers(answerSource, options), ",")
		}
		link := htmlpkg.EscapeString(strings.TrimSpace(data.QuestionLink))
		commentsJSON := buildCommentsJSON(data.Comments, includeComments)

		questionText, previewText, exhibitURLs, answerExhibitURLs := buildQuestionTextAndPreview(data)

		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(renderQuestionCard(qid, questionNumber, isOpen, correct, link, commentsJSON, questionText, previewText, exhibitURLs, answerExhibitURLs, data.Solution, options))
	}

	if droppedEmpty > 0 {
		fmt.Fprintf(os.Stderr, "[WARN] Dropped %d question(s) with no content (no text, options, or exhibits).\n", droppedEmpty)
	}
	if renderedWithoutOptions > 0 {
		fmt.Fprintf(os.Stderr, "[INFO] Rendered %d question(s) without parseable options (image-only or unrecognised markup).\n", renderedWithoutOptions)
	}

	if b.Len() == 0 {
		return ""
	}

	return strings.TrimSpace(b.String())
}

func questionHasAnyContent(data models.QuestionData) bool {
	if strings.TrimSpace(data.Content) != "" {
		return true
	}
	if strings.TrimSpace(data.Header) != "" {
		return true
	}
	if strings.TrimSpace(data.Title) != "" {
		return true
	}
	if len(data.ExhibitURLs) > 0 {
		return true
	}
	if len(data.AnswerExhibitURLs) > 0 {
		return true
	}
	for _, q := range data.Questions {
		if strings.TrimSpace(q) != "" {
			return true
		}
	}
	return false
}

func renderQuestionCard(
	qid string,
	questionNumber int,
	isOpen bool,
	correct string,
	link string,
	commentsJSON string,
	questionText string,
	previewText string,
	exhibitURLs []string,
	answerExhibitURLs []string,
	solution *models.AnswerSolution,
	options []answerOption,
) string {
	var b strings.Builder

	cardClass := "q-card"
	if isOpen {
		cardClass += " open"
	}

	fmt.Fprintf(&b, "<!-- QUESTION %d -->\n", questionNumber)
	fmt.Fprintf(&b, "<div class=\"%s\" id=\"%s\" data-correct=\"%s\"\n", cardClass, qid, correct)
	fmt.Fprintf(&b, "     data-link=\"%s\"\n", link)
	fmt.Fprintf(&b, "     data-comments='%s'>\n", commentsJSON)
	fmt.Fprintf(&b, "    <div class=\"q-top\" onclick=\"toggleCard('%s')\">\n", qid)
	fmt.Fprintf(&b, "        <span class=\"q-number\">Q%d</span>\n", questionNumber)
	fmt.Fprintf(&b, "        <span class=\"q-preview\" id=\"%s-preview\">%s</span>\n", qid, previewText)
	fmt.Fprintf(&b, "        <span class=\"q-status\" id=\"%s-status\"></span>\n", qid)
	b.WriteString("        <span class=\"q-toggle\">&#9662;</span>\n")
	b.WriteString("    </div>\n")
	b.WriteString("    <div class=\"q-body\">\n")

	if len(exhibitURLs) > 0 {
		for idx, exhibitURL := range exhibitURLs {
			label := "Exhibit"
			if len(exhibitURLs) > 1 {
				label = fmt.Sprintf("Exhibit %d", idx+1)
			}

			fmt.Fprintf(&b, "        <div class=\"q-exhibit\">\n")
			fmt.Fprintf(&b, "            <span class=\"q-exhibit-label\">%s</span>\n", htmlpkg.EscapeString(label))
			fmt.Fprintf(&b, "            <img src=\"%s\" alt=\"%s\"\n", htmlpkg.EscapeString(exhibitURL), htmlpkg.EscapeString(label))
			b.WriteString("                 onerror=\"this.parentElement.style.display='none'\"\n")
			b.WriteString("                 onclick=\"zoomImage(this.src)\">\n")
			b.WriteString("            <button class=\"q-exhibit-zoom\" onclick=\"zoomImage(this.parentElement.querySelector('img').src)\" title=\"Zoom image\">+</button>\n")
			b.WriteString("        </div>\n")
		}
	}

	fmt.Fprintf(&b, "        <div class=\"q-text\" id=\"%s-text\">%s</div>\n", qid, questionText)

	fmt.Fprintf(&b, "        <div class=\"opts\" id=\"%s-opts\">\n", qid)
	switch {
	case len(options) == 0 && solution != nil:
		// HOTSPOT-style canonical answer from /exams/{provider}/{slug}/view/.
		// The Hot Area image from the question body is intentionally NOT shown
		// as the answer here — it's the user-interaction area, not the result.
		renderCanonicalAnswerBlock(&b, solution)
	case len(options) == 0 && len(answerExhibitURLs) > 0:
		// No canonical solution available (typically because the question is
		// beyond ExamTopics' freely-exposed quota on /view/). Fall back to
		// the Hot Area image from the question body, but label it clearly
		// as the interaction area — NOT the verified answer.
		renderHotAreaFallbackBlock(&b, answerExhibitURLs)
	case len(options) == 0:
		b.WriteString("            <div class=\"opt-empty\">No selectable answer options were extracted for this question. See the exhibit or question text above; the original discussion is linked at the bottom.</div>\n")
	}
	for _, option := range options {
		letter := htmlpkg.EscapeString(option.Letter)
		cleanText, imageURLs := splitImageMarkers(option.Text)
		text := htmlpkg.EscapeString(cleanText)
		fmt.Fprintf(&b, "            <div class=\"opt\" data-val=\"%s\" onclick=\"pick(this,'%s')\">\n", letter, qid)
		fmt.Fprintf(&b, "                <div class=\"opt-letter\">%s</div>\n", letter)
		fmt.Fprintf(&b, "                <div class=\"opt-text\" data-original=\"%s\">%s</div>\n", text, text)
		for _, imgURL := range imageURLs {
			escaped := htmlpkg.EscapeString(imgURL)
			fmt.Fprintf(&b, "                <img class=\"opt-image\" src=\"%s\" alt=\"Option %s\" onerror=\"this.style.display='none'\" onclick=\"event.stopPropagation();zoomImage(this.src)\">\n", escaped, letter)
		}
		b.WriteString("            </div>\n")
	}
	b.WriteString("        </div>\n")

	fmt.Fprintf(&b, "        <div class=\"result-bar\" id=\"%s-result\"></div>\n", qid)
	b.WriteString("        <div class=\"q-actions\">\n")
	fmt.Fprintf(&b, "            <button class=\"btn btn-submit\" id=\"%s-submit\" onclick=\"submit('%s')\" disabled>Submit</button>\n", qid, qid)
	fmt.Fprintf(&b, "            <button class=\"btn btn-cheat\" id=\"%s-cheat\" onclick=\"cheat('%s')\">Sneak Peek</button>\n", qid, qid)
	fmt.Fprintf(&b, "            <a class=\"btn btn-discuss hidden\" id=\"%s-discuss\" href=\"#\" target=\"_blank\">ExamTopics</a>\n", qid)
	fmt.Fprintf(&b, "            <button class=\"btn btn-comments\" id=\"%s-comments\" onclick=\"openComments('%s')\">Comments</button>\n", qid, qid)
	fmt.Fprintf(&b, "            <button class=\"btn btn-reset hidden\" id=\"%s-reset\" onclick=\"reset('%s')\">Retry</button>\n", qid, qid)
	b.WriteString("        </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("</div>")

	return b.String()
}

func buildQuestionTextAndPreview(data models.QuestionData) (string, string, []string, []string) {
	answerExhibitURLs := extractAnswerExhibitURLs(data)
	exhibitURLs := extractQuestionExhibitURLs(data, answerExhibitURLs)

	content := removeSuggestedAnswerText(cleanQuestionText(stripImageURLs(data.Content)))
	header := removeSuggestedAnswerText(cleanQuestionText(stripImageURLs(data.Header)))
	title := removeSuggestedAnswerText(cleanQuestionText(stripImageURLs(data.Title)))

	var body string
	switch {
	case content != "" && !looksLikeExamMetadata(content):
		body = content
	case header != "" && !looksLikeExamMetadata(header):
		body = header
	case content != "":
		body = content
	case header != "":
		body = header
	default:
		body = title
	}

	if body == "" {
		body = "Question text unavailable."
	}

	preview := truncatePreview(body, 95)
	formattedBody := formatHTMLText(body)

	return formattedBody, htmlpkg.EscapeString(preview), exhibitURLs, answerExhibitURLs
}

func cleanQuestionText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n")
}

func removeSuggestedAnswerText(text string) string {
	if text == "" {
		return ""
	}
	stripped := suggestedAnswerPattern.ReplaceAllString(text, "")
	return cleanQuestionText(stripped)
}

func looksLikeExamMetadata(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "question #") ||
		strings.Contains(lower, "topic #") ||
		strings.Contains(lower, "actual exam question from")
}

func stripImageURLs(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	text = urlPattern.ReplaceAllStringFunc(text, func(match string) string {
		if isLikelyImageURL(match) {
			return ""
		}
		return match
	})

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	return text
}

// extractAnswerExhibitURLs returns the answer-side image URLs only, applying
// the same dedup/validation rules as question-side exhibits. Used to render
// the dedicated answer-block for HOTSPOT/DRAG-DROP questions.
func extractAnswerExhibitURLs(data models.QuestionData) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(data.AnswerExhibitURLs))

	add := func(rawURL string) {
		candidate := trimTrailingPunctuation(strings.TrimSpace(rawURL))
		if candidate == "" {
			return
		}
		if !isLikelyImageURL(candidate) {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	for _, url := range data.AnswerExhibitURLs {
		add(url)
	}
	return out
}

// extractQuestionExhibitURLs returns the question-side image URLs, taking
// AnswerExhibitURLs into account so the same URL is never rendered both as a
// question exhibit and as an answer exhibit. URL fallbacks scraped out of the
// question text are also subtracted against the answer set.
func extractQuestionExhibitURLs(data models.QuestionData, answerURLs []string) []string {
	seen := map[string]struct{}{}
	// Pre-seed with answer URLs so the URL-from-text fallback can't re-add
	// them on the question side.
	for _, u := range answerURLs {
		candidate := trimTrailingPunctuation(strings.TrimSpace(u))
		if candidate != "" {
			seen[candidate] = struct{}{}
		}
	}
	out := make([]string, 0, 2)

	add := func(rawURL string) {
		candidate := trimTrailingPunctuation(strings.TrimSpace(rawURL))
		if candidate == "" {
			return
		}
		if !isLikelyImageURL(candidate) {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}

	for _, exhibit := range data.ExhibitURLs {
		add(exhibit)
	}

	combined := strings.Join([]string{data.Content, data.Header, strings.Join(data.Questions, "\n")}, "\n")
	urls := urlPattern.FindAllString(combined, -1)
	for _, rawURL := range urls {
		add(rawURL)
	}

	return out
}

// extractExhibitURLs is kept for backward compatibility with callers/tests
// that don't differentiate question and answer images.
func extractExhibitURLs(data models.QuestionData) []string {
	answer := extractAnswerExhibitURLs(data)
	q := extractQuestionExhibitURLs(data, answer)
	if len(answer) == 0 {
		return q
	}
	out := make([]string, 0, len(q)+len(answer))
	out = append(out, q...)
	out = append(out, answer...)
	return out
}

func trimTrailingPunctuation(input string) string {
	return strings.TrimRight(input, ".,;)")
}

// renderCanonicalAnswerBlock writes the authoritative "Reveal Solution"
// content (scraped from /exams/.../view/) into the answer slot of a question
// card. It surfaces the correct letter/image, an explanation paragraph with
// inline images, and a tidied references list.
func renderCanonicalAnswerBlock(b *strings.Builder, sol *models.AnswerSolution) {
	b.WriteString("            <div class=\"answer-block answer-block-canonical\">\n")
	b.WriteString("                <h4 class=\"answer-block-title\">Correct Answer</h4>\n")

	if letter := strings.TrimSpace(sol.Letter); letter != "" {
		fmt.Fprintf(b, "                <div class=\"answer-letter\">%s</div>\n", htmlpkg.EscapeString(letter))
	}

	// Inline image(s) that sit directly inside .correct-answer (HOTSPOT case).
	for _, imgURL := range sol.Images {
		escaped := htmlpkg.EscapeString(imgURL)
		b.WriteString("                <div class=\"answer-exhibit\">\n")
		fmt.Fprintf(b, "                    <img src=\"%s\" alt=\"Correct answer\"\n", escaped)
		b.WriteString("                         onerror=\"this.parentElement.style.display='none'\"\n")
		b.WriteString("                         onclick=\"zoomImage(this.src)\">\n")
		b.WriteString("                    <button class=\"q-exhibit-zoom\" onclick=\"zoomImage(this.parentElement.querySelector('img').src)\" title=\"Zoom image\">+</button>\n")
		b.WriteString("                </div>\n")
	}

	// Description (explanation paragraphs with interleaved images).
	if strings.TrimSpace(sol.Description) != "" {
		b.WriteString("                <div class=\"answer-description\">")
		b.WriteString(renderAnswerDescriptionHTML(sol.Description))
		b.WriteString("</div>\n")
	}

	// References extracted from the description text (links the user can
	// click to read the underlying documentation).
	if len(sol.References) > 0 {
		b.WriteString("                <div class=\"answer-references\">\n")
		b.WriteString("                    <strong>References:</strong>\n")
		b.WriteString("                    <ul>\n")
		for _, ref := range sol.References {
			esc := htmlpkg.EscapeString(ref)
			fmt.Fprintf(b, "                        <li><a href=\"%s\" target=\"_blank\" rel=\"noopener\">%s</a></li>\n", esc, esc)
		}
		b.WriteString("                    </ul>\n")
		b.WriteString("                </div>\n")
	}

	b.WriteString("            </div>\n")
}

// renderHotAreaFallbackBlock surfaces the question's own Hot Area / answer-area
// image when no canonical solution is available. The label is intentionally
// "Answer Area" rather than "Answer" so users understand this is the
// interaction surface from the question itself, not a verified answer.
func renderHotAreaFallbackBlock(b *strings.Builder, answerExhibitURLs []string) {
	b.WriteString("            <div class=\"answer-block answer-block-fallback\">\n")
	b.WriteString("                <h4 class=\"answer-block-title\">Answer Area (unverified)</h4>\n")
	b.WriteString("                <p class=\"answer-block-note\">Canonical solution not freely available from ExamTopics. The image below is the question's own interaction area; cross-check with the community discussion linked at the bottom.</p>\n")
	for idx, answerURL := range answerExhibitURLs {
		label := "Answer Area"
		if len(answerExhibitURLs) > 1 {
			label = fmt.Sprintf("Answer Area %d", idx+1)
		}
		escaped := htmlpkg.EscapeString(answerURL)
		b.WriteString("                <div class=\"answer-exhibit\">\n")
		fmt.Fprintf(b, "                    <img src=\"%s\" alt=\"%s\"\n", escaped, htmlpkg.EscapeString(label))
		b.WriteString("                         onerror=\"this.parentElement.style.display='none'\"\n")
		b.WriteString("                         onclick=\"zoomImage(this.src)\">\n")
		b.WriteString("                    <button class=\"q-exhibit-zoom\" onclick=\"zoomImage(this.parentElement.querySelector('img').src)\" title=\"Zoom image\">+</button>\n")
		b.WriteString("                </div>\n")
	}
	b.WriteString("            </div>\n")
}

// renderAnswerDescriptionHTML produces the inner HTML for the answer
// description paragraph. Input text is plain (output of serializeAnswerDescription):
//   - "\n" denotes a line break (rendered as <br>)
//   - "[[IMG:<url>]]" denotes an inline image
//   - http(s) URLs are rendered as clickable links
// Everything else is HTML-escaped.
func renderAnswerDescriptionHTML(text string) string {
	if text == "" {
		return ""
	}
	parts := imageMarkerPattern.Split(text, -1)
	markers := imageMarkerPattern.FindAllStringSubmatch(text, -1)

	var b strings.Builder
	for i, segment := range parts {
		b.WriteString(formatAnswerDescriptionSegment(segment))
		if i < len(markers) {
			esc := htmlpkg.EscapeString(strings.TrimSpace(markers[i][1]))
			fmt.Fprintf(&b, `<img class="answer-desc-image" src="%s" alt="" onerror="this.style.display='none'" onclick="zoomImage(this.src)">`, esc)
		}
	}
	return b.String()
}

// formatAnswerDescriptionSegment HTML-escapes a text fragment, turns http(s)
// URLs into <a> tags, and converts "\n" to "<br>". URLs are sentinel-replaced
// before escaping so the escaping does not mangle their contents.
func formatAnswerDescriptionSegment(segment string) string {
	if segment == "" {
		return ""
	}
	var refs []string
	with := descURLPattern.ReplaceAllStringFunc(segment, func(u string) string {
		u = strings.TrimRight(u, ".,;:)\"'")
		refs = append(refs, u)
		return fmt.Sprintf("\x01U%d\x01", len(refs)-1)
	})
	escaped := htmlpkg.EscapeString(with)
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	for i, ref := range refs {
		token := fmt.Sprintf("\x01U%d\x01", i)
		esc := htmlpkg.EscapeString(ref)
		anchor := fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`, esc, esc)
		escaped = strings.Replace(escaped, token, anchor, 1)
	}
	return escaped
}

// splitImageMarkers strips [[IMG:<url>]] markers out of an option text and
// returns the cleaned text plus the extracted URLs (in order, deduplicated).
func splitImageMarkers(text string) (string, []string) {
	if !strings.Contains(text, "[[IMG:") {
		return strings.TrimSpace(text), nil
	}
	var urls []string
	seen := map[string]struct{}{}
	cleaned := imageMarkerPattern.ReplaceAllStringFunc(text, func(match string) string {
		m := imageMarkerPattern.FindStringSubmatch(match)
		if len(m) == 2 {
			u := strings.TrimSpace(m[1])
			if u != "" {
				if _, ok := seen[u]; !ok {
					seen[u] = struct{}{}
					urls = append(urls, u)
				}
			}
		}
		return ""
	})
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned, urls
}

func isLikelyImageURL(input string) bool {
	lower := strings.ToLower(input)
	if strings.Contains(lower, "img.examtopics.com") {
		return true
	}
	if !imageURLPattern.MatchString(input) {
		return false
	}
	pathOnly := strings.SplitN(lower, "?", 2)[0]
	return strings.HasSuffix(pathOnly, ".png") ||
		strings.HasSuffix(pathOnly, ".jpg") ||
		strings.HasSuffix(pathOnly, ".jpeg") ||
		strings.HasSuffix(pathOnly, ".gif") ||
		strings.HasSuffix(pathOnly, ".webp") ||
		strings.HasSuffix(pathOnly, ".bmp") ||
		strings.HasSuffix(pathOnly, ".svg") ||
		strings.Contains(lower, "/image")
}

func truncatePreview(text string, maxLen int) string {
	plain := strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
	if plain == "" {
		return "Question preview"
	}

	runes := []rune(plain)
	if len(runes) <= maxLen {
		return plain
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func formatHTMLText(text string) string {
	escaped := htmlpkg.EscapeString(text)
	escaped = strings.ReplaceAll(escaped, "\n\n", "<br><br>")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return escaped
}

func parseOptions(rawOptions []string) []answerOption {
	joined := strings.Join(rawOptions, "\n")
	joined = strings.ReplaceAll(joined, "\r", "")
	joined = strings.ReplaceAll(joined, "**", "")

	matches := optionDotPattern.FindAllStringSubmatch(joined, -1)
	if len(matches) == 0 {
		matches = optionColonPattern.FindAllStringSubmatch(joined, -1)
	}

	options := make([]answerOption, 0, 6)
	seen := map[string]struct{}{}

	for _, m := range matches {
		if len(m) != 3 {
			continue
		}
		letter := strings.ToUpper(strings.TrimSpace(m[1]))
		text := strings.TrimSpace(m[2])
		if letter == "" || text == "" {
			continue
		}
		if _, exists := seen[letter]; exists {
			continue
		}
		seen[letter] = struct{}{}
		options = append(options, answerOption{Letter: letter, Text: text})
	}

	if len(options) > 0 {
		return options
	}

	letters := []string{"A", "B", "C", "D", "E", "F"}
	for i, line := range rawOptions {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		clean = strings.TrimPrefix(clean, "-")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		if i >= len(letters) {
			break
		}
		options = append(options, answerOption{Letter: letters[i], Text: clean})
	}

	return options
}

func extractCorrectAnswers(raw string, options []answerOption) []string {
	raw = strings.TrimSpace(raw)
	answers := map[string]struct{}{}

	if raw != "" {
		for _, m := range answerLetterPattern.FindAllStringSubmatch(strings.ToUpper(raw), -1) {
			if len(m) == 2 {
				answers[m[1]] = struct{}{}
			}
		}

		if len(answers) == 0 {
			if m := answerCompactPattern.FindStringSubmatch(strings.ToUpper(raw)); len(m) == 2 {
				for _, letter := range answerLettersRunes.FindAllString(m[1], -1) {
					answers[letter] = struct{}{}
				}
			}
		}
	}

	if len(answers) == 0 {
		normRaw := normalizeForComparison(raw)
		for _, opt := range options {
			if normRaw != "" && (normRaw == normalizeForComparison(opt.Text) || strings.Contains(normRaw, normalizeForComparison(opt.Text))) {
				answers[opt.Letter] = struct{}{}
			}
		}
	}

	ordered := make([]string, 0, len(answers))
	for _, opt := range options {
		if _, ok := answers[opt.Letter]; ok {
			ordered = append(ordered, opt.Letter)
		}
	}

	if len(ordered) > 0 {
		return ordered
	}
	// No answer could be determined from the suggested-answer markup, the
	// community vote tally, or text matching. Return nothing rather than
	// guessing the first option: a confident-but-wrong "A" is worse than an
	// honestly-empty answer (the card renders with no pre-marked correct
	// option, exactly like image-only HOTSPOT questions).
	return nil
}

func normalizeForComparison(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = invalidCharsPattern.ReplaceAllString(text, "")
	return text
}

func buildCommentsJSON(raw []models.CommentData, includeComments bool) string {
	comments := []templateComment{}
	if includeComments {
		for _, comment := range raw {
			user := strings.TrimSpace(comment.User)
			if user == "" {
				user = "Anonymous"
			}

			comments = append(comments, templateComment{
				User:   user,
				Answer: strings.ToUpper(strings.TrimSpace(comment.Answer)),
				Text:   strings.TrimSpace(comment.Text),
			})
		}
	}

	payload, err := json.Marshal(comments)
	if err != nil {
		return htmlpkg.EscapeString("[]")
	}
	return htmlpkg.EscapeString(string(payload))
}

func getHTMLOutputPath(outputPath string) string {
	cleanPath := strings.TrimSpace(outputPath)
	if cleanPath == "" {
		cleanPath = "examtopics_output"
	}

	base := strings.TrimSuffix(cleanPath, filepath.Ext(cleanPath))
	if base == "" {
		base = cleanPath
	}

	return base + ".html"
}

func SaveLinks(filename string, links []models.QuestionData) {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("  <meta charset=\"utf-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("  <title>Saved Question Links</title>\n</head>\n<body>\n")
	b.WriteString("  <h1>Saved Question Links</h1>\n  <ul>\n")

	for _, link := range links {
		if link.QuestionLink == "" {
			continue
		}
		fmt.Fprintf(&b, "    <li><a href=\"%s\">%s</a></li>\n", link.QuestionLink, link.QuestionLink)
	}

	b.WriteString("  </ul>\n</body>\n</html>\n")
	writeFile(filename, b.String())
}
