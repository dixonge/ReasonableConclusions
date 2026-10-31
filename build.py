#!/usr/bin/env python3
"""Build the static site.

Reads content/ and writes plain HTML into public/ (or site.yaml output_dir).
The only dependency is the Markdown package. Run: python3 build.py
"""

from __future__ import annotations

import html
import json
import re
import shutil
import sys
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path

try:
    import markdown
except ImportError:
    sys.exit("Missing the markdown package. Run: pip install -r requirements.txt")

ROOT = Path(__file__).resolve().parent
CONTENT = ROOT / "content"
MEDIA = ROOT / "media"
ASSETS = ROOT / "assets"

# Live URLs on the Hugo site, and where they should land.
# Mixed-case paths, their lowercase twins, and a capitalised category
# (Science/…) are all written into public/_redirects.
OLD_URLS = {
    "/science/Apocalypse-Mirage/": "/science/apocalypse-mirage/",
    "/science/Consensus-as-Religion/": "/science/consensus-as-religion/",
    "/science/Coral-Reefs/": "/science/coral-reefs/",
    "/science/COVID-19/": "/science/covid-19/",
    "/science/Disappearing-Birds/": "/science/disappearing-birds/",
    "/science/Insect-Armaggedon/": "/science/insect-armageddon/",
    "/science/Losing-My-Faith-in-Science/": "/science/losing-my-faith-in-science/",
    "/science/Monarch-Butterflies/": "/science/monarch-butterflies/",
    "/science/Sea-Levels/": "/science/sea-levels/",
    "/science/Speculative-Extinctions/": "/science/speculative-extinctions/",
    "/science/Temperatures/": "/science/temperatures/",
    "/science/The-Replication-Crisis/": "/science/the-replication-crisis/",
    "/politics/2020-Elections/": "/politics/2020-elections/",
    "/politics/Elections/": "/politics/elections/",
    "/religion/An-Atheists-Love-for-Keith-Green/": "/religion/an-atheists-love-for-keith-green/",
    "/religion/Deconversion/": "/religion/deconversion/",
    "/miscellaneous/My-Career-In-Desktop-Support/": "/miscellaneous/my-career-in-desktop-support/",
    "/miscellaneous/Twenty-Years-of-TheDixons.net/": "/miscellaneous/twenty-years-of-thedixons-net/",
    "/About-Me/": "/about-me/",
    "/categories/": "/",
    "/tags/": "/",
    "/Science/": "/science/",
    "/Politics/": "/politics/",
    "/Religion/": "/religion/",
    "/Miscellaneous/": "/miscellaneous/",
}

MONTHS = (
    "January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December",
)


def parse_scalar(raw: str):
    value = raw.strip()
    if value in ("true", "false"):
        return value == "true"
    if re.fullmatch(r"-?\d+", value):
        return int(value)
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1].replace('\\"', '"').replace("\\'", "'")
    return value


def parse_yaml(text: str) -> dict:
    """Small YAML subset: scalars, and one level of lists of mappings."""
    root: dict = {}
    list_key = None
    item = None
    for raw in text.splitlines():
        if not raw.strip() or raw.strip().startswith("#"):
            continue
        if raw.startswith("  - "):
            if list_key is None:
                raise SystemExit(f"List item outside a list: {raw}")
            item = {}
            root[list_key].append(item)
            rest = raw[4:]
            if rest.strip():
                key, value = rest.split(":", 1)
                item[key.strip()] = parse_scalar(value)
            continue
        if raw.startswith("    ") and item is not None:
            key, value = raw.strip().split(":", 1)
            item[key.strip()] = parse_scalar(value)
            continue
        item = None
        if raw.endswith(":") and " " not in raw.strip()[:-1]:
            list_key = raw.strip()[:-1]
            root[list_key] = []
            continue
        list_key = None
        key, value = raw.split(":", 1)
        root[key.strip()] = parse_scalar(value)
    return root


def split_front_matter(text: str) -> tuple[dict, str]:
    if text.startswith("---"):
        parts = text.split("---", 2)
        if len(parts) == 3:
            return parse_yaml(parts[1]), parts[2].lstrip("\n")
    return {}, text


