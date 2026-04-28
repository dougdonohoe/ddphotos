# Configuration

A DD Photos site is driven by three config files. Both Docker and developer modes
use the same files — only the path conventions differ.

| File               | Required | Purpose                                                 |
|--------------------|----------|---------------------------------------------------------|
| `albums.yaml`      | yes      | Albums, site settings, photo base paths                 |
| `descriptions.txt` | no       | Per-album descriptions shown on the home page           |
| `site.env`         | no       | Deploy credentials (never committed)                    |

**Docker mode:** `ddphotos init` creates a `config/` directory with starter versions of
all three files, ready to edit directly — no copying needed.

**Developer mode:** The repo's `config/` directory contains example files. Copy and edit
them to get started:

```bash
cp config/albums.example.yaml config/albums.yaml
cp config/descriptions.example.txt config/descriptions.txt
cp config/site.example.env config/site.env
```

The `sample/config/` files are a working example that drives the demo site at
[ddphotos.donohoe.info](https://ddphotos.donohoe.info).

---

## albums.yaml

The primary config file. See [config/albums.example.yaml](../config/albums.example.yaml)
for the full format and all options.

### Site Settings

The `settings:` block defines the site's identity and optional features:

```yaml
settings:
  id: my-site                            # required; names the output directory
  site_name: "My Photo Albums"           # required
  site_url: https://photos.example.com   # required
  site_description: "My photos"          # required
  copyright_owner: "Your Name"           # required
  copyright_year: 2020                   # required
  descriptions: descriptions.txt         # path to descriptions file (relative to config dir)
  allow_crawling: false                  # set true to allow search engine indexing
  site_title_html: "<b>My Photos</b>"    # optional HTML for home page title
  site_subtitle_html: "Since 2010"       # optional HTML below the title
  site_overview_html: "Welcome!"         # optional HTML above album cards
```

| Setting              | Required | Description                                                                                       |
|----------------------|----------|---------------------------------------------------------------------------------------------------|
| `id`                 | yes      | Names the output directory; must be lowercase letters, digits, and hyphens                        |
| `site_name`          | yes      | Site title shown in the browser tab and OG tags                                                   |
| `site_url`           | yes      | Canonical base URL (e.g. `https://photos.example.com`); used in sitemap and OG tags               |
| `site_description`   | yes      | Meta description and OG description for the home page                                             |
| `copyright_owner`    | yes      | Name shown in the footer copyright line                                                           |
| `copyright_year`     | yes      | Start year shown in the footer copyright line                                                     |
| `descriptions`       | no       | Path to the descriptions file, relative to the config dir (default: `descriptions.txt`)           |
| `allow_crawling`     | no       | Set to `true` to allow search engine crawling; adds `Sitemap:` to `robots.txt` (default: `false`) |
| `site_title_html`    | no       | HTML for the site title on the home page; falls back to `site_name` when omitted                  |
| `site_subtitle_html` | no       | HTML rendered below the site title in a smaller font                                              |
| `site_overview_html` | no       | HTML rendered above the album cards (slightly larger than album descriptions)                     |

### How Config Reaches the Frontend

`photogen` acts as a conduit between `albums.yaml` and the site frontend. The frontend
never reads `albums.yaml` directly — instead, `photogen` processes it and writes a set of
static JSON files that the browser fetches at runtime:

| File                                            | Content                                                                | Encrypted when                            |
|-------------------------------------------------|------------------------------------------------------------------------|-------------------------------------------|
| `config.json`                                   | Site ID, hero/CSS filenames, password hints, which albums file to load | Never — always plaintext (bootstrap file) |
| `html.json` / `html.enc.json`                   | `site_title_html`, `site_subtitle_html`, `site_overview_html`          | Site password is set                      |
| `albums.json` / `albums.enc.json`               | Album list with names, slugs, descriptions, date ranges, cover photos  | Site password is set                      |
| `<album>/index.json` / `<album>/index.enc.json` | Per-album photo list: filenames, dimensions, dates, captions           | Album or site password is set             |
| `sitemap.xml`                                   | URLs for each album, built from `site_url`                             | Never                                     |
| `hero.jpg`                                      | Cropped hero banner image                                              | Never                                     |
| `custom.css`                                    | Copied from the file named in `settings.css`                           | Never                                     |

`config.json` is the one file the frontend always fetches first, in plaintext, to
bootstrap the page. It tells the browser what site it is, whether albums are encrypted,
where to find the hero and CSS, and what hints to show before a password is entered.

The three `*_html` fields are the only settings that are encrypted when a site password
is set, since they may contain private links or contact details. All other settings
travel via `config.json` which is always plaintext.

### Hero Image

An optional full-width banner image can be displayed at the top of the home page.
Add a `hero:` block under `settings:`:

```yaml
settings:
  hero:
    image: my-banner.jpg   # filename; joined to 'base' if set, else relative to config dir
    base: drive            # optional — same base map as album entries
    crop: center           # top | center | bottom (default: center)
```

`photogen` hard-crops the source image to 1600×250px and writes it as `hero.jpg` in the
albums output directory. The hero is never encrypted and takes priority as the `og:image`
on the home page.

To regenerate the hero without reprocessing albums or rebuilding indexes:

```bash
ddphotos photogen -- -hero-only        # Docker mode
bin/photogen -hero-only -doit          # developer mode
```

### Custom CSS

To override site styles, add a `css:` entry under `settings:`:

```yaml
settings:
  css: custom.css   # filename relative to this config dir
```

`photogen` copies the file to the site output as `custom.css`. The frontend injects it
site-wide as a `<link>` after the built-in styles, so any rules inside it take effect as
normal cascade overrides. Redefining CSS custom properties (e.g. `--bg-color`,
`--text-color-2nd`) is the cleanest approach — no specificity battles needed.

### Password Protection

Encryption is enabled by adding a `passwords:` entry under `settings:` pointing to a
YAML passwords file (path relative to the config dir):

```yaml
settings:
  passwords: passwords.yaml
```

The `-passwords` CLI flag overrides this at run time. When a passwords file is present,
`photogen` encrypts `albums.json` and each album's `index.json` using AES-256-GCM (keys
derived via PBKDF2-SHA256). Encrypted files are written as `.enc.json` alongside their
plaintext counterparts. The custom HTML fields (`site_title_html`, `site_subtitle_html`,
`site_overview_html`) are encrypted into `html.enc.json` and decrypted as part of the
same unlock step.

