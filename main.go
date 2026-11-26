package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	gmext "github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// ---------- types ----------

type note struct {
	Path string
	Slug string
	Meta map[string]any
	Body string
	HTML string
	URL  string
}

func (n *note) When() string   { return eraWhen(n.Meta) }
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

type Section struct {
	Path          string
	Slug          string
	URL           string
	Meta          map[string]any
	Notes         []*note
	Children      []*Section
	HadDraftsOnly bool
}

func (s *Section) Title() string {
	if v, ok := s.Meta["title"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return strings.Title(strings.ReplaceAll(s.Slug, "-", " "))
}
func (s *Section) Order() int {
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
func (s *Section) Group() string {
	if v, ok := s.Meta["group"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
func (s *Section) Placeholder() bool {
	if v, ok := s.Meta["placeholder"]; ok {
		b, _ := v.(bool)
		return b
	}
	return false
}
func (s *Section) SortOldest() bool {
	if v, ok := s.Meta["sort"]; ok {
		return fmt.Sprintf("%v", v) == "oldest"
	}
	return false
}

type page struct {
	URL   string
	Title string
	Meta  map[string]any
	Body  string
	HTML  string
}

var months = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

// ---------- helpers ----------

func parseScalar(raw string) any {
	value := strings.TrimSpace(raw)
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
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
		rest := text[3:]
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

func eraWhen(meta map[string]any) string {
	if v, ok := meta["label"]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	start := formatWhen(fmt.Sprintf("%v", meta["date"]), "")
	if endRaw, ok := meta["date_end"]; ok && endRaw != nil {
		end := formatWhen(fmt.Sprintf("%v", endRaw), "")
		if start != end {
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

func normaliseAttrs(s string) string {
	reImg := regexp.MustCompile(`<img src="([^"]*)" alt="([^"]*)"`)
	s = reImg.ReplaceAllString(s, `<img alt="$2" src="$1"`)
	reFn := regexp.MustCompile(`<a href="(#[^"]*)" class="footnote-ref" role="doc-noteref">`)
	s = reFn.ReplaceAllString(s, `<a class="footnote-ref" href="$1">`)
	s = strings.ReplaceAll(s, `<div class="footnotes" role="doc-endnotes">`, `<div class="footnote">`)
	reBr := regexp.MustCompile(`<a href="#fnref:([0-9]+)" class="footnote-backref" role="[^"]*">[^<]*</a>`)
	s = reBr.ReplaceAllStringFunc(s, func(m string) string {
		sub := reBr.FindStringSubmatch(m)
		return `<a class="footnote-backref" href="#fnref:` + sub[1] + `" title="Jump back to footnote ` + sub[1] + ` in the text">&#8617;</a>`
	})
	s = regexp.MustCompile(`</blockquote>\s*\n\s*<blockquote>`).ReplaceAllString(s, "\n")
	reBQ := regexp.MustCompile(`(?s)<blockquote>(.*?)</blockquote>`)
	s = reBQ.ReplaceAllStringFunc(s, func(m string) string {
		sub := reBQ.FindStringSubmatch(m)
		body := regexp.MustCompile(`</p>\n\s*\n<p>`).ReplaceAllString(sub[1], "</p>\n<p>")
		return "<blockquote>" + body + "</blockquote>"
	})
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

func mdToHTML(text string) string {
	re1 := regexp.MustCompile(`\{\{<\s*youtube\s+"([^"]+)"\s*>\}\}`)
	text = re1.ReplaceAllStringFunc(text, func(m string) string {
		sub := re1.FindStringSubmatch(m)
		return "\n\nhttps://www.youtube.com/watch?v=" + sub[1] + "\n\n"
	})
	re2 := regexp.MustCompile(`(?m)^https://(?:www\.)?youtube\.com/watch\?v=([\w-]{6,})[ \t]*$`)
	text = re2.ReplaceAllStringFunc(text, func(m string) string {
		sub := re2.FindStringSubmatch(m)
		return "\n\n<iframe class=\"video\" src=\"https://www.youtube.com/embed/" + sub[1] + "\" title=\"YouTube video\" allowfullscreen loading=\"lazy\"></iframe>\n\n"
	})
	re3 := regexp.MustCompile(`~~(.+?)~~`)
	text = re3.ReplaceAllString(text, `<del>$1</del>`)

	var buf strings.Builder
	if err := mdConverter.Convert([]byte(text), &buf); err != nil {
		panic(err)
	}
	rendered := buf.String()
	rendered = strings.TrimRight(rendered, "\n")
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
	fields := strings.Fields(text)
	return strings.Join(fields, " ")
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

// ---------- loading ----------

func loadNote(path string) *note {
	raw, _ := os.ReadFile(path)
	meta, body := splitFrontMatter(string(raw))
	if _, ok := meta["date"]; !ok {
		fmt.Println("missing date", path)
		os.Exit(1)
	}
	if _, ok := meta["summary"]; !ok {
		fmt.Println("missing summary", path)
		os.Exit(1)
	}
	return &note{Path: path, Slug: filepath.Base(path)[:len(filepath.Base(path))-len(filepath.Ext(path))], Meta: meta, Body: body}
}

func loadSection(path, url string) *Section {
	sec := &Section{Path: path, Slug: filepath.Base(path), URL: url, Meta: map[string]any{}}
	index := filepath.Join(path, "_index.md")
	if b, err := os.ReadFile(index); err == nil {
		m, _ := splitFrontMatter(string(b))
		sec.Meta = m
	}
	entries, _ := os.ReadDir(path)
	type fi struct {
		name string
		isDir bool
	}
	var items []fi
	for _, e := range entries {
		items = append(items, fi{e.Name(), e.IsDir()})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].name) < strings.ToLower(items[j].name) })
	sawNote := false
	for _, it := range items {
		if strings.HasPrefix(it.name, ".") || strings.HasPrefix(it.name, "_") {
			continue
		}
		full := filepath.Join(path, it.name)
		if it.isDir {
			sec.Children = append(sec.Children, loadSection(full, url+it.name+"/"))
		} else if strings.HasSuffix(it.name, ".md") {
			sawNote = true
			n := loadNote(full)
			if isDraft(n.Meta) {
				continue
			}
			sec.Notes = append(sec.Notes, n)
		}
	}
	sec.HadDraftsOnly = sawNote && len(sec.Notes) == 0
	reverse := !sec.SortOldest()
	sort.Slice(sec.Notes, func(i, j int) bool {
		a, b := sortKey(sec.Notes[i].Meta), sortKey(sec.Notes[j].Meta)
		if reverse {
			return a > b
		}
		return a < b
	})
	sort.Slice(sec.Children, func(i, j int) bool {
		if sec.Children[i].Order() != sec.Children[j].Order() {
			return sec.Children[i].Order() < sec.Children[j].Order()
		}
		return strings.ToLower(sec.Children[i].Title()) < strings.ToLower(sec.Children[j].Title())
	})
	for _, n := range sec.Notes {
		n.URL = url + n.Slug + "/"
		n.HTML = mdToHTML(n.Body)
	}
	return sec
}

func loadPage(path, url string) *page {
	raw, _ := os.ReadFile(path)
	meta, body := splitFrontMatter(string(raw))
	p := &page{URL: url, Title: "", Meta: meta, Body: body}
	if v, ok := meta["title"]; ok {
		p.Title = fmt.Sprintf("%v", v)
	} else {
		p.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	p.HTML = mdToHTML(body)
	return p
}

// ---------- nav ----------

func navEntries(section *Section) []struct {
	order int
	kind  string
} {
	// We'll use parallel slices instead; implement render directly.
	return nil
}

func navList(section *Section) []interface{} {
	groups := map[string][]*Section{}
	var loose []*Section
	for _, c := range section.Children {
		if c.Group() != "" {
			groups[c.Group()] = append(groups[c.Group()], c)
		} else {
			loose = append(loose, c)
		}
	}
	type entry struct {
		order   int
		kind    string
		payload any
	}
	var entries []entry
	for _, c := range loose {
		entries = append(entries, entry{c.Order(), "section", c})
	}
	for name, members := range groups {
		mem := members
		sort.Slice(mem, func(i, j int) bool {
			if mem[i].Order() != mem[j].Order() {
				return mem[i].Order() < mem[j].Order()
			}
			return strings.ToLower(mem[i].Title()) < strings.ToLower(mem[j].Title())
		})
		minO := mem[0].Order()
		for _, m := range mem {
			if m.Order() < minO {
				minO = m.Order()
			}
		}
		entries = append(entries, entry{minO, "group", struct {
			name    string
			members []*Section
		}{name, mem}})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].order != entries[j].order {
			return entries[i].order < entries[j].order
		}
		return entries[i].kind < entries[j].kind
	})
	out := make([]interface{}, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out
}

// helper to read entry
type navEntry struct {
	order   int
	kind    string
	payload any
}

func navEntries2(section *Section) []navEntry {
	groups := map[string][]*Section{}
	var loose []*Section
	for _, c := range section.Children {
		if c.Group() != "" {
			groups[c.Group()] = append(groups[c.Group()], c)
		} else {
			loose = append(loose, c)
		}
	}
	var entries []navEntry
	for _, c := range loose {
		entries = append(entries, navEntry{c.Order(), "section", c})
	}
	for name, members := range groups {
		mem := append([]*Section(nil), members...)
		sort.Slice(mem, func(i, j int) bool {
			if mem[i].Order() != mem[j].Order() {
				return mem[i].Order() < mem[j].Order()
			}
			return strings.ToLower(mem[i].Title()) < strings.ToLower(mem[j].Title())
		})
		minO := mem[0].Order()
		for _, m := range mem {
			if m.Order() < minO {
				minO = m.Order()
			}
		}
		entries = append(entries, navEntry{minO, "group", struct {
			name    string
			members []*Section
		}{name, mem}})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].order != entries[j].order {
			return entries[i].order < entries[j].order
		}
		return entries[i].kind < entries[j].kind
	})
	return entries
}

func renderSectionNav(section *Section, current string) string {
	openAttr := " open"
	currentAttr := ""
	if strings.TrimRight(current, "/") == strings.TrimRight(section.URL, "/") {
		currentAttr = ` aria-current="page"`
	}
	parts := []string{
		fmt.Sprintf(`<details data-nav="%s" %s>`, esc(section.URL), openAttr),
		fmt.Sprintf(`<summary><a href="%s"%s>%s</a></summary>`, section.URL, currentAttr, esc(section.Title())),
		"<ul>",
	}
	for _, e := range navEntries2(section) {
		if e.kind == "section" {
			child := e.payload.(*Section)
			if child.Children != nil || len(child.Notes) > 0 || child.Placeholder() {
				if len(child.Children) > 0 {
					parts = append(parts, "<li>"+renderSectionNav(child, current)+"</li>")
				} else {
					attr := ""
					if strings.HasPrefix(current, child.URL) {
						attr = ` aria-current="page"`
					}
					parts = append(parts, fmt.Sprintf(`<li class="topic"><a href="%s"%s>%s</a></li>`, child.URL, attr, esc(child.Title())))
				}
			}
		} else {
			g := e.payload.(struct {
				name    string
				members []*Section
			})
			groupURL := section.URL + slugify(g.name) + "/"
			groupOpen := " open"
			groupCurrent := ""
			if strings.TrimRight(current, "/") == strings.TrimRight(groupURL, "/") {
				groupCurrent = ` aria-current="page"`
			}
			parts = append(parts, fmt.Sprintf(`<li><details class="group" data-nav="%s"%s><summary><a href="%s"%s>%s</a></summary><ul>`,
				esc(groupURL), groupOpen, groupURL, groupCurrent, esc(g.name)))
			for _, member := range g.members {
				attr := ""
				if strings.HasPrefix(current, member.URL) {
					attr = ` aria-current="page"`
				}
				parts = append(parts, fmt.Sprintf(`<li class="topic"><a href="%s"%s>%s</a></li>`, member.URL, attr, esc(member.Title())))
			}
			parts = append(parts, "</ul></details></li>")
		}
	}
	parts = append(parts, "</ul></details>")
	return strings.Join(parts, "")
}

func sidebarHTML(sections []*Section, pages []*page, current string) string {
	homeLinks := []string{}
	for _, p := range pages {
		attr := ""
		if (strings.TrimRight(current, "/") == strings.TrimRight(p.URL, "/")) || (p.URL == "/" && current == "/") {
			attr = ` aria-current="page"`
		}
		label := p.Title
		if p.URL == "/" {
			label = "Home"
		}
		homeLinks = append(homeLinks, fmt.Sprintf(`<a href="%s"%s>%s</a>`, p.URL, attr, esc(label)))
	}
	blocks := []string{}
	for _, s := range sections {
		blocks = append(blocks, renderSectionNav(s, current))
	}
	return `<aside class="sidebar-drawer" id="site-nav"><nav class="sidebar" aria-label="Sections"><div class="nav-home">` + strings.Join(homeLinks, "") + `</div>` + strings.Join(blocks, "") + `</nav></aside>`
}

func crumbs(items []struct {
	href  string
	label string
}) string {
	// use parallel
	return ""
}

// crumbs with tuples
func crumbsT(items [][2]string) string {
	bits := []string{}
	for i, it := range items {
		if i > 0 {
			bits = append(bits, " / ")
		}
		if i == len(items)-1 {
			bits = append(bits, esc(it[1]))
		} else {
			bits = append(bits, fmt.Sprintf(`<a href="%s">%s</a>`, it[0], esc(it[1])))
		}
	}
	if len(items) == 0 {
		return ""
	}
	return fmt.Sprintf(`<p class="crumbs">%s</p>`, strings.Join(bits, ""))
}

func sourceLine(n *note) string {
	bits := []string{}
	if v, ok := n.Meta["source"]; ok && v != nil {
		bits = append(bits, fmt.Sprintf("Source: %v", v))
	}
	if fmt.Sprintf("%v", n.Meta["date_confidence"]) == "approximate" {
		bits = append(bits, "date approximate")
	}
	if len(bits) == 0 {
		return ""
	}
	return fmt.Sprintf(`<p class="source">%s</p>`, esc(strings.Join(bits, " · ")))
}

func layout(site map[string]any, title, current, body string, sections []*Section, pages []*page, description string) string {
	desc := esc(description)
	if description == "" {
		desc = esc(fmt.Sprintf("%v", site["description"]))
	}
	fullTitle := title
	if title != fmt.Sprintf("%v", site["title"]) {
		fullTitle = title + " · " + fmt.Sprintf("%v", site["title"])
	}
	lang := "en"
	if v, ok := site["language"]; ok {
		lang = fmt.Sprintf("%v", v)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<meta name="description" content="%s">
<link rel="stylesheet" href="/assets/site.css">
</head>
<body>
<a class="skip" href="#content">Skip to content</a>
<header class="topbar">
  <a class="brand" href="/">%s<small>%s</small></a>
  <div class="top-tools">
    <button type="button" class="menu-toggle" id="menu" aria-expanded="false" aria-controls="site-nav">Menu</button>
    <form class="search" action="/search/" method="get" role="search">
      <label class="visually-hidden" for="q">Search notes</label>
      <input id="q" name="q" type="search" placeholder="Search notes" autocomplete="off">
      <div class="search-results" hidden></div>
    </form>
    <button type="button" class="theme" id="theme">Dark</button>
  </div>
</header>
<div class="shell">
  %s
  <main class="main" id="content">
    %s
  </main>
</div>
<footer class="site">%s · %s</footer>
<script src="/assets/site.js"></script>
</body>
</html>
`,
		esc(lang),
		esc(fullTitle),
		desc,
		esc(fmt.Sprintf("%v", site["title"])),
		esc(fmt.Sprintf("%v", site["tagline"])),
		sidebarHTML(sections, pages, current),
		body,
		esc(fmt.Sprintf("%v", site["title"])),
		esc(fmt.Sprintf("%v", site["tagline"])),
	)
}

func writePage(out, url, htmlText string) {
	dest := filepath.Join(out, "index.html")
	if url != "/" {
		dest = filepath.Join(out, strings.Trim(url, "/"), "index.html")
	}
	os.MkdirAll(filepath.Dir(dest), 0755)
	os.WriteFile(dest, []byte(htmlText), 0644)
}

func eraHead(n *note, nameTag string) string {
	title := esc(n.Title())
	if n.Title() == "" {
		title = "Note"
	}
	dated := fmt.Sprintf(`<span class="when">%s</span>`, esc(n.When()))
	if n.Summary() != "" {
		dated += " – " + esc(n.Summary())
	}
	return fmt.Sprintf(`<div class="era-head"><%s class="era-name">%s</%s><p class="era-sub">%s</p></div>`, nameTag, title, nameTag, dated)
}

func renderNoteBlock(n *note, newest bool) string {
	link := fmt.Sprintf(`<p class="permalink"><a href="%s">Permalink</a></p>`, n.URL)
	meta := sourceLine(n)
	if newest {
		return fmt.Sprintf(`<article class="era era-current" id="%s"><p class="era-kicker">Current view</p>%s%s<div class="prose">%s</div>%s</article>`,
			esc(n.Slug), eraHead(n, "h2"), meta, n.HTML, link)
	}
	return fmt.Sprintf(`<details class="era" id="%s"><summary>%s</summary>%s<div class="prose">%s</div>%s</details>`,
		esc(n.Slug), eraHead(n, "span"), meta, n.HTML, link)
}

// ---------- main ----------

func esc(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "&#39;", "&#x27;")
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "build" {
		fmt.Println("usage: rc build")
		os.Exit(1)
	}
	initMD()

	root, _ := os.Getwd()
	// find repo root by site.yaml
	for {
		if _, err := os.Stat(filepath.Join(root, "site.yaml")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}

	siteRaw, _ := os.ReadFile(filepath.Join(root, "site.yaml"))
	site := parseYAML(string(siteRaw))

	out := filepath.Join(root, fmt.Sprintf("%v", site["output_dir"]))
	if _, err := os.Stat(out); err == nil {
		os.RemoveAll(out)
	}
	os.MkdirAll(out, 0755)

	contentDir := filepath.Join(root, "content")
	mediaDir := filepath.Join(root, "media")
	assetsDir := filepath.Join(root, "assets")

	home := loadPage(filepath.Join(contentDir, "home.md"), "/")
	about := loadPage(filepath.Join(contentDir, "about-me.md"), "/about-me/")
	pages := []*page{home}
	if !isDraft(about.Meta) {
		pages = append(pages, about)
	}

	var sections []*Section
	entries, _ := os.ReadDir(contentDir)
	var dirNames []string
	for _, e := range entries {
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
		}
	}
	sort.Slice(dirNames, func(i, j int) bool { return strings.ToLower(dirNames[i]) < strings.ToLower(dirNames[j]) })
	for _, name := range dirNames {
		p := filepath.Join(contentDir, name)
		if _, err := os.Stat(filepath.Join(p, "_index.md")); err == nil {
			sections = append(sections, loadSection(p, "/"+name+"/"))
		}
	}
	// prune
	var prune func(s *Section) bool
	prune = func(s *Section) bool {
		s.Children = func() []*Section {
			var out []*Section
			for _, c := range s.Children {
				if prune(c) {
					out = append(out, c)
				}
			}
			return out
		}()
		visible := len(s.Notes) > 0 || len(s.Children) > 0
		if s.Placeholder() && !s.HadDraftsOnly {
			visible = true
		}
		return visible
	}
	var pruned []*Section
	for _, s := range sections {
		if prune(s) {
			pruned = append(pruned, s)
		}
	}
	sections = pruned
	sort.Slice(sections, func(i, j int) bool {
		if sections[i].Order() != sections[j].Order() {
			return sections[i].Order() < sections[j].Order()
		}
		return strings.ToLower(sections[i].Title()) < strings.ToLower(sections[j].Title())
	})

	// attach bodies
	var attach func(s *Section)
	attach = func(s *Section) {
		if b, err := os.ReadFile(filepath.Join(s.Path, "_index.md")); err == nil {
			_, body := splitFrontMatter(string(b))
			if strings.TrimSpace(body) != "" {
				s.Meta["body_html"] = mdToHTML(body)
			} else {
				s.Meta["body_html"] = ""
			}
		}
		for _, c := range s.Children {
			attach(c)
		}
	}
	for _, s := range sections {
		attach(s)
	}

	// thinking
	thinkingRaw, _ := os.ReadFile(filepath.Join(root, "data", "thinking.yaml"))
	thinking := parseYAML(string(thinkingRaw))
	table := thinkingHTML(thinking)

	search := []map[string]any{}

	writePage(out, "/", renderHome(site, home, sections, pages, table))
	if !isDraft(about.Meta) {
		writePage(out, "/about-me/", renderSimple(site, about, sections, pages))
		search = append(search, map[string]any{
			"title": about.Title, "topic": "About", "category": "About",
			"when": "", "summary": "", "url": "/about-me/",
			"text": plainText(about.HTML),
		})
	}
	search = append(search, map[string]any{
		"title": "Home", "topic": "Home", "category": "Home",
		"when": "", "summary": plainText(inlineMD(fmt.Sprintf("%v", thinking["intro"]))),
		"url": "/", "text": plainText(pages[0].HTML + " " + table),
	})

	for _, s := range sections {
		walkPublish(site, s, [][2]string{}, sections, pages, out, &search)
	}

	// search page
	searchPage := layout(site, "Search", "/search/",
		`<article id="search-page"><h1>Search</h1><div id="search-results"></div></article>`,
		sections, pages, "Search notes")
	writePage(out, "/search/", searchPage)

	notFound := layout(site, "Not found", "/404.html",
		"<article><h1>Not found</h1><p>That page is not here. Try the search box.</p></article>",
		sections, pages, "")
	os.WriteFile(filepath.Join(out, "404.html"), []byte(notFound), 0644)

	// search index
	b, _ := json.Marshal(search)
	// ensure_ascii false: use encoder
	_ = b
	jb, _ := json.Marshal(search)
	os.WriteFile(filepath.Join(out, "search-index.json"), jb, 0644)

	published := publishedURLs(sections)
	for _, p := range pages {
		if p.URL != "/" {
			u := p.URL
			if !strings.HasSuffix(u, "/") {
				u += "/"
			}
			published[u] = true
		}
	}
	redirects := redirectLines(published)
	os.WriteFile(filepath.Join(out, "_redirects"), []byte(strings.Join(redirects, "\n")+"\n"), 0644)

	// assets + media
	copyDir(assetsDir, filepath.Join(out, "assets"))
	if _, err := os.Stat(mediaDir); err == nil {
		copyDir(mediaDir, filepath.Join(out, "media"))
	}

	fmt.Printf("Wrote %s (%d searchable notes)\n", out, len(search))
}

// ---------- render helpers ----------

func thinkingHTML(rowsDoc map[string]any) string {
	intro := inlineMD(fmt.Sprintf("%v", rowsDoc["intro"]))
	head := "<tr><th><span class='visually-hidden'>Area</span></th><th>THEN</th><th>LATER</th><th>NOW</th></tr>"
	body := []string{}
	for _, r := range rowsDoc["rows"].([]any) {
		row := r.(map[string]any)
		body = append(body, fmt.Sprintf("<tr><th scope=\"row\"><a href=\"%s\">%s</a></th><td data-label=\"THEN\">%s</td><td data-label=\"LATER\">%s</td><td data-label=\"NOW\">%s</td></tr>",
			esc(fmt.Sprintf("%v", row["href"])),
			esc(fmt.Sprintf("%v", row["area"])),
			inlineMD(fmt.Sprintf("%v", row["then"])),
			inlineMD(fmt.Sprintf("%v", row["later"])),
			inlineMD(fmt.Sprintf("%v", row["now"])),
		))
	}
	return `<section class="thinking" aria-label="How my thinking changed"><h2>How my thinking changed</h2>` +
		fmt.Sprintf(`<p class="thinking-intro">%s</p>`, intro) +
		fmt.Sprintf(`<table><thead>%s</thead><tbody>%s</tbody></table></section>`, head, strings.Join(body, ""))
}

func renderTopic(site map[string]any, section *Section, ancestors [][2]string, sections []*Section, pages []*page) string {
	image := ""
	if v, ok := section.Meta["image"]; ok && v != nil {
		image = fmt.Sprintf(`<p><img class="hero" src="/media/%s" alt=""></p>`, esc(fmt.Sprintf("%v", v)))
	}
	trail := append([][2]string{}, ancestors...)
	trail = append(trail, [2]string{section.URL, section.Title()})
	if section.Placeholder() && len(section.Notes) == 0 {
		bodyHTML := ""
		if v, ok := section.Meta["body_html"]; ok && v != nil {
			bodyHTML = fmt.Sprintf("%v", v)
		}
		inner := fmt.Sprintf("%s<h1>%s</h1><div class=\"prose\">%s</div><p class=\"coming\">Coming soon</p>",
			crumbsT(trail), esc(section.Title()), bodyHTML)
		blurb := ""
		if v, ok := section.Meta["summary"]; ok && v != nil {
			blurb = fmt.Sprintf("%v", v)
		}
		return layout(site, section.Title(), section.URL, inner, sections, pages, blurb)
	}
	eras := []string{}
	for i, n := range section.Notes {
		eras = append(eras, renderNoteBlock(n, i == 0))
	}
	erasStr := strings.Join(eras, "")
	if len(section.Notes) == 0 {
		erasStr = `<p class="coming">Coming soon</p>`
	}
	inner := fmt.Sprintf("%s<h1>%s</h1>%s%s", crumbsT(trail), esc(section.Title()), image, erasStr)
	blurb := ""
	if len(section.Notes) > 0 {
		blurb = section.Notes[0].Summary()
	} else if v, ok := section.Meta["summary"]; ok && v != nil {
		blurb = fmt.Sprintf("%v", v)
	}
	return layout(site, section.Title(), section.URL, inner, sections, pages, blurb)
}

func renderIndex(site map[string]any, section *Section, ancestors [][2]string, sections []*Section, pages []*page) string {
	trail := append([][2]string{}, ancestors...)
	trail = append(trail, [2]string{section.URL, section.Title()})
	items := []string{}
	for _, e := range navEntries2(section) {
		if e.kind == "section" {
			child := e.payload.(*Section)
			summary := ""
			if v, ok := child.Meta["summary"]; ok && v != nil {
				summary = fmt.Sprintf("%v", v)
			}
			if summary == "" && len(child.Notes) > 0 {
				summary = child.Notes[0].Summary()
			}
			if summary == "" && child.Placeholder() {
				summary = "Coming soon"
			}
			extra := ""
			if summary != "" {
				extra = fmt.Sprintf("<p>%s</p>", esc(summary))
			}
			items = append(items, fmt.Sprintf(`<li><a href="%s">%s</a>%s</li>`, child.URL, esc(child.Title()), extra))
		} else {
			g := e.payload.(struct {
				name    string
				members []*Section
			})
			groupURL := section.URL + slugify(g.name) + "/"
			names := []string{}
			for _, m := range g.members {
				names = append(names, m.Title())
			}
			items = append(items, fmt.Sprintf(`<li><a href="%s">%s</a><p>%s</p></li>`, groupURL, esc(g.name), esc(strings.Join(names, ", "))))
		}
	}
	intro := ""
	if v, ok := section.Meta["summary"]; ok && v != nil {
		intro = fmt.Sprintf(`<div class="prose"><p>%s</p></div>`, esc(fmt.Sprintf("%v", v)))
	}
	inner := fmt.Sprintf("%s<h1>%s</h1>%s<ul class=\"card-list\">%s</ul>", crumbsT(trail), esc(section.Title()), intro, strings.Join(items, ""))
	desc := ""
	if v, ok := section.Meta["summary"]; ok && v != nil {
		desc = fmt.Sprintf("%v", v)
	}
	return layout(site, section.Title(), section.URL, inner, sections, pages, desc)
}

func renderGroup(site map[string]any, parent *Section, name string, members []*Section, ancestors [][2]string, sections []*Section, pages []*page) string {
	url := parent.URL + slugify(name) + "/"
	trail := append([][2]string{}, ancestors...)
	trail = append(trail, [2]string{parent.URL, parent.Title()}, [2]string{url, name})
	items := []string{}
	for _, m := range members {
		summary := ""
		if len(m.Notes) > 0 {
			summary = m.Notes[0].Summary()
		}
		items = append(items, fmt.Sprintf(`<li><a href="%s">%s</a><p>%s</p></li>`, m.URL, esc(m.Title()), esc(summary)))
	}
	inner := fmt.Sprintf("%s<h1>%s</h1><ul class=\"card-list\">%s</ul>", crumbsT(trail), esc(name), strings.Join(items, ""))
	return layout(site, name, url, inner, sections, pages, "")
}

func renderHome(site map[string]any, page *page, sections []*Section, pages []*page, table string) string {
	inner := fmt.Sprintf(`<article class="home"><h1 class="visually-hidden">%s</h1><div class="prose">%s</div>%s</article>`,
		esc(page.Title), page.HTML, table)
	return layout(site, fmt.Sprintf("%v", site["title"]), "/", inner, sections, pages, fmt.Sprintf("%v", site["description"]))
}

func renderSimple(site map[string]any, page *page, sections []*Section, pages []*page) string {
	inner := fmt.Sprintf("<article><h1>%s</h1><div class=\"prose\">%s</div></article>", esc(page.Title), page.HTML)
	return layout(site, page.Title, page.URL, inner, sections, pages, "")
}

func walkPublish(site map[string]any, section *Section, ancestors [][2]string, sections []*Section, pages []*page, out string, search *[]map[string]any) {
	if len(section.Children) > 0 {
		writePage(out, section.URL, renderIndex(site, section, ancestors, sections, pages))
	} else if len(section.Notes) > 0 || section.Placeholder() {
		writePage(out, section.URL, renderTopic(site, section, ancestors, sections, pages))
	}
	for _, e := range navEntries2(section) {
		if e.kind == "group" {
			g := e.payload.(struct {
				name    string
				members []*Section
			})
			url := section.URL + slugify(g.name) + "/"
			writePage(out, url, renderGroup(site, section, g.name, g.members, ancestors, sections, pages))
		}
	}
	for _, n := range section.Notes {
		trail := append([][2]string{}, ancestors...)
		trail = append(trail, [2]string{section.URL, section.Title()}, [2]string{n.URL, n.When()})
		others := []string{}
		for _, other := range section.Notes {
			if other.URL != n.URL {
				others = append(others, fmt.Sprintf(`<li><a href="%s">%s</a></li>`, other.URL, esc(other.Heading())))
			}
		}
		inner := fmt.Sprintf("%s<h1>%s</h1><p class=\"era-meta\">%s%s</p>%s<div class=\"prose\">%s</div><p class=\"permalink\"><a href=\"%s\">All eras of %s</a></p><ul class=\"card-list\">%s</ul>",
			crumbsT(trail),
			esc(func() string {
				if v, ok := n.Meta["title"]; ok && v != nil {
					return fmt.Sprintf("%v", v)
				}
				return section.Title()
			}()),
			esc(n.When()),
			func() string {
				if n.Summary() != "" {
					return " – " + esc(n.Summary())
				}
				return ""
			}(),
			sourceLine(n),
			n.HTML,
			section.URL,
			esc(section.Title()),
			strings.Join(others, ""),
		)
		writePage(out, n.URL, layout(site, func() string {
			if v, ok := n.Meta["title"]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
			return section.Title()
		}(), n.URL, inner, sections, pages, n.Summary()))
		*search = append(*search, map[string]any{
			"title": func() string {
				if v, ok := n.Meta["title"]; ok && v != nil {
					return fmt.Sprintf("%v", v)
				}
				return section.Title()
			}(),
			"topic":    section.Title(),
			"category": func() string {
				if len(ancestors) > 0 {
					return ancestors[len(ancestors)-1][1]
				}
				return section.Title()
			}(),
			"when":    n.When(),
			"summary": n.Summary(),
			"url":     n.URL,
			"text":    plainText(n.HTML + " " + n.Summary() + " " + func() string {
				if v, ok := n.Meta["title"]; ok && v != nil {
					return fmt.Sprintf("%v", v)
				}
				return ""
			}()),
		})
	}
	for _, c := range section.Children {
		walkPublish(site, c, append([][2]string{}, append(ancestors, [2]string{section.URL, section.Title()})...), sections, pages, out, search)
	}
}

func sectionIsVisible(s *Section) bool {
	if len(s.Notes) > 0 || len(s.Children) > 0 {
		return true
	}
	if s.Placeholder() && !s.HadDraftsOnly {
		return true
	}
	return false
}

func publishedURLs(sections []*Section) map[string]bool {
	urls := map[string]bool{"/": true, "/search/": true}
	var walk func(s *Section)
	walk = func(s *Section) {
		urls[s.URL] = true
		for _, n := range s.Notes {
			urls[n.URL] = true
		}
		for _, e := range navEntries2(s) {
			if e.kind == "group" {
				g := e.payload.(struct {
					name    string
					members []*Section
				})
				if len(g.members) > 0 {
					urls[s.URL+slugify(g.name)+"/"] = true
				}
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	for _, s := range sections {
		walk(s)
	}
	return urls
}

func redirectLines(published map[string]bool) []string {
	lines := map[string]string{}
	add := func(src, dest string) {
		if !strings.HasPrefix(src, "/") {
			src = "/" + src
		}
		if !strings.HasSuffix(src, "/") {
			src = src + "/"
		}
		if strings.TrimRight(src, "/") == strings.TrimRight(dest, "/") {
			return
		}
		lines[src] = dest
		bare := strings.TrimRight(src, "/")
		if bare != "" && bare != strings.TrimRight(dest, "/") {
			lines[bare] = dest
		}
	}
	oldUrls := map[string]string{
		"/science/Apocalypse-Mirage/":            "/science/apocalypse-mirage/",
		"/science/Consensus-as-Religion/":         "/science/consensus-as-religion/",
		"/science/Coral-Reefs/":                  "/science/coral-reefs/",
		"/science/COVID-19/":                     "/science/covid-19/",
		"/science/Disappearing-Birds/":            "/science/disappearing-birds/",
		"/science/Insect-Armaggedon/":            "/science/insect-armageddon/",
		"/science/Losing-My-Faith-in-Science/":    "/science/losing-my-faith-in-science/",
		"/science/Monarch-Butterflies/":           "/science/monarch-butterflies/",
		"/science/Sea-Levels/":                   "/science/sea-levels/",
		"/science/Speculative-Extinctions/":       "/science/speculative-extinctions/",
		"/science/Temperatures/":                 "/science/temperatures/",
		"/science/The-Replication-Crisis/":        "/science/the-replication-crisis/",
		"/politics/2020-Elections/":               "/politics/2020-elections/",
		"/politics/Elections/":                   "/politics/elections/",
		"/religion/An-Atheists-Love-for-Keith-Green/": "/religion/an-atheists-love-for-keith-green/",
		"/religion/Deconversion/":                "/religion/deconversion/",
		"/miscellaneous/My-Career-In-Desktop-Support/": "/miscellaneous/my-career-in-desktop-support/",
		"/miscellaneous/Twenty-Years-of-TheDixons.net/": "/miscellaneous/twenty-years-of-thedixons-net/",
		"/About-Me/":                             "/about-me/",
		"/categories/":                           "/",
		"/tags/":                                 "/",
		"/Science/":                              "/science/",
		"/Politics/":                             "/politics/",
		"/Religion/":                             "/religion/",
		"/Miscellaneous/":                        "/miscellaneous/",
	}
	categories := []string{"science", "politics", "religion", "miscellaneous"}
	for src, dest := range oldUrls {
		variants := map[string]bool{src: true, strings.ToLower(src): true}
		parts := []string{}
		for _, p := range strings.Split(strings.Trim(src, "/"), "/") {
			if p != "" {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			low := strings.ToLower(parts[0])
			for _, c := range categories {
				if low == c {
					cap := strings.ToUpper(parts[0][:1]) + parts[0][1:]
					variants["/"+strings.Join(append([]string{cap}, parts[1:]...), "/")+"/"] = true
					lowerRest := []string{cap}
					for _, p := range parts[1:] {
						lowerRest = append(lowerRest, strings.ToLower(p))
					}
					variants["/"+strings.Join(lowerRest, "/")+"/"] = true
				}
			}
		}
		target := dest
		if !strings.HasSuffix(target, "/") {
			target += "/"
		}
		if !published[target] {
			continue
		}
		for v := range variants {
			add(v, dest)
		}
	}
	keys := make([]string, 0, len(lines))
	for k := range lines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []string{}
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s %s 301", k, lines[k]))
	}
	return out
}

func copyDir(src, dst string) {
	os.MkdirAll(dst, 0755)
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			os.MkdirAll(target, 0755)
			return nil
		}
		data, _ := os.ReadFile(path)
		os.WriteFile(target, data, info.Mode())
		return nil
	})
}