def parse_when(value: str) -> tuple[date, str]:
    parts = str(value).split("-")
    year = int(parts[0])
    if len(parts) == 1:
        return date(year, 1, 1), "year"
    month = int(parts[1])
    if len(parts) == 2:
        return date(year, month, 1), "month"
    return date(year, month, int(parts[2])), "day"


def format_when(value: str, precision: str | None = None) -> str:
    parsed, detected = parse_when(value)
    precision = precision or detected
    if precision == "year":
        return str(parsed.year)
    if precision == "month":
        return f"{MONTHS[parsed.month - 1]} {parsed.year}"
    return f"{MONTHS[parsed.month - 1]} {parsed.day}, {parsed.year}"


def era_when(meta: dict) -> str:
    if meta.get("label"):
        return str(meta["label"])
    start = format_when(str(meta["date"]))
    if meta.get("date_end"):
        end = format_when(str(meta["date_end"]))
        if start != end:
            # "March 2020" + "April 2020" -> "March–April 2020" when the year matches.
            if start.endswith(end[-4:]) and start[-4:] == end[-4:]:
                return f"{start[:-5]}–{end}"
            return f"{start} – {end}"
    return start


def sort_key(meta: dict) -> date:
    raw = str(meta.get("date_end") or meta["date"])
    parsed, _ = parse_when(raw)
    return parsed


def slugify(name: str) -> str:
    return name.strip().lower().replace(" ", "-")


def md_to_html(text: str) -> str:
    text = re.sub(
        r"\{\{<\s*youtube\s+\"([^\"]+)\"\s*>\}\}",
        lambda m: f"\n\nhttps://www.youtube.com/watch?v={m.group(1)}\n\n",
        text,
    )
    text = re.sub(
        r"(?m)^https://(?:www\.)?youtube\.com/watch\?v=([\w-]{6,})\s*$",
        lambda m: (
            f'<iframe class="video" src="https://www.youtube.com/embed/{m.group(1)}"'
            f' title="YouTube video" allowfullscreen loading="lazy"></iframe>'
        ),
        text,
    )
    text = re.sub(r"~~(.+?)~~", r"<del>\1</del>", text)
    converter = markdown.Markdown(extensions=["extra", "sane_lists"], output_format="html")
    return converter.convert(text)


def inline_md(text: str) -> str:
    rendered = md_to_html(text).strip()
    return re.sub(r"^<p>|</p>$", "", rendered)


def plain_text(html_text: str) -> str:
    text = re.sub(r"<script[\s\S]*?</script>", " ", html_text)
    text = re.sub(r"<[^>]+>", " ", text)
    text = html.unescape(text)
    return re.sub(r"\s+", " ", text).strip()


@dataclass
class Note:
    path: Path
    slug: str
    meta: dict
    body: str
    html: str = ""
    url: str = ""

    @property
    def when(self) -> str:
        return era_when(self.meta)

    @property
    def summary(self) -> str:
        return str(self.meta.get("summary") or "")

    @property
    def heading(self) -> str:
        return f"{self.when} – {self.summary}" if self.summary else self.when


@dataclass
class Section:
    path: Path
    slug: str
    url: str
    meta: dict
    notes: list[Note] = field(default_factory=list)
    children: list[Section] = field(default_factory=list)

    @property
    def title(self) -> str:
        return str(self.meta.get("title") or self.slug.replace("-", " ").title())

    @property
    def order(self) -> int:
        return int(self.meta.get("order") or 100)
    @property
    def group(self) -> str:
        return str(self.meta.get("group") or "")

    @property
    def placeholder(self) -> bool:
        return bool(self.meta.get("placeholder"))

    @property
    def sort_oldest(self) -> bool:
        return str(self.meta.get("sort") or "") == "oldest"


@dataclass
class Page:
    url: str
    title: str
    meta: dict
    body: str
    html: str = ""


def load_note(path: Path) -> Note:
    meta, body = split_front_matter(path.read_text(encoding="utf-8"))
    if "date" not in meta:
        raise SystemExit(f"{path} is missing date")
    if "summary" not in meta:
        raise SystemExit(f"{path} is missing summary")
    return Note(path=path, slug=path.stem, meta=meta, body=body)


