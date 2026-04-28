# Generating Photos (`photogen`)

To resize photos and generate the JSON indexes, run `photogen`. Albums are
defined in a YAML config file (default: `config/albums.yaml`). See
[config/albums.example.yaml](../config/albums.example.yaml) for the full format.

The `settings:` block at the top of `albums.yaml` has several required fields that are
validated before any files are written:

```yaml
settings:
  id: my-site                                    # required; names the output directory
  site_name: "My Photo Albums"                   # required
  site_url: https://photos.example.com           # required
  site_description: "A collection of photos"     # required
  copyright_owner: "Your Name"                   # required
  copyright_year: 2020                           # required
  # allow_crawling: false                        # optional; default false
```

Album descriptions are in a TXT file (default: `config/descriptions.txt`).
See [config/descriptions.example.txt](../config/descriptions.example.txt)
for the format.

The `settings.id` field is required and determines the output directory name (e.g. `id: prod`
produces `albums/prod`). It must contain only lowercase letters, digits, and hyphens.
The `-site-id` flag overrides this, which is useful when generating an encrypted variant
alongside the standard output from the same config.

Output goes to `$DDPHOTOS_ALBUMS_DIR}/{id}` (git-ignored). `DDPHOTOS_ALBUMS_DIR` defaults
to `albums` at the repo root (from `config/defaults.env`). Override with the `-out` flag
or by setting `DDPHOTOS_ALBUMS_DIR` in the environment.

To run with defaults:

```bash
go run cmd/photogen/photogen.go -resize -index -clean -doit
```

To use a different albums file (e.g., a development subset):

```bash
go run cmd/photogen/photogen.go -albums albums-dev.yaml -resize -index -clean -doit
```

## Hero Image

An optional full-width banner image can be displayed at the top of the home page.
Add a `hero:` block under `settings:` in `albums.yaml`:

```yaml
settings:
  hero:
    image: my-banner.jpg   # filename; joined to 'base' if set, else relative to config dir
    base: drive            # optional — same base map as album entries
    crop: center           # top | center | bottom (default: center)
```

`photogen` hard-crops the source image to 1600×250px and writes it as `hero.jpg`
alongside `config.json` (generated when `-resize` is set). The hero is never
encrypted and takes priority as the `og:image` on the home page.

To regenerate the hero without reprocessing albums or rebuilding indexes, use
`-hero-only`. It always overwrites the existing `hero.jpg` regardless of `-force`:

```bash
go run cmd/photogen/photogen.go -hero-only -doit
```

## Custom CSS

To override site styles, add a `css:` entry under `settings:`:

```yaml
settings:
  css: custom.css   # filename relative to this config dir
```

`photogen` copies the file to the site output as `custom.css` (generated when
`-index` is set). The frontend injects it site-wide as a `<link>` after the
built-in styles, so any rules inside it take effect as normal cascade overrides.
Redefining CSS custom properties (e.g. `--bg-color`, `--text-color-2nd`) is the
cleanest approach — no specificity battles needed.

## CLI Flags

