package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gmext "github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	root    string
	content string
	media   string
	assets  string
)

type note struct {
	Path    string
	Slug    string
	Meta    map[string]any
	Body    string
	HTML    string
	URL     string
}

func (n *note) When() string { return eraWhen(n.Meta) }
func (n *note) Summary() string {
	if v, ok := n.Meta["summary"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
func (n *note) Title() string {
	if v, ok := n.Meta["title"]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}
func (n *note) Heading() string {
	dated := n.When()
	if s := n.Summary(); s != "" {
		dated = n.When() + " – " + s
	}
	if n.Title() != "" {
		return n.Title() + " — " + dated
	}
	return dated
}

type section struct {
	Path          string
	Slug          string
	URL           string
	Meta          map[string]any
	Notes         []*note
	Children      []*section
	HadDraftsOnly bool
}

func (s *section) Title() string {
	if v, ok := s.Meta["title"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return strings.Title(strings.ReplaceAll(s.Slug, "-", " "))
}
func (s *section) Order() int {
	if v, ok := s.Meta["order"]; ok {
		switch t := v.(type) {
		case int:
			return t
		case float64:
			return int(t)
		}
	}
	return 100
}
func (s *section) Group() string {
	if v, ok := s.Meta["group"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
func (s *section) Placeholder() bool {
	if v, ok := s.Meta["placeholder"]; ok {
		b, _ := v.(bool)
		return b
	}
	return false
}
func (s *section) SortOldest() bool {
	if v, ok := s.Meta["sort"]; ok {
		return fmt.Sprintf("%v", v) == "oldest"
	}
	return false
}

type page struct {
	URL  string
	Title string
	Meta map[string]any
	Body string
	HTML string
}

var months = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

func parseScalar(raw string) any {
	value := strings.TrimSpace(raw)
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	// int
	neg := false
	i := 0
	if len(value) > 0 && (value[0] == '-' || value[0] == '+') {
		if value[0] == '-' {
			neg = true
		}
		i = 1
	}
	allDigits := len(value) > i
	for j := i; j < len(value); j++ {
		if value[j] < '0' || value[j] > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		n := 0
		for j := i; j < len(value); j++ {
			n = n*10 + int(value[j]-'0')
		}
		if neg {
			n = -n
		}
		return n
	}
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[0] == value[len(value)-1] {
		inner := value[1 : len(value)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\'`, `'`)
		return inner
	}
	return value
}

func parseYAML(text string) map[string]any {
	root := map[string]any{}
	var listKey string
	var item map[string]any
	hasList := false
	for _, raw := range strings.Split(text, "\n") {
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		if strings.HasPrefix(raw, "  - ") {
			if !hasList {
				panic("list item outside list")
			}
			item = map[string]any{}
			arr := root[listKey].([]any)
			arr = append(arr, item)
			root[listKey] = arr
			rest := strings.TrimSpace(raw[4:])
			if rest != "" {
				parts := strings.SplitN(rest, ":", 2)
				item[strings.TrimSpace(parts[0])] = parseScalar(parts[1])
			}
			continue
		}
		if strings.HasPrefix(raw, "    ") && item != nil {
			parts := strings.SplitN(strings.TrimSpace(raw), ":", 2)
			item[strings.TrimSpace(parts[0])] = parseScalar(parts[1])
			continue
		}
		item = nil
		trimmed := strings.TrimSpace(raw)
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(strings.TrimSuffix(trimmed, ":"), " ") {
			listKey = strings.TrimSuffix(trimmed, ":")
			root[listKey] = []any{}
			hasList = true
			continue
		}
		hasList = false
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			continue
		}
		root[strings.TrimSpace(parts[0])] = parseScalar(parts[1])
	}
	return root
}

func splitFrontMatter(text string) (map[string]any, string) {
	if strings.HasPrefix(text, "---") {
		// split into 3
		rest := text[3:]
		// find next ---
		idx := strings.Index(rest, "---")
		if idx >= 0 {
			fm := rest[:idx]
			body := rest[idx+3:]
			body = strings.TrimPrefix(body, "\n")
			return parseYAML(fm), body
		}
	}
	return map[string]any{}, text
}

func parseWhen(value string) (string, string) {
	// returns iso date and precision
	parts := strings.Split(value, "-")
	year := parts[0]
	month := "01"
	day := "01"
	precision := "year"
	if len(parts) >= 2 {
		month = parts[1]
		precision = "month"
	}
	if len(parts) >= 3 {
		day = parts[2]
		precision = "day"
	}
	return fmt.Sprintf("%s-%s-%s", year, month, day), precision
}

func formatWhen(value string, precision string) string {
	iso, detected := parseWhen(value)
	if precision == "" {
		precision = detected
	}
	y := iso[0:4]
	m := iso[5:7]
	d := iso[8:10]
	mi, _ := atoi(m)
	if precision == "year" {
		return y
	}
	if precision == "month" {
		return months[mi-1] + " " + y
	}
	day, _ := atoi(d)
	return fmt.Sprintf("%s %d, %s", months[mi-1], day, y)
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func eraWhen(meta map[string]any) string {
	if v, ok := meta["label"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	start := formatWhen(fmt.Sprintf("%v", meta["date"]), "")
	if endRaw, ok := meta["date_end"]; ok && endRaw != nil {
		end := formatWhen(fmt.Sprintf("%v", endRaw), "")
		if start != end {
			// check same year suffix
			if strings.HasSuffix(start, end[len(end)-4:]) && start[len(start)-4:] == end[len(end)-4:] {
				return start[:len(start)-5] + "–" + end
			}
			return start + " – " + end
		}
	}
	return start
}

func sortKey(meta map[string]any) string {
	raw := ""
	if v, ok := meta["date_end"]; ok && v != nil {
		raw = fmt.Sprintf("%v", v)
	} else {
		raw = fmt.Sprintf("%v", meta["date"])
	}
	iso, _ := parseWhen(raw)
	return iso
}

func slugify(name string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(name, " ", "-")))
}

var mdConverter goldmark.Markdown

func initMD() {
	mdConverter = goldmark.New(
		goldmark.WithExtensions(gmext.Table, gmext.Footnote, gmext.DefinitionList),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)
}

func mdToHTML(text string) string {
	// youtube shortcode
	re1 := regexp.MustCompile(`\{\{<\s*youtube\s+"([^"]+)"\s*>\}\}`)
	text = re1.ReplaceAllStringFunc(text, func(m string) string {
		sub := re1.FindStringSubmatch(m)
		return "\n\nhttps://www.youtube.com/watch?v=" + sub[1] + "\n\n"
	})
	re2 := regexp.MustCompile(`(?m)^https://(?:www\.)?youtube\.com/watch\?v=([\w-]{6,})\s*$`)
	text = re2.ReplaceAllStringFunc(text, func(m string) string {
		sub := re2.FindStringSubmatch(m)
		return `<iframe class="video" src="https://www.youtube.com/embed/` + sub[1] + `" title="YouTube video" allowfullscreen loading="lazy"></iframe>`
	})
	re3 := regexp.MustCompile(`~~(.+?)~~`)
	text = re3.ReplaceAllString(text, `<del>$1</del>`)

	// lazy continuation: end a quote when a non-quote line follows directly
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(trimmed, ">") && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, ">") {
				out = append(out, "")
			}
		}
	}
	text = strings.Join(out, "\n")

	var buf strings.Builder
	if err := mdConverter.Convert([]byte(text), &buf); err != nil {
		panic(err)
	}
	rendered := buf.String()
	rendered = strings.TrimRight(rendered, "\n")
	// attribute-order normalisation to match Python-Markdown extra
	rendered = normaliseAttrs(rendered)
	return rendered
}

func inlineMD(text string) string {
	rendered := strings.TrimSpace(mdToHTML(text))
	rendered = regexp.MustCompile(`^<p>|</p>$`).ReplaceAllString(rendered, "")
	return rendered
}

func plainText(htmlText string) string {
	text := regexp.MustCompile(`(?s)<script.*?</script>`).ReplaceAllString(htmlText, " ")
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func isDraft(meta map[string]any) bool {
	v, ok := meta["draft"]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.ToLower(strings.TrimSpace(t)) == "true"
	}
	return false
}

// ... main etc
func normaliseAttrs(s string) string {
	// img: src then alt -> alt then src
	reImg := regexp.MustCompile(`<img src="([^"]*)" alt="([^"]*)"`)
	s = reImg.ReplaceAllString(s, `<img alt="$2" src="$1"`)
	// footnote anchor: reorder + drop role
	reFn := regexp.MustCompile(`<a href="(#[^"]*)" class="footnote-ref" role="doc-noteref">`)
	s = reFn.ReplaceAllString(s, `<a class="footnote-ref" href="$1">`)
	reBr := regexp.MustCompile(`<a href="(#[^"]*)" class="footnote-backref" role="[^"]*">[^<]*</a>`)
	s = reBr.ReplaceAllString(s, `<a class="footnote-backref" href="$1" title="Jump back to footnote 1 in the text">&#8617;</a>`)
	// footnote container
	s = strings.ReplaceAll(s, `<div class="footnotes" role="doc-endnotes">`, `<div class="footnote">`)
	// merge adjacent blockquotes
	s = regexp.MustCompile(`</blockquote>\s*\n\s*<blockquote>`).ReplaceAllString(s, "\n")
	// &quot; -> " only in text nodes (between > and <)
	reQ := regexp.MustCompile(`>([^<]*)&quot;([^<]*)<`)
	for {
		ns := reQ.ReplaceAllString(s, `>$1"$2<`)
		if ns == s {
			break
		}
		s = ns
	}
	return s
}

func main() {
	initMD()
	fmt.Println("not implemented fully")
}
