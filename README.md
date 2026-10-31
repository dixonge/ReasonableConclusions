# Reasonable Conclusions

Glenn Dixon's notes, as plain HTML generated from Markdown. This is not a blog. Each topic is a stack of dated notes. The newest note is the current view. Older notes stay on the page, folded, and each one has its own link.

The same build can later run another site. Change `site.yaml` (title, domain) and the folders under `content/`. A folder can be as deep as you need: category, then topic, then notes. Or country, state, city, article. Or a surname and the ancestors under it.

## Add a note

1. Pick the topic folder, for example `content/science/covid-19/`.
2. Add one Markdown file. The name should start with the date: `2026-11-02-a-new-take.md`.
3. Put this at the top of the file:

```markdown
---
title: "COVID-19"
date: "2026-11-02"
summary: "One sentence about what you thought then."
source: "ReasonableConclusions"
---

The note itself, in your own words.
```

`date` can be a year (`2003`), a month (`2020-04`), or a full day (`2022-04-17`). If the date is a guess, add `date_confidence: "approximate"`. `summary` is the one line that shows on the folded older notes.

Add `draft: true` while a note is not ready. A draft is left out of the site: no page, no line on the topic, no sidebar link, no search hit, and no redirect to it. Leave the line out, or set `draft: false`, when you want it published. If every note in a topic is a draft, that topic is hidden too.

To start a new topic, make a folder with an `_index.md`:

```markdown
---
title: "A new topic"
order: 12
---
```

`order` sets the sidebar position. `group: "Climate"` tucks the topic under a group inside its category. `placeholder: true` shows "Coming soon" until a note exists. `sort: "oldest"` lists notes oldest-first (useful later for ancestors).

The home-page belief table is `data/thinking.yaml`. Edit the words there. There are no dates in that file.

Pictures go in `media/`. In a note, link them as `![](/media/your-file.jpg)`.

## Build

```bash
pip install -r requirements.txt
python3 build.py
```

That writes the site into `public/`. To look at it on your machine:

```bash
python3 -m http.server -d public
```

Then open http://localhost:8000/

## Cloudflare Pages

Do not deploy until Glenn says so. When he does, the Pages project settings are:

- Build command: `pip install -r requirements.txt && python3 build.py`
- Output directory: `public`

The build writes `public/_redirects`, which sends the old Hugo addresses (including the mixed-case ones, the lowercase ones, and the misspelled `/science/Insect-Armaggedon/`) to the new pages.

## Large files in media/

These are over 500 KB. They are the originals, unchanged:

- `media/Global-trends-clinical-studies-ivermectin_COVID-19.pdf` (about 1.0 MB)
- `media/covid-19.jpeg` (about 925 KB)
- `media/sea-underwater-biology-fish-coral-coral-reef-reef-aquarium-habitat-ecosystem-fish-tank-natural-environment-marine-biology-coral-reef-fish-pomacentridae-981484.jpeg` (about 740 KB)
- `media/thedixons.net-registration.png` (about 532 KB)