Decryption happens entirely in the browser using the Web Crypto API — passwords are never
sent to a server.

**Do not commit real passwords.** Store the passwords file outside the repo or in a
git-ignored directory (e.g. `.secrets/`).

#### Passwords File Format

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

#### Frontend Behavior (Encrypted Sites)

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

**Cover flash prevention:** Album cover images are cached in localStorage after unlock,
as a visual indicator that an album is accessible.
An inline script in `app.html` runs synchronously before first paint, reading the cover
cache and setting CSS custom properties (`--ddp-cover-{slug}`) on `<html>`. This means
the cover image is visible from the very first paint with no flash, even before Svelte
hydrates.

**localStorage key format** (useful for debugging):

| Key                         | Contains                          |
|-----------------------------|-----------------------------------|
| `ddp_site_{siteId}`         | Site-wide password                |
| `ddp_album_{siteId}_{slug}` | Per-album password for `slug`     |
| `ddp_cover_{siteId}_{slug}` | Cached cover image URL for `slug` |

All keys are scoped to `siteId` so that switching between builds (which use different
HMAC keys and produce different filenames) automatically invalidates stale cached data.

---

## descriptions.txt

Per-album descriptions shown on the home page album cards. Referenced from `albums.yaml`
via `settings.descriptions`. See [config/descriptions.example.txt](../config/descriptions.example.txt)
for the format.

---

## site.env

Holds deploy credentials — nothing that affects the built site itself.
See [Deploy Variables](ENV.md#deploy-variables-siteenv) for the full variable reference.

This file should never be committed. Store it outside the repo or in a git-ignored location.

**rsync deployment:**

```bash
RSYNC_HOST=user@your-server.example.com
RSYNC_DEST=/path/to/your/web/root/
CLOUDFRONT_ID=YOUR_CLOUDFRONT_DISTRIBUTION_ID   # optional; invalidates cache after deploy if you are using CloudFront
```

**S3 + CloudFront deployment:**

```bash
S3_BUCKET=your-s3-bucket
CLOUDFRONT_ID=YOUR_CLOUDFRONT_DISTRIBUTION_ID   # optional; invalidates cache after deploy if you are using CloudFront
```

See [Deployment](DEPLOY.md) for full deploy details.