| Flag          | Default       | Description                                                                                    |
|---------------|---------------|------------------------------------------------------------------------------------------------|
| `-config-dir` | `config`      | Directory containing the albums YAML and descriptions files                                    |
| `-albums`     | `albums.yaml` | Albums YAML filename within `-config-dir`                                                      |
| `-doit`       | `false`       | Write files; without this, runs in dry-run mode                                                |
| `-resize`     | `false`       | Generate resized WebP image variants                                                           |
| `-index`      | `false`       | Generate JSON index files and sitemap.xml                                                      |
| `-out`        | *(from env)*  | Albums directory override (overrides `DDPHOTOS_ALBUMS_DIR`)                                    |
| `-limit N`    | `0` (all)     | Limit photos per album (useful during development)                                             |
| `-force`      | `false`       | Regenerate files even if they already exist                                                    |
| `-workers N`  | `0` (auto)    | Concurrent resize workers (auto = NumCPU/2, min 2)                                             |
| `-album`      | `""` (all)    | Comma-separated album slugs to process                                                         |
| `-site-url`   | *(from YAML)* | Sitemap base URL override (overrides `settings.site_url`)                                      |
| `-site-id`    | *(from YAML)* | Override `settings.id`; useful for generating multiple output sites from one config            |
| `-passwords`  | *(from YAML)* | Path to passwords file; overrides `settings.passwords` (see [Passwords File](#passwords-file)) |
| `-css`        | *(from YAML)* | Path to custom CSS file; overrides `settings.css` (see [Custom CSS](#custom-css))              |
| `-clean`      | `false`       | Remove stale files from processed album directories after a run (requires `-resize`)           |
| `-hero-only`  | `false`       | Regenerate the hero image only; skips all album processing and index/JSON generation           |

## Photo Descriptions (`photogen.txt`)

To add per-photo descriptions, create a `photogen.txt` file in the album's
source photo directory. One line per photo:

```
filename_without_extension Description text here.
# blank lines and lines starting with # are ignored
```

Example:

```
Patagonia-042 First view of Torres del Paine at sunrise.
Patagonia-107 Crossing the John Gardner Pass in the wind.
```

Descriptions are stored in `index.json` and used as:

- `alt` text on grid and lightbox images
- Hover caption overlay in the grid (desktop)
- Always-visible caption in the grid (mobile)
- Caption overlaid on the photo in the lightbox

To also use the file for **sort order** (instead of EXIF date), set
`manual_sort_order: true` on the album entry in `albums.yaml`. Photos not
listed in `photogen.txt` are sorted by date and appended at the end.

## Recursive Albums (`recurse: true`)

Set `recurse: true` on an album entry to collect photos from all subdirectories.
The output is flattened: each photo's ID and filename get a sanitized prefix
derived from its subdirectory path, preventing name collisions.

```
Craig's/img001.jpg      → ID: craigs_img001,       file: craigs_img001.jpg
Ski 2007/Alan's/a.jpg   → ID: ski2007_alans_a,     file: ski2007_alans_a.jpg
```

There are three modes depending on configuration:

| Mode          | Config                                                       | Behavior                                                                                        |
|---------------|--------------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| Off (default) | `recurse: false`                                             | Only photos in the album root directory are collected; subdirectories ignored                   |
| Auto sort     | `recurse: true`, no `photogen.txt`                           | Root photos date-sorted, then subdirectories processed alphabetically, each date-sorted         |
| Manual sort   | `recurse: true` + `manual_sort_order: true` + `photogen.txt` | Subfolder names in `photogen.txt` expand inline; photos and subfolder groups freely interleaved |

**Per-subfolder `photogen.txt`**: place a `photogen.txt` in any subfolder for captions
and (with `manual_sort_order: true`) local sort order within that folder. Entries use
the bare filename without prefix — photogen applies the prefix automatically.

**Controlling inter-folder order**: with `manual_sort_order: true`, a `photogen.txt`
at any level can reference subfolder names as placeholders. Subfolder entries expand
inline, so you can freely interleave root photos and subfolder groups:

```
# photogen.txt at album root
photo_a.jpg
Craig's
photo_b.jpg
Halstead
```

Subfolders not listed in `photogen.txt` are appended alphabetically at the end with
a warning. Photos not listed are date-sorted and appended at the end of their group
with a warning.

**Cover photo**: when `cover` is set on a recursive album, use the prefixed filename
(e.g. `cover: craigs_img001.jpg`). If omitted, the first collected photo is used.
The prefixed filename is in the `fileName` field of `index.json`. To find it from an
original filename, use `sourcePath` (see below) or grep the decoded index.

**Working example**: the sample Uganda album (`sample/source/uganda/`) uses `recurse: true`
with a `subfolder/` subdirectory. Its root `photogen.txt` uses `subfolder` as a placeholder
at the end to append those photos after the root-level ones. The album entry in
`sample/config/albums.yaml` shows the full configuration including `cover`, `manual_sort_order`,
and `recurse`.

**`sourcePath` field**: all photos include a `sourcePath` field in `index.json`
with their original relative path from the album source base directory (e.g. `"2008 - Big Sky/Craig's/img001.jpg"`).
This makes it easy to find the prefixed `fileName` for a given original file:

```bash
# plain album
grep -B2 "IMG_0436" albums/my-site/my-album/index.json

# encrypted album
go run cmd/decode/decode.go albums/my-site/my-album/index.enc.json | grep -B2 "IMG_0436"
```

## Passwords File

When a passwords file is present, `photogen` encrypts `albums.json` and each album's
`index.json` using AES-256-GCM (keys derived via PBKDF2-SHA256). The encrypted files
are written as `.enc.json` alongside their plaintext counterparts. `config.json` is
always written in plaintext — it contains only non-sensitive metadata (site ID, hints,
hero/CSS filenames) needed to bootstrap the frontend before any password is entered.

Custom HTML fields (`site_title_html`, `site_subtitle_html`, `site_overview_html`) can
contain private information such as links to private documents or contact details. When
a site password is configured, these fields are written to `html.enc.json` (encrypted). 
On unencrypted sites they are written to `html.json`
(plaintext). If none of the three fields are set, neither file is written. The frontend
fetches and decrypts `html.enc.json` as part of the same unlock step as `albums.enc.json`,
so there is no additional password prompt.

Decryption happens entirely in-browser using the Web Crypto API. Passwords are never
sent to a server.

The `key` field enables an additional layer of protection: WebP filenames for encrypted
albums are derived via HMAC-SHA256 rather than using the original filename, so the
actual photo files cannot be guessed even if someone knows the source filename.

A `SiteID` is written to `config.json` and used by the frontend to scope all
localStorage keys (stored passwords, cover image cache) to the current build. This
prevents stale data from a previous build bleeding through after a re-encryption with
new passwords or a different `key`.

Encryption is enabled by pointing photogen at a YAML passwords file, either via
`settings.passwords` in `albums.yaml` (filename relative to the config dir) or the
`-passwords` CLI flag (absolute or relative path; overrides `settings.passwords`).
Comments (lines starting with `#`) are ignored.

```yaml
key: hmac-secret

site:
  password: site-wide-password
  hint: Optional hint shown in the password dialog

albums:
  album-slug:
    password: per-album-password
    hint: Optional hint shown in the album password dialog
```

| Field                    | Description                                                                                                                                                       |
|--------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `key`                    | HMAC-SHA256 secret used to derive UUID-format WebP filenames for encrypted albums, preventing filename guessing (e.g. `IMG_3961.webp` becomes `3f8a1c2d-...webp`) |
| `site.password`          | Encrypts `albums.json` and all per-album `index.json` files site-wide                                                                                             |
| `site.hint`              | Optional hint shown in the site-wide password dialog (always visible, even before a password attempt)                                                             |
| `albums.<slug>.password` | Per-album password; encrypts only that album's `index.json`. Falls back to `site.password` if not set                                                             |
| `albums.<slug>.hint`     | Optional hint shown in that album's password dialog                                                                                                               |

Sample passwords files are in `sample/config/` — `passwords-all.yaml` (full site) and
`passwords-uganda.yaml` (single album). Both contain demo-only passwords and a prominent WARNING header.

**Do not commit real passwords.** Store production passwords outside the repo or in a
git-ignored directory (e.g. `.secrets/`).

### Frontend Behavior (Encrypted Sites)

When the frontend loads an encrypted page, it:

1. Reads `config.json` (always plaintext) to get the `siteId`, hints, and which albums
   file to load (`albums.json` vs `albums.enc.json`). If `htmlFile` is set, also fetches
   `html.json` (plaintext) or holds `html.enc.json` as a raw blob for later decryption.
2. Checks localStorage for a stored password scoped to the current `siteId`.
3. If a stored password decrypts successfully, both `albums.enc.json` and `html.enc.json`
   are decrypted in parallel — the page renders with all content in a single DOM update,
   with no flash.
4. If no stored password works, a full-screen `PasswordPrompt` overlay appears with a
   lock icon, a password input, and an optional hint. A wrong password triggers a shake
   animation; a correct one stores the password in localStorage and decrypts all content.

**Stored passwords and auto-unlock:** After a successful unlock, the password is saved
to localStorage so subsequent visits auto-decrypt without prompting. Append `?clear` to
any URL to clear all stored passwords and covers and return to the prompt:

```
http://localhost:5173/?clear
http://localhost:5173/albums/uganda?clear
```

**Cover flash prevention:** Album cover images are cached in localStorage after unlock.
An inline script in `app.html` runs synchronously before first paint, reading the cover
cache and setting CSS custom properties (`--ddp-cover-{slug}`) on `<html>`. This means
the cover image is visible from the very first paint with no flash, even before Svelte
hydrates.

**localStorage key format** (useful for debugging):

| Key                          | Contains                                      |
|------------------------------|-----------------------------------------------|
| `ddp_site_{siteId}`          | Site-wide password                            |
| `ddp_album_{siteId}_{slug}`  | Per-album password for `slug`                 |
| `ddp_cover_{siteId}_{slug}`  | Cached cover image URL for `slug`             |

All keys are scoped to `siteId` so that switching between builds (which use different
HMAC keys and produce different filenames) automatically invalidates stale cached data.

## Decoding Encrypted Files (`decode`)

The `decode` tool decrypts `.enc.json` files produced by `photogen` and prints the
plaintext JSON. Useful for inspecting what photogen wrote without running the full
frontend.

```bash
go run cmd/decode/decode.go <path.enc.json>
go run cmd/decode/decode.go -passwords <pw-file> <path.enc.json>
```

`photogen` embeds the passwords file path in every `.enc.json` it writes, so in most
cases no flags are needed:

```bash
go run cmd/decode/decode.go albums/sample-pw-uganda/uganda/index.enc.json
go run cmd/decode/decode.go albums/sample-pw-all/albums.enc.json
```

If the passwords file has moved, or the file was generated without an embedded path,
pass `-passwords` explicitly:

```bash
go run cmd/decode/decode.go -passwords sample/config/passwords-uganda.yaml \
  albums/sample-pw-uganda/uganda/index.enc.json
```

The correct password is selected automatically from the filename:

| File              | Password used                                    |
|-------------------|--------------------------------------------------|
| `albums.enc.json` | Site-wide password (`site.password`)             |
| `html.enc.json`   | Site-wide password (`site.password`)             |
| `index.enc.json`  | Per-album password for the parent directory slug |

## Finding a Cover Photo (`search_cover.sh`)

When browsing the site, and you want to set a photo as an album cover, you need its
`fileName` value for the `cover:` field in `albums.yaml`. The easiest way to get it is
to right-click the photo, copy the image URL, and pass it to `bin/search_cover.sh`:

```bash
bin/search_cover.sh <url>
```

The script parses the album slug and image path from the URL, locates the album's
`index.json` (or `index.enc.json` for encrypted albums — decoded automatically via
`cmd/decode`), and searches for the matching `src` entry to print the `fileName`, `id`,
and `sourcePath`.

The search is scoped to `DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID` (defaults from
`config/defaults.env`). Override to search a different site:

```bash
DDPHOTOS_SITE_ID=sample-pw-all bin/search_cover.sh http://localhost:5173/albums/uganda/full/1996ae71-5ada-d233-8f26-53e46fac4f64.webp```
```

Output:

```
Album:  uganda
Index:  /Users/donohoe/work/ddphotos/albums/sample-pw-all/uganda/index.enc.json
src:    full/1996ae71-5ada-d233-8f26-53e46fac4f64.webp

fileName: subfolder_img_840_d.jpg
id:       subfolder_img_840_d
sourcePath: uganda/subfolder/img_840_d.jpg
```