def load_section(path: Path, url: str) -> Section:
    index = path / "_index.md"
    meta = {}
    if index.exists():
        meta, _ = split_front_matter(index.read_text(encoding="utf-8"))
    section = Section(path=path, slug=path.name, url=url, meta=meta)
    for child in sorted(path.iterdir(), key=lambda p: p.name.lower()):
        if child.name.startswith(".") or child.name.startswith("_"):
            continue
        if child.is_dir():
            section.children.append(load_section(child, url + child.name + "/"))
        elif child.suffix == ".md":
            section.notes.append(load_note(child))
    reverse = not section.sort_oldest
    section.notes.sort(key=lambda n: sort_key(n.meta), reverse=reverse)
    section.children.sort(key=lambda s: (s.order, s.title.lower()))
    for note in section.notes:
        note.url = url + note.slug + "/"
        note.html = md_to_html(note.body)
    return section


def load_page(path: Path, url: str) -> Page:
    meta, body = split_front_matter(path.read_text(encoding="utf-8"))
    page = Page(url=url, title=str(meta.get("title") or path.stem), meta=meta, body=body)
    page.html = md_to_html(body)
    return page


def nav_entries(section: Section):
    groups: dict[str, list[Section]] = {}
    loose: list[Section] = []
    for child in section.children:
        if child.group:
            groups.setdefault(child.group, []).append(child)
        else:
            loose.append(child)
    entries = [(child.order, "section", child) for child in loose]
    for name, members in groups.items():
        members.sort(key=lambda s: (s.order, s.title.lower()))
        entries.append((min(m.order for m in members), "group", (name, members)))
    entries.sort(key=lambda item: (item[0], item[1]))
    return entries


def render_section_nav(section: Section, current: str) -> str:
    open_attr = " open"
    current_attr = ' aria-current="page"' if current.rstrip("/") == section.url.rstrip("/") else ""
    parts = [
        f'<details data-nav="{html.escape(section.url)}" {open_attr}>',
        f'<summary><a href="{section.url}"{current_attr}>{html.escape(section.title)}</a></summary>',
        "<ul>",
    ]
    for _order, kind, payload in nav_entries(section):
        if kind == "section":
            child = payload
            if child.children or child.notes or child.placeholder:
                if child.children:
                    parts.append(f"<li>{render_section_nav(child, current)}</li>")
                else:
                    attr = ' aria-current="page"' if current.startswith(child.url) else ""
                    parts.append(
                        f'<li class="topic"><a href="{child.url}"{attr}>{html.escape(child.title)}</a></li>'
                    )
        else:
            name, members = payload
            group_url = section.url + slugify(name) + "/"
            group_open = " open"
            group_current = ' aria-current="page"' if current.rstrip("/") == group_url.rstrip("/") else ""
            parts.append(
                f'<li><details class="group" data-nav="{html.escape(group_url)}"{group_open}>'
                f'<summary><a href="{group_url}"{group_current}>{html.escape(name)}</a></summary><ul>'
            )
            for member in members:
                attr = ' aria-current="page"' if current.startswith(member.url) else ""
                parts.append(f'<li class="topic"><a href="{member.url}"{attr}>{html.escape(member.title)}</a></li>')
            parts.append("</ul></details></li>")
    parts.append("</ul></details>")
    return "".join(parts)


def sidebar_html(sections: list[Section], pages: list[Page], current: str) -> str:
    home_links = []
    for page in pages:
        attr = ' aria-current="page"' if current.rstrip("/") == page.url.rstrip("/") or (page.url == "/" and current == "/") else ""
        home_links.append(f'<a href="{page.url}"{attr}>{html.escape(page.title if page.url != "/" else "Home")}</a>')
    blocks = [render_section_nav(section, current) for section in sections]
    return (
        '<aside class="sidebar-drawer" id="site-nav">'
        f'<nav class="sidebar" aria-label="Sections"><div class="nav-home">{"".join(home_links)}</div>'
        f'{"".join(blocks)}</nav></aside>'
    )


def crumbs(items: list[tuple[str, str]]) -> str:
    bits = []
    for i, (href, label) in enumerate(items):
        if i:
            bits.append(" / ")
        if i == len(items) - 1:
            bits.append(html.escape(label))
        else:
            bits.append(f'<a href="{href}">{html.escape(label)}</a>')
    return f'<p class="crumbs">{"".join(bits)}</p>' if items else ""


