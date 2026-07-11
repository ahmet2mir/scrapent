# scrapent

`scrapent` is a small command-line tool that archives blog posts and their
images from an [ENT / entcore](https://github.com/edificeio/entcore) school
space (for example `ent-ecoles.ac-xxxxxxxxxx.fr`).

For each blog it downloads every published post as `content.json` and saves the
full-resolution images referenced in the post.

## Features

- Authenticates against an ENT space and reuses the session for all requests.
- Downloads every published post of one or more blogs.
- Extracts and downloads images embedded in each post.
- Generates a clean PDF of each article (pure Go, no external tools).
- Resumable: an article that already has a `content.json` on disk is not
  fetched again, and an image or PDF already present is not regenerated.
- Human-readable output tree named after the blog and each post's title.

## Installation

Build from source (Go 1.24+):

```bash
go build -o bin/scrapent ./cmd/scrapent
```

Or download a prebuilt binary for Linux, macOS or Windows from the releases
page.

## Configuration

Settings come from subcommand flags or environment variables. A local `.env`
file is loaded automatically if present. All flags are subcommand-local, so they
go after the subcommand name.

Credentials are flags of the authenticating subcommands (`blog list` and
`blog get`); they can also come from the environment:

| Flag              | Environment variable | Required | Description                                        |
|-------------------|----------------------|----------|----------------------------------------------------|
| `--login`         | `SCRAPENT_LOGIN`     | yes      | ENT account login                                  |
| `--password`      | `SCRAPENT_PASSWORD`  | yes      | ENT account password                               |
| `--domain`        | `SCRAPENT_DOMAIN`    | yes      | ENT domain, e.g. `ent-ecoles.ac-xxxxxxxxxx.fr`     |

`blog get` and `blog generate` also take `--blog-dir` (`-d`, `SCRAPENT_BLOG_DIR`,
default `dist`) for the directory blogs are stored in and read from.

Example `.env`:

```dotenv
SCRAPENT_LOGIN="jane.doe"
SCRAPENT_PASSWORD="s3cret"
SCRAPENT_DOMAIN="ent-ecoles.ac-xxxxxxxxxx.fr"
SCRAPENT_BLOG_DIR="dist"
```

## Usage

### List available blogs

```bash
./bin/scrapent blog list            # styled bullet list (default)
./bin/scrapent blog list -f json    # machine-readable JSON
```

`name` is the slugified blog title (the same slug used for directories); copy a
`name:id` pair straight into `blog get --id`. `--format` (`-f`) accepts
`terminal` (default) or `json`. Colors are dropped automatically when the output
is piped.

### Download blogs

```bash
# one or more blogs (repeat --id)
./bin/scrapent blog get \
  --id class-a:3695f372-778d-yyyy-zzzzz-xxxxxxxxxxxxxxxxxx \
  --id class-b:28c48eea-568a-yyyy-zzzzz-xxxxxxxxxxxxxxxxxx
```

A bare id with no `:` is used as its own folder name.

Per-resource flags control what is fetched. `--force-*` re-downloads or
regenerates even when the target already exists; `--skip-*` skips that kind
entirely, present or not:

| Flag                | Effect                                             |
|---------------------|----------------------------------------------------|
| `--force-articles`  | re-download `content.json` even if present         |
| `--skip-articles`   | never fetch articles (use existing `content.json`) |
| `--force-images`    | re-download images even if present                 |
| `--skip-images`     | never download images                              |
| `--force-pdf`       | regenerate article PDFs (and the merge) even if present |
| `--skip-pdf`        | never generate or merge PDFs                        |

### Regenerate PDFs offline

Rebuild article and merged PDFs from an already-downloaded data directory,
without authentication or network access:

```bash
./bin/scrapent blog generate -d dist            # every blog under dist/
./bin/scrapent blog generate -d dist \
  --name class-a --name class-b                  # only these blog directories
./bin/scrapent blog generate -d dist --force     # rebuild even existing PDFs
```

It reads each `content.json` (and the images next to it), regenerates any
missing or invalid article PDF, and re-merges each blog's `<blog>.pdf`. Login,
password and domain are not required for this command.

### Migrate a legacy content.json

Older exports store the post body as raw HTML with millisecond dates and no
structured `jsonContent`. `migrate` converts one such file to the current
format (RFC3339 dates, HTML parsed into text and `custom-image` nodes):

```bash
./bin/scrapent blog migrate path/to/content.json        # print converted JSON
./bin/scrapent blog migrate -i path/to/content.json      # rewrite the file in place
```

No authentication is required. Combine with `blog generate` to rebuild PDFs
from a migrated data directory.

## Output layout

```
dist/
└── class-a/                              # blog name (from --id name:id)
    ├── .id                               # blog id
    ├── class-a.pdf                       # all articles merged, oldest to newest
    └── 2025-12-05-la-rentree/            # <post date>-<slugified title>
        ├── .id                           # article id
        ├── content.json                  # full post payload
        ├── 2025-12-05-la-rentree.pdf     # rendered article (title, text, images)
        └── 2025-12-05-08-30-00--<img>.jpg
```

A PDF of each article is generated (pure Go, no external tools) with its title,
date, author, text and images. Like the rest of the output it is resumable: an
existing article PDF is not regenerated.

All article PDFs of a blog are then
merged into a single `<blog>.pdf`, ordered by the `created` date (oldest first).

Post titles are slugified for the directory name: accents are transliterated
(`é` -> `e`), spaces become dashes, the name is lowercased, and any other
character (punctuation, emoji, ...) is dropped.

## Development

A `Makefile` wraps the quality gate:

```bash
make fmt tidy lint security test build
```

- `test` runs the unit tests.
- `lint` runs golangci-lint, `security` runs gosec.
- `build` cross-compiles binaries with goreleaser into `build/`.
