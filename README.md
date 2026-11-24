# respawn

Comic-book slide decks as code: write them in Markdown, present them with live terminals,
and run the room with polls, questions, breaks, exercises and a follow-along audience copy.

One binary, no runtime to install. Slides are Markdown files in a git repository, so you
review them, diff them and ship them like code.

- **Markdown with components**: cards, steps, flows, tables, bars, stats, code, commands,
  Mermaid diagrams, prompts, reveals, splash screens. Raw HTML when you need it.
- **Several decks from one source**: a short talk and a full workshop share the same slides;
  `decks:` and `variants:` decide what goes where.
- **Live terminals** on lab slides, in the presentation's workspace, with the slide's commands
  already in the shell history.
- **Presenter tools**: speaker notes window with previous and next slide, search, timers,
  coffee and lunch breaks, sticky notes, sound effects, a Halloween edition, PDF export
  (16:9, one slide per page, no margins, no print dialog).
- **The room**: `--audience` serves a read-only copy of the deck on your network. People join
  with a session code (or a QR code), follow your slide, answer polls, send questions, see
  breaks and exercises. They never get ahead of you, never see notes or terminals.
  `--tunnel` does the same through a Cloudflare Tunnel, for any network.

respawn started as the slide system of the [AI4Dev](https://ai4dev.dev) course and is a fork
of David Gageot's [demoit](https://github.com/dgageot/demoit).

## Install

```sh
brew tap matteocodogno/respawn https://github.com/matteocodogno/respawn
brew install matteocodogno/respawn/respawn
```

Update with `brew upgrade respawn`. With Go 1.25+ instead:
`go install github.com/matteocodogno/respawn/cmd/respawn@latest`.

The PDF export needs Chrome, Chromium, Brave or Edge (`RESPAWN_CHROME=/path/to/browser`
picks one by hand). `--tunnel` needs `cloudflared` (`brew install cloudflared`).

## Quick start

```sh
respawn new my-talk          # a copy of the showcase, to read and to change
cd my-talk
respawn serve --dev          # http://localhost:8888, reloads while you edit
```

Press `?` in the deck for every shortcut.

| Command | What it does |
| --- | --- |
| `respawn new <folder>` | a new presentation |
| `respawn serve [deck]` | present a deck (`--port`, `--dev`, `--audience`, `--tunnel`, `--resume`, `--session CODE`, `--live N`) |
| `respawn build [--out dir]` | build every deck and report errors with file and line; `--out` writes the result |
| `respawn pdf [deck] --from N --to M [--out file.pdf] [--mode text]` | a range of slides as a PDF (default: `~/Downloads`, named after the module). The default `exact` mode is a picture of each slide as the browser shows it; `text` is lighter, with selectable text, but the print engine draws some effects differently |
| `respawn version` | |

Every command works on the presentation in the current folder; `--dir` picks another one.

## A presentation

```
my-talk/
├── respawn.yaml        # settings
├── slides/             # *.md, read in name order: 00-cover.md, 01-basics.md...
├── assets/             # optional: images/, media/, or any built-in file to override
│   └── images/logo.png #   -> /images/logo.png
├── workspace/          # where the live terminals open
└── .respawn/           # session, polls and questions of the audience copy (gitignore it)
```

### respawn.yaml

```yaml
respawn: ">=0.1"                 # the respawn versions this presentation needs
title: My Talk
brand: "MY TALK // FIELD MANUAL" # footer of every slide
lang: en                         # <html lang>, clock format of the breaks
namespace: mytalk                # prefix of what the deck keeps in the browser

decks:                           # default: one deck called "main" on 8888
  - {name: talk, label: TALK, port: 8888}
  - {name: workshop, label: WORKSHOP, port: 8889}
# the audience copy of a deck is on its port + 2: keep decks two ports apart from each other's

cover:
  scribbles: ["slides are code", "commit before you present!"]
  press: "PRESS START →"

labs:
  kickers: '^\s*(lab|demo)'      # kickers of slides with a live terminal
  hands_on: '^\s*(exercise)'     # ...and these, when they show a command
  folder: workspace              # where the terminals open

catalog:
  kickers: '^\s*(recap|recall|conclusion)'  # slides that go to the exercise catalog ("x")
  primary: workshop              # when an exercise is in several decks, this version wins

footer: {intro: INTRO}           # footer chip before the first module
accents: [yellow, cyan, magenta, green, orange, purple, blue, pink]  # one per module
history: [git status]            # extra commands in the terminals' history
gif: media/party.gif             # "h" in the deck: a gif from assets/
strings:                         # text shown to the room, see below
  break.coffee.sub: Pausa caffè

tunnel: {name: mytalk-live, host: live.example.dev}
```

`strings` overrides the room-facing text of the runtime: `break.coffee.title`,
`break.coffee.sub`, `break.coffee.minutes`, `break.lunch.title`, `break.lunch.sub`,
`break.lunch.minutes`, `break.back` ("Back at"), `break.paused` ("Paused").

## Slides

A slide is a YAML header between `---` lines, then a Markdown body. The next `---` header
starts the next slide.

````markdown
---
title: Cards, steps, flows
kicker: Components_
decks: [workshop]          # only in these decks (default: all)
timer: 5 MIN               # a timer badge; click it (or "m") to start
notes: What you say. The notes window ("s") shows it.
---

Plain **Markdown** paragraphs and lists.

::: cards cols=3
- icon: bolt               # a Material Symbols name
  label: Fast
  title: Write in Markdown
  text: The slides build in milliseconds.
:::
````

### Header keys

| Key | Meaning |
| --- | --- |
| `title`, `kicker`, `sub`, `lead` | the slide's texts (Markdown inline) |
| `layout` | `content` (default), `cover`, `module` (a module opener: `num: MODULE 01_`), `part`, `statement` |
| `decks` | the decks that show this slide |
| `variants` | per-deck overrides: `variants: {workshop: {kicker: Workshop, body: "..."}}` |
| `center` | center the body |
| `timer` | timer badge |
| `term` | `true`/`false` to force the live terminal, or a folder |
| `catalog`, `exercise`, `generic` | exercise catalog: force in or out, group consecutive slides as steps of one exercise, or mark as fitting after any module |
| `sid` | stable id (sticky notes are keyed by it); default from the title |
| `notes` | speaker notes |

### Body blocks

- Paragraphs and `- ` / `1. ` lists, with `` `code` ``, `**bold**`, `*italic*`.
- Code: ```` ```kotlin title="Main.kt" size=sm scroll nolig ````.
- A command line: ```` ```cmd ```` (on a lab slide it also goes to the terminal history),
  ```` ```cmd noprompt ````.
- A diagram: ```` ```mermaid ````.
- Components: `::: name params` ... `:::`. The closing fence is the same number of colons,
  alone on its line. A component that holds components uses more colons (`:::: cols`).

| Component | Content |
| --- | --- |
| `p`, `lead`, `small`, `label` | text |
| `callout icon=... tone=...` | text; tones: `primary`, `secondary`, `tertiary`, `muted` |
| `prompt` | text in a prompt box |
| `reveal label="..."` | hidden until clicked |
| `stat big=3x tone=...` | a big number and its caption |
| `list big ordered` | YAML list of Markdown strings |
| `cards cols=3`, `grid cols=4`, `flow`, `steps` | YAML list of `{num, icon, label, title, text}` |
| `card` | YAML `{icon, label, title, text, body, tone, hl, plain, reveal}` |
| `kv` | YAML list of `[key, value]` |
| `table` | YAML `{headers: [...], rows: [[...]]}` |
| `bars max=100` | YAML list of `[label, value, shown]` |
| `img src=logo.png height=12` | YAML `{caption}`; the file is `assets/images/logo.png` |
| `splash icon=... word="LEVEL 2!"` | YAML `{caption, chips: [...]}` |
| `nexttoken` | YAML `{prompt, steps}`: next-token probabilities, step by step |
| `cols tpl="1fr 2fr"` | other blocks, columns split by `+++` |
| `raw` | HTML, as is |

Any inline value can be `{raw: "<b>html</b>"}`. Errors say file and line:
`respawn build` checks everything before you present.

## The room

```sh
respawn serve workshop --audience          # http://<your-ip>:8891 on the same network
respawn serve workshop --tunnel            # https://<tunnel.host> from anywhere
respawn serve workshop --tunnel --resume   # restart without losing the room
```

Press `j` for the QR code. Each start gets a new session code; `--resume` keeps the last
one and the room's slide. Polls and questions are kept in `.respawn/` until you clear them.
Only a browser on your own computer drives the room: requests from the network, and
everything that comes through the tunnel, can only read.

One-time tunnel setup (Cloudflare account, a domain on Cloudflare):

```sh
cloudflared tunnel login
cloudflared tunnel create mytalk-live
cloudflared tunnel route dns mytalk-live live.example.dev
```

then `tunnel: {name: mytalk-live, host: live.example.dev}` in `respawn.yaml`.

## Customizing the look

Everything in `assets/` of this repository (style, fonts, scripts, the notes and print pages)
is built into the binary. A file at the same path in a presentation's `assets/` replaces it,
for that presentation only.

## Development

```sh
go test ./...
go run ./cmd/respawn serve --dir examples/showcase --dev
```

Releases: push a `v*` tag. The release workflow builds macOS and Linux binaries, publishes
them, and updates `Formula/respawn.rb`.

## License

Apache 2.0. Contains code from [demoit](https://github.com/dgageot/demoit) by David Gageot
(Apache 2.0); see [NOTICE](NOTICE).