def source_line(note: Note) -> str:
    bits = []
    if note.meta.get("source"):
        bits.append(f"Source: {note.meta['source']}")
    if str(note.meta.get("date_confidence") or "") == "approximate":
        bits.append("date approximate")
    if not bits:
        return ""
    return f'<p class="source">{html.escape(" · ".join(bits))}</p>'


def layout(site: dict, title: str, current: str, body: str, sections, pages, description: str = "") -> str:
    desc = html.escape(description or str(site.get("description") or ""))
    full = html.escape(title if title == site["title"] else f"{title} · {site['title']}")
    return f"""<!DOCTYPE html>
<html lang="{html.escape(str(site.get('language') or 'en'))}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{full}</title>
<meta name="description" content="{desc}">
<link rel="stylesheet" href="/assets/site.css">
</head>
<body>
<a class="skip" href="#content">Skip to content</a>
<header class="topbar">
  <a class="brand" href="/">{html.escape(str(site['title']))}<small>{html.escape(str(site.get('tagline') or ''))}</small></a>
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
  {sidebar_html(sections, pages, current)}
  <main class="main" id="content">
    {body}
  </main>
</div>
<footer class="site">{html.escape(str(site['title']))} · {html.escape(str(site.get('tagline') or ''))}</footer>
<script src="/assets/site.js"></script>
</body>
</html>
"""


def write_page(out: Path, url: str, html_text: str) -> None:
    dest = out / "index.html" if url == "/" else out / url.strip("/") / "index.html"
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(html_text, encoding="utf-8")


def render_note_block(note: Note, newest: bool) -> str:
    link = f'<p class="permalink"><a href="{note.url}">Permalink</a></p>'
    meta = source_line(note)
    if newest:
        return (
            f'<article class="era era-current" id="{html.escape(note.slug)}">'
            f'<p class="era-kicker">Current view</p>'
            f"<h2>{html.escape(note.heading)}</h2>{meta}"
            f'<div class="prose">{note.html}</div>{link}</article>'
        )
    return (
        f'<details class="era" id="{html.escape(note.slug)}">'
        f"<summary><span class=\"when\">{html.escape(note.when)}</span>"
        f" – {html.escape(note.summary)}</summary>"
        f"{meta}<div class=\"prose\">{note.html}</div>{link}</details>"
    )


def render_topic(site, section: Section, ancestors, sections, pages) -> str:
    image = ""
    if section.meta.get("image"):
        image = f'<p><img class="hero" src="/media/{html.escape(str(section.meta["image"]))}" alt=""></p>'
    trail = ancestors + [(section.url, section.title)]
    if section.placeholder and not section.notes:
        prose = md_to_html(section.meta.get("_body") or "")
        # body stored separately
        body_html = section.meta.get("body_html") or ""
        inner = (
            f"{crumbs(trail)}<h1>{html.escape(section.title)}</h1>"
            f'<div class="prose">{body_html}</div><p class="coming">Coming soon</p>'
        )
        return layout(site, section.title, section.url, inner, sections, pages, str(section.meta.get("summary") or ""))
    eras = "".join(render_note_block(note, i == 0) for i, note in enumerate(section.notes))
    if not section.notes:
        eras = '<p class="coming">Coming soon</p>'
    inner = f"{crumbs(trail)}<h1>{html.escape(section.title)}</h1>{image}{eras}"
    blurb = section.notes[0].summary if section.notes else str(section.meta.get("summary") or "")
    return layout(site, section.title, section.url, inner, sections, pages, blurb)


def render_index(site, section: Section, ancestors, sections, pages) -> str:
    trail = ancestors + [(section.url, section.title)]
    items = []
    for _order, kind, payload in nav_entries(section):
        if kind == "section":
            child = payload
            summary = str(child.meta.get("summary") or "")
            if not summary and child.notes:
                summary = child.notes[0].summary
            if not summary and child.placeholder:
                summary = "Coming soon"
            extra = f"<p>{html.escape(summary)}</p>" if summary else ""
            items.append(
                f"<li><a href=\"{child.url}\">{html.escape(child.title)}</a>{extra}</li>"
            )
        else:
            name, members = payload
            group_url = section.url + slugify(name) + "/"
            names = ", ".join(m.title for m in members)
            items.append(
                f"<li><a href=\"{group_url}\">{html.escape(name)}</a>"
                f"<p>{html.escape(names)}</p></li>"
            )
    intro = f'<div class="prose"><p>{html.escape(str(section.meta["summary"]))}</p></div>' if section.meta.get("summary") else ""
    inner = (
        f"{crumbs(trail)}<h1>{html.escape(section.title)}</h1>{intro}"
        f'<ul class="card-list">{"".join(items)}</ul>'
    )
    return layout(site, section.title, section.url, inner, sections, pages, str(section.meta.get("summary") or ""))


