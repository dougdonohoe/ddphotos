# Configuration

A DD Photos site is driven by a handful of config files. Both Docker and developer modes
use the same files — only the path conventions differ.

| File                 | Required | Purpose                                                   |
|----------------------|----------|-----------------------------------------------------------|
| `albums.yaml`        | yes      | Albums, site settings, photo base paths                   |
| `customization.yaml` | no       | Overrides to the site's chrome                            |
| `passwords.yaml`     | no       | Password protection (name it via `settings.passwords`)    |
| `custom.css`         | no       | Style overrides (name it via `settings.css`)              |
| `site.env`           | no       | Deploy credentials                                        |
| `immich.env`         | no       | Immich credentials (only for albums with a `sync:` block) |

**Docker mode:** `ddphotos init` creates a `config/` directory holding all of the above,
ready to edit directly — no copying needed. It also installs a `sample-photos/` folder
next to `config/` and wires it into `albums.yaml` as the base `sample-base`, so the
starter site has real photos on your machine to work with. Delete both once you have
added your own albums.

**Developer mode:** The repo's `config/` directory contains example files. Copy and edit
them to get started:

```bash
cp config/albums.example.yaml config/albums.yaml
cp config/site.example.env config/site.env
```

A good reference are the `sample/config/` files that drives the demo site at
[ddphotos.donohoe.info↗](https://ddphotos.donohoe.info).

**Unknown keys are rejected.** Every key in the YAML files below has to be one photogen
knows, at every level. Almost all of them are optional, so a misspelled key would otherwise
be silently ignored and the default used instead, with nothing to show for it:

```
parse config/albums.yaml: yaml: unmarshal errors:
  line 3: field site_nmae not found in type photogen.AlbumsSettings
```

The line number and the offending key are in the message. Album slugs and `bases` names are
free-form, since they are map keys rather than settings. An empty file, or one that is only
comments, is treated as "nothing configured" rather than an error.

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
  allow_crawling: false                  # set true to allow search engine indexing
  site_title_html: "<b>My Photos</b>"    # optional HTML for home page title
  site_subtitle_html: "Since 2010"       # optional HTML below the title
  site_overview_html: "Welcome!"         # optional HTML above album cards
```

| Setting              | Required | Description                                                                                                  |
|----------------------|----------|--------------------------------------------------------------------------------------------------------------|
| `id`                 | yes      | Names the output directory; must be lowercase letters, digits, and hyphens                                   |
| `site_name`          | yes      | Site title shown in the browser tab and OG tags                                                              |
| `site_url`           | yes      | Canonical base URL (e.g. `https://photos.example.com`); used in sitemap and OG tags                          |
| `site_description`   | yes      | Meta description and OG description for the home page                                                        |
| `copyright_owner`    | yes      | Name shown in the footer copyright line                                                                      |
| `copyright_year`     | yes      | Start year shown in the footer copyright line                                                                |
| `descriptions`       | no       | Path to a [descriptions file](#album-descriptions), relative to the config dir; see that section for details |
| `allow_crawling`     | no       | Set to `true` to allow search engine crawling; adds `Sitemap:` to `robots.txt` (default: `false`)            |
| `site_title_html`    | no       | HTML for the site title on the home page; falls back to `site_name` when omitted                             |
| `site_subtitle_html` | no       | HTML rendered below the site title in a smaller font                                                         |
| `site_overview_html` | no       | HTML rendered above the album cards (slightly larger than album descriptions)                                |

### Album Slugs

Each album's `slug` is both its output directory and its URL (`/albums/<slug>`), so
`photogen` checks every slug before it reads any photos, and stops with an error naming
the album if one breaks a rule:

- It must **start with a letter or digit** and contain only **letters, digits, dashes and
  underscores**. No dots, spaces or slashes, so `uganda.2007` and `_draft` are rejected.
- It can be **at most 64 characters**, the same limit the DD Photos App enforces.
- It must be **unique**. Two albums with the same slug would share one output directory,
  and the second would silently overwrite the first.
- Two slugs must not **differ only by case**, such as `Patagonia` and `patagonia`. They are
  distinct URLs but the same directory on macOS and Windows.

Upper case is allowed otherwise. The site `id` is stricter: lowercase letters, digits and
hyphens only.

### Source Bases

The `bases:` block defines named paths to where your source photos live. Albums (and
the hero image) reference a base by name via the `base:` key, so you can keep album
entries short and change a root path in one place:

```yaml
bases:
  drive: /Volumes/MyDrive/Photos   # absolute path (e.g. an external drive)
  cloud: /Users/me/Dropbox/Photos  # absolute path (e.g. a cloud-synced folder)
  local: photos                    # relative to the root DD Photos folder (sibling of config/)

albums:
  - slug: patagonia
    name: Patagonia
    base: drive                    # references bases.drive
    source: 2026-Patagonia         # joined to the base path -> /Volumes/MyDrive/Photos/2026-Patagonia
```

Each base value is either an absolute path or a path relative to the root DD Photos
folder. An album's `source:` is joined to its named base; an album with no `base:` must
give an absolute path in `source:`.

The starter config written by `ddphotos init` is a working example of the relative form —
its `sample-base` points at the `sample-photos/` folder installed beside `config/`:

```yaml
bases:
  sample-base: sample-photos

albums:
  - slug: vacation
    name: Vacation
    base: sample-base
    source: vacation               # -> sample-photos/vacation
```

In **Docker mode**, the `ddphotos` wrapper script reads these base paths and mounts
absolute ones into the container automatically, so the paths you list must exist on your
machine. Relative bases need no mount — they already live under the ddphotos folder,
which is always mounted.

> **Note:** a relative base is resolved against the working directory `photogen` runs in.
> In Docker mode that is always your ddphotos folder, so it behaves as described above. In
> developer mode, run `photogen` from that folder.

### Syncing an Album from a Photo Manager

Most albums point at a folder you curate yourself. An album can instead carry a `sync:`
block, and `photogen` will fetch its photos from an upstream photo manager before the
normal build:

```yaml
albums:
  - slug: galapagos
    name: Galápagos 2024          # optional; falls back to the upstream album name
    # description: >-             # optional; falls back to the upstream description
    #   Snorkeling with sea lions and a lot of blue-footed boobies.
    # cover: IMG_0042.jpg         # optional; the file name as it lands in the sync folder
    sync:
      provider: immich            # an album syncs from exactly one provider
      album_id: d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18
      captions: true              # write photogen.txt from each asset's description
```

| Key        | Required | Default | Meaning                                             |
|------------|----------|---------|-----------------------------------------------------|
| `provider` | yes      | none    | `immich` or `mock` (see [TESTING.md](TESTING.md))   |
| `album_id` | yes      | none    | Provider-specific album identifier (Immich: a UUID) |
| `captions` | no       | `true`  | Write and maintain `photogen.txt` from descriptions |

**Finding the `album_id`:** in Immich, open the album and copy the last path segment out of
your browser's address bar.

```
http://localhost:2283/albums/d8052d5c-9ff1-4228-9f02-5cdd3d2e2d18
                             └──────────── album_id ────────────┘
```

#### Immich credentials: `immich.env`

Credentials live in `immich.env` beside `albums.yaml`, so they stay out of a file you might
share:

```bash
# config/immich.env
IMMICH_API_KEY=your-api-key
IMMICH_INSTANCE_URL=http://localhost:2283
```

- The key comes from Immich's **Account Settings → API Keys**, and needs exactly three
  permissions: `asset.read`, `asset.download` and `album.read`.
- The URL is accepted with or without a trailing `/api`. An instance behind a reverse proxy
  at `https://photos.example.com/immich` works too.
- **A value set in the environment wins over the file**, so a CI run needs no secrets file on
  disk. The file is read only when an album actually names the `immich` provider.
- `config/immich.env` and `config/immich-*.env` are gitignored. The API key is never logged,
  printed in an error, or written to `metadata.yaml`.

In Docker, `http://localhost:2283` is still the right value to write — see
[Docker](DOCKER.md#syncing-from-immich-in-docker).

#### What Immich publishes, and what it does not

A few assets in an Immich album do not reach the site, and each one says so as a warning:

| Asset                | What happens                                                                                                                                                    |
|----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Hidden or locked     | Skipped. An **archived** asset is published, on the grounds that you put it in the album on purpose                                                             |
| In the trash         | Skipped                                                                                                                                                         |
| RAW                  | Skipped: photogen cannot resize it                                                                                                                              |
| A Live Photo's video | Skipped when a photo and a video in the album share a base name; the still is published                                                                         |
| Edited in Immich     | **Published, unedited.** Immich serves the pre-edit original, and strips the EXIF date from its edited copy, which would sort the photo to the end of the album |

#### Where synced media lives

```
{DDPHOTOS_SYNC_DIR}/{site-id}/{provider}/{slug}/
├── metadata.yaml     # written by sync; do not edit
├── photogen.txt      # written by sync when captions are on; yours to edit
├── IMG_1583.jpeg
└── IMG_1584.jpeg
```

`DDPHOTOS_SYNC_DIR` defaults to `sync/` beside `config/` and is resolved exactly like
`DDPHOTOS_ALBUMS_DIR`: the `-sync-dir` flag, then the environment, then
`config/defaults.env`. It is namespaced by `settings.id` for the same reason `albums/` is:
two config files sharing one root and one slug would otherwise share a folder and prune
each other's photos.

The folder is an ordinary album source, so everything downstream — scanning, EXIF, resize,
cover, index — is unchanged. Nothing under it is deployed; the deploy and export scripts
work from `albums/` and `build/`.

**`photogen.txt` is the only file in there you may edit.** Everything else is managed:
sync writes the media and `metadata.yaml`, and deletes anything it does not recognize.

#### Validation rules

- `sync:` and `source:`/`base:` are mutually exclusive, because a synced album's source is
  derived rather than configured.
- `source:` is required *unless* the album has a `sync:` block.
- `name:` is optional when `sync:` is present. It resolves as: the `albums.yaml` value,
  then the upstream name recorded in `metadata.yaml`, then the slug.
- `description:` resolves as: the inline value, then the descriptions file, then the
  upstream description. The upstream value slots below both existing sources, so
  "inline wins over the file" is unchanged.
- `provider` must be a name `photogen` knows, and `album_id` must not be empty.
- For the `immich` provider, `album_id` must look like a UUID, and no two albums may sync the
  same one from the same provider. Both are checked as the config is read, before anything
  downloads.
- Everything else about the album — slug rules, `cover:`, `manual_sort_order:`,
  `recurse:` — is unchanged.

#### Captions

With `captions: true` (the default), sync maintains `photogen.txt` from the upstream
descriptions. Because a caption can change upstream *or* locally between runs,
`metadata.yaml` records what upstream said last time and the two are merged:

| What changed         | Result                                     |
|----------------------|--------------------------------------------|
| Only upstream        | The upstream text is written               |
| Only your local edit | Your edit is kept, on this and every run   |
| Both                 | The upstream text wins, and photogen warns |

Empty is a value, so clearing a description upstream clears an untouched local caption.
Line order is preserved — existing photos keep their position and new ones are appended —
so a manual ordering used by `manual_sort_order: true` survives a re-sync. Comments, blank
lines and subfolder entries stay where you put them. A line for a photo removed upstream is
dropped, as is an entry naming neither a photo nor a subfolder. To make local captions
permanently authoritative, set `captions: false`.

Upstream descriptions are plain text and DD Photos captions render as HTML, so `&`, `<`
and `>` are escaped on the way in. A caption you write by hand in `photogen.txt` is still
raw HTML, exactly as it is for a non-synced album.

#### Running it

Syncing happens on every run unless you pass `-no-sync`, and **it is real work even
without `-doit`**: it downloads and it prunes. That is deliberate, since a dry run whose
whole point is to show what would be built needs the source folder to exist. See
[PHOTOGEN.md](PHOTOGEN.md#syncing) for `-sync-only`, `-no-sync` and `-sync-dir`.

`sample/config-sync/` is a complete working example that needs no network or account:
run `make sample-sync` to sync two albums from the offline
[`mock` provider](TESTING.md#the-mock-sync-provider) and serve the result.

### How Config Reaches the Frontend

`photogen` acts as a conduit between `albums.yaml` and the site frontend. The frontend
never reads `albums.yaml` directly — instead, `photogen` processes it and writes a set of
static JSON files that the browser fetches at runtime:

| File                                            | Content                                                                           | Encrypted when                            |
|-------------------------------------------------|-----------------------------------------------------------------------------------|-------------------------------------------|
| `config.json`                                   | Site ID, hero/CSS filenames, password hints, album nav, which albums file to load | Never — always plaintext (bootstrap file) |
| `html.json` / `html.enc.json`                   | `site_title_html`, `site_subtitle_html`, `site_overview_html`                     | Site password is set                      |
| `albums.json` / `albums.enc.json`               | Album list with names, slugs, descriptions, date ranges, cover photos             | Site password is set                      |
| `<album>/index.json` / `<album>/index.enc.json` | Per-album photo list: filenames, dimensions, dates, captions                      | Album or site password is set             |
| `sitemap.xml`                                   | Site root plus each album without a password, built from `site_url`               | Never                                     |
| `hero.jpg`                                      | Cropped hero banner image                                                         | Never                                     |
| `custom.css`                                    | Copied from the file named in `settings.css`                                      | Never                                     |

`config.json` is the one file the frontend always fetches first, in plaintext, to
bootstrap the page. It tells the browser what site it is, whether albums are encrypted,
where to find the hero and CSS, and what hints to show before a password is entered.

The three `*_html` fields are the only settings that are encrypted when a site password
is set, since they may contain private links or contact details. All other settings
travel via `config.json` which is always plaintext — including
[`album_nav`](#album_nav) from `customization.yaml`, so do not put a private URL in a nav
link on a password-protected site.

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

The hero must be a still image (`.jpg`, `.jpeg`, `.png`, `.webp`, `.tif`, `.tiff`, `.heic`,
`.heif`, `.avif`); a video is rejected at config validation, because the hero is a hard crop. An
album `cover:` **may** point at a video, which uses its poster frame instead. See
[Video](PHOTOGEN.md#video).

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

See **[Custom CSS](CUSTOM-CSS.md)** for the full guide: the available custom properties,
the class names worth targeting, why some overrides need `!important` (Svelte scopes most
built-in rules, which raises their specificity above yours), how to iterate in about a
second without reprocessing photos, and worked examples.

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

**Encrypted albums are left out of `sitemap.xml`.** An album's slug is the one thing a
crawler could learn about it without the password, so the sitemap withholds it. The site
root is always listed, since it is a public page that serves the password prompt, which
means a site-wide password leaves a sitemap with that single entry.

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
| `albums.<slug>.password` | Per-album password; encrypts only that album's `index.json`. Required whenever an `albums.<slug>` entry is present                                                |
| `albums.<slug>.hint`     | Optional hint shown in that album's password dialog                                                                                                               |

**An album with no `albums.<slug>` entry is still encrypted when `site.password` is set.** Its
`index.json` is encrypted with the site password, and a visitor unlocks it by entering that
password once: the frontend stores it under `ddp_site_<siteId>` and reuses it for every album
that has no password of its own. Such an album never shows a dialog of its own, which is why an
entry carrying only a `hint` is rejected at startup rather than treated as "encrypted with the
site password, plus a hint". A hint is only ever displayed in that album's own password dialog,
so to give an album a hint, give it a unique password.

An `albums.<slug>` entry whose slug is not in `albums.yaml` — typically an album that was
deleted but whose password was left behind — is ignored, and `photogen` prints a warning
naming the slug. It protects nothing, so it does not count towards whether the site is
encrypted.

Sample passwords files are in `sample/config/` — `passwords-all.yaml` (full site),
`passwords-uganda.yaml` (single album), and `passwords-keyonly.yaml` (a test fixture with a
`key` and a password for a nonexistent album, so nothing is encrypted). The first two
contain demo-only passwords.

A passwords file that declares no effective password — no `site.password`, and no
`albums.<slug>.password` for an album that exists — protects nothing: the `key` only applies
to albums that have a password, so every album stays public and filenames stay unobfuscated.
Such a site is reported as unencrypted in `config.json`, which means no logout button is shown.

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

| Key                         | Contains                                                                                   |
|-----------------------------|--------------------------------------------------------------------------------------------|
| `ddp_theme`                 | Light/dark mode preference (`light` or `dark`)                                             |
| `ddp_site_id`               | Current build token (`siteId` or `siteId:keyId`); triggers stale cache clearance on change |
| `ddp_site_{siteId}`         | Site-wide password (encryption only)                                                       |
| `ddp_album_{siteId}_{slug}` | Per-album password for `slug` (encryption only)                                            |
| `ddp_cover_{siteId}_{slug}` | Cached cover image URL for `slug` (encryption only)                                        |

The `ddp_*` keys are scoped to `siteId` so that switching between builds (which use different
HMAC keys and produce different filenames) automatically invalidates stale cached data.
`?clear` removes all `ddp_*` keys (including `ddp_theme`), returning the site to its default state.

---

## customization.yaml

An optional file sitting alongside `albums.yaml`, holding changes to the site's chrome.
`photogen` picks it up automatically when it is present — there is nothing to reference
from `albums.yaml`. Without the file, the site looks exactly as it does by default.

It is deliberately separate from `albums.yaml` so that tools which manage album
definitions never need to know about it. See
[config/customization.example.yaml](../config/customization.example.yaml) for the
annotated version.

Two `photogen` flags control it:

| Flag                    | Effect                                                                                    |
|-------------------------|-------------------------------------------------------------------------------------------|
| `-customization <file>` | Read this file instead of `<config-dir>/customization.yaml`. Errors if it does not exist. |
| `-no-customization`     | Ignore `customization.yaml` even when present.                                            |

### album_nav

By default, every album page's header starts with a `← Albums` link back to the home page
(which is the album list). Setting `album_nav` **replaces** that link with your own:

```yaml
album_nav:
  - label: Back to Maps
    href: https://example.com/
    id: back-to-maps
    new_tab: true
  - label: All Albums
    href: /
    id: back-to-albums
```

| Key       | Required | Description                                                                            |
|-----------|----------|----------------------------------------------------------------------------------------|
| `label`   | yes      | Link text                                                                              |
| `href`    | yes      | Either a path starting with `/` (a page on this site) or a full URL including a scheme |
| `id`      | no       | HTML `id` on the anchor, so [custom CSS](#custom-css) can style that one link          |
| `new_tab` | no       | Open in a new tab (adds `target="_blank" rel="noopener"`); default `false`             |

Links whose `href` starts with `/` are resolved against the site root and navigate
client-side, so they stay fast and keep working if the site moves. A bare relative path
such as `albums/foo` is rejected, since it would resolve differently depending on which
page the visitor is on.

The links inherit the header's default link styling, so a bare config looks like the
stock header. Give a link an `id` to style it on its own:

```css
#back-to-maps {
	display: inline-block;
	padding: 0.45rem 0.9rem;
	border: 1px solid rgba(74, 222, 128, 0.5);
	border-radius: 999px;
	background: rgba(74, 222, 128, 0.14);
	color: inherit;
	font-weight: 700;
	text-decoration: none;
}
```

An `id` selector outranks the built-in `header a` rule, so no `!important` is needed.
Ids must start with a letter, contain only letters, digits, hyphens or underscores, and
be unique within the list — `photogen` rejects anything else so a typo cannot silently
produce a selector that never matches.

Nav links travel in `config.json`, which is never encrypted. That keeps them working for
a visitor who unlocked with only a per-album password, but it also means the labels and
URLs are public even on a password-protected site.

---

## Album Descriptions

Per-album descriptions are shown on the home page album cards and on each album page.
The preferred way to set them is inline in `albums.yaml`:

```yaml
albums:
  - slug: patagonia
    name: Patagonia
    source: /photos/patagonia
    description: Two weeks hiking the Torres del Paine circuit.
```

### descriptions.txt

As an alternative, descriptions can be kept in a separate file and referenced from
`albums.yaml` via `settings.descriptions`:

```yaml
settings:
  descriptions: descriptions.txt
```

This is useful when you want to share one descriptions file across multiple config
files. See [config/descriptions.example.txt](../config/descriptions.example.txt) for
the format.

When both an inline `description:` and a `descriptions.txt` entry exist for the same
album, the inline value takes precedence.

---

## site.env

Holds deploy credentials — nothing that affects the built site itself.
See [Deploy Variables](ENV.md#deploy-variables-siteenv) for the full variable reference.

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