def render_group(site, parent: Section, name: str, members: list[Section], ancestors, sections, pages) -> str:
    url = parent.url + slugify(name) + "/"
    trail = ancestors + [(parent.url, parent.title), (url, name)]
    items = []
    for member in members:
        summary = member.notes[0].summary if member.notes else ""
        items.append(
            f"<li><a href=\"{member.url}\">{html.escape(member.title)}</a>"
            f"<p>{html.escape(summary)}</p></li>"
        )
    inner = f"{crumbs(trail)}<h1>{html.escape(name)}</h1><ul class=\"card-list\">{''.join(items)}</ul>"
    return layout(site, name, url, inner, sections, pages)


def thinking_html(rows_doc: dict) -> str:
    intro = inline_md(str(rows_doc["intro"]))
    head = (
        "<tr><th><span class='visually-hidden'>Area</span></th>"
        "<th>THEN</th><th>LATER</th><th>NOW</th></tr>"
    )
    body = []
    for row in rows_doc["rows"]:
        body.append(
            "<tr>"
            f'<th scope="row"><a href="{html.escape(str(row["href"]))}">{html.escape(str(row["area"]))}</a></th>'
            f'<td data-label="THEN">{inline_md(str(row["then"]))}</td>'
            f'<td data-label="LATER">{inline_md(str(row["later"]))}</td>'
            f'<td data-label="NOW">{inline_md(str(row["now"]))}</td>'
            "</tr>"
        )
    return (
        '<section class="thinking" aria-label="How my thinking changed">'
        "<h2>How my thinking changed</h2>"
        f'<p class="thinking-intro">{intro}</p>'
        f"<table><thead>{head}</thead><tbody>{''.join(body)}</tbody></table>"
        "</section>"
    )


def render_home(site, page: Page, sections, pages, table: str) -> str:
    inner = f'<article class="home"><h1 class="visually-hidden">{html.escape(page.title)}</h1><div class="prose">{page.html}</div>{table}</article>'
    return layout(site, str(site["title"]), "/", inner, sections, pages, str(site.get("description") or ""))


def render_simple(site, page: Page, sections, pages) -> str:
    inner = f"<article><h1>{html.escape(page.title)}</h1><div class=\"prose\">{page.html}</div></article>"
    return layout(site, page.title, page.url, inner, sections, pages)


def walk_publish(site, section: Section, ancestors, sections, pages, out: Path, search: list) -> None:
    if section.children:
        write_page(out, section.url, render_index(site, section, ancestors, sections, pages))
    elif section.notes or section.placeholder:
        write_page(out, section.url, render_topic(site, section, ancestors, sections, pages))
    for _order, kind, payload in nav_entries(section):
        if kind == "group":
            name, members = payload
            url = section.url + slugify(name) + "/"
            write_page(out, url, render_group(site, section, name, members, ancestors, sections, pages))
    for note in section.notes:
        trail = ancestors + [(section.url, section.title), (note.url, note.when)]
        others = "".join(
            f'<li><a href="{other.url}">{html.escape(other.heading)}</a></li>'
            for other in section.notes
            if other.url != note.url
        )
        inner = (
            f"{crumbs(trail)}<h1>{html.escape(note.meta.get('title') or section.title)}</h1>"
            f"<p class=\"era-meta\">{html.escape(note.heading)}</p>{source_line(note)}"
            f'<div class="prose">{note.html}</div>'
            f'<p class="permalink"><a href="{section.url}">All eras of {html.escape(section.title)}</a></p>'
            f"<ul class=\"card-list\">{others}</ul>"
        )
        write_page(
            out,
            note.url,
            layout(site, str(note.meta.get("title") or section.title), note.url, inner, sections, pages, note.summary),
        )
        search.append({
            "title": str(note.meta.get("title") or section.title),
            "topic": section.title,
            "category": ancestors[-1][1] if ancestors else section.title,
            "when": note.when,
            "summary": note.summary,
            "url": note.url,
            "text": plain_text(note.html + " " + note.summary + " " + str(note.meta.get("title") or "")),
        })
    for child in section.children:
        walk_publish(site, child, ancestors + [(section.url, section.title)], sections, pages, out, search)


def redirect_lines() -> list[str]:
    lines = {}

    def add(src: str, dest: str) -> None:
        if not src.startswith("/"):
            src = "/" + src
        if not src.endswith("/"):
            src = src + "/"
        if src.rstrip("/") == dest.rstrip("/"):
            return
        lines[src] = dest
        bare = src.rstrip("/")
        if bare and bare != dest.rstrip("/"):
            lines[bare] = dest

    categories = ("science", "politics", "religion", "miscellaneous")
    for src, dest in OLD_URLS.items():
        variants = {src, src.lower()}
        parts = [p for p in src.strip("/").split("/") if p]
        if parts and parts[0].lower() in categories:
            cap = parts[0][:1].upper() + parts[0][1:]
            variants.add("/" + "/".join([cap, *parts[1:]]) + "/")
            variants.add("/" + "/".join([cap, *[p.lower() for p in parts[1:]]]) + "/")
        for variant in variants:
            add(variant, dest)
    ordered = sorted(lines)
    return [f"{src} {lines[src]} 301" for src in ordered]


def attach_bodies(section: Section) -> None:
    index = section.path / "_index.md"
    if index.exists():
        _meta, body = split_front_matter(index.read_text(encoding="utf-8"))
        section.meta["body_html"] = md_to_html(body) if body.strip() else ""
    for child in section.children:
        attach_bodies(child)


def main() -> None:
    site = parse_yaml((ROOT / "site.yaml").read_text(encoding="utf-8"))
    out = ROOT / str(site.get("output_dir") or "public")
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)

    pages = [
        load_page(CONTENT / "home.md", "/"),
        load_page(CONTENT / "about-me.md", "/about-me/"),
    ]
    sections = []
    for child in sorted(CONTENT.iterdir(), key=lambda p: p.name.lower()):
        if child.is_dir() and (child / "_index.md").exists():
            sections.append(load_section(child, f"/{child.name}/"))
    sections.sort(key=lambda s: (s.order, s.title.lower()))
    for section in sections:
        attach_bodies(section)

    thinking = parse_yaml((ROOT / "data" / "thinking.yaml").read_text(encoding="utf-8"))
    table = thinking_html(thinking)
    search: list[dict] = []

    write_page(out, "/", render_home(site, pages[0], sections, pages, table))
    write_page(out, "/about-me/", render_simple(site, pages[1], sections, pages))
    search.append({
        "title": pages[1].title,
        "topic": "About",
        "category": "About",
        "when": "",
        "summary": "",
        "url": "/about-me/",
        "text": plain_text(pages[1].html),
    })
    search.append({
        "title": "Home",
        "topic": "Home",
        "category": "Home",
        "when": "",
        "summary": plain_text(inline_md(str(thinking["intro"]))),
        "url": "/",
        "text": plain_text(pages[0].html + " " + table),
    })

    for section in sections:
        walk_publish(site, section, [], sections, pages, out, search)

    search_page = layout(
        site,
        "Search",
        "/search/",
        '<article id="search-page"><h1>Search</h1><div id="search-results"></div></article>',
        sections,
        pages,
        "Search notes",
    )
    write_page(out, "/search/", search_page)
    not_found = layout(
        site,
        "Not found",
        "/404.html",
        "<article><h1>Not found</h1><p>That page is not here. Try the search box.</p></article>",
        sections,
        pages,
    )
    (out / "404.html").write_text(not_found, encoding="utf-8")

    (out / "search-index.json").write_text(json.dumps(search, ensure_ascii=False), encoding="utf-8")
    (out / "_redirects").write_text("\n".join(redirect_lines()) + "\n", encoding="utf-8")
    shutil.copytree(ASSETS, out / "assets")
    if MEDIA.exists():
        shutil.copytree(MEDIA, out / "media")
        for path in MEDIA.rglob("*"):
            if path.is_file() and path.stat().st_size >= 500_000:
                print(f"oversized media: {path.relative_to(ROOT)} ({path.stat().st_size} bytes)")

    print(f"Wrote {out} ({len(search)} searchable notes)")


if __name__ == "__main__":
    main()
