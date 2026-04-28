# Generating Photos (`photogen`)

To resize photos and generate the JSON indexes, run `photogen`. The command varies by mode:

| Mode      | Command                                                               |
|-----------|-----------------------------------------------------------------------|
| Docker    | `ddphotos photogen`                                                   |
| Developer | `bin/photogen` (see [INSTALL.md](INSTALL.md#developer-tools-on-path)) |

Albums are defined in `config/albums.yaml`. See [CONFIGURATION.md](CONFIGURATION.md)
for the full config reference, including site settings, hero image, custom CSS, and
password protection.

Output goes to `<albums-dir>/<id>/` (git-ignored):

| Mode      | Default output location                                                            | Override                                     |
|-----------|------------------------------------------------------------------------------------|----------------------------------------------|
| Docker    | `albums/` inside the `ddphotos` script directory (i.e. `~/my-ddphotos/albums/`)    | `--albums-dir` pre-command flag              |
| Developer | `albums/` at the repo root (set by `DDPHOTOS_ALBUMS_DIR` in `config/defaults.env`) | `-out` flag or `DDPHOTOS_ALBUMS_DIR` env var |

To run with defaults:

```bash
ddphotos photogen                         # Docker mode (default flags: -resize -index -clean -doit)
bin/photogen -resize -index -clean -doit  # developer mode
```

To use a different albums file (e.g., a development subset):

```bash
ddphotos photogen -- -albums albums-dev.yaml   # Docker mode
bin/photogen -albums albums-dev.yaml           # developer mode
```

## CLI Flags

| Flag          | Default       | Description                                                                                                                          |
|---------------|---------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `-config-dir` | `config`      | Directory containing the albums YAML and descriptions files                                                                          |
| `-albums`     | `albums.yaml` | Albums YAML filename within `-config-dir`                                                                                            |
| `-doit`       | `false`       | Write files; without this, runs in dry-run mode                                                                                      |
| `-resize`     | `false`       | Generate resized WebP image variants                                                                                                 |
| `-index`      | `false`       | Generate JSON index files and sitemap.xml                                                                                            |
| `-out`        | *(from env)*  | Albums directory override (overrides `DDPHOTOS_ALBUMS_DIR`)                                                                          |
| `-limit N`    | `0` (all)     | Limit photos per album (useful during development)                                                                                   |
| `-force`      | `false`       | Regenerate files even if they already exist                                                                                          |
| `-workers N`  | `0` (auto)    | Concurrent resize workers (auto = NumCPU/2, min 2)                                                                                   |
| `-album`      | `""` (all)    | Comma-separated album slugs to process                                                                                               |
| `-site-url`   | *(from YAML)* | Sitemap base URL override (overrides `settings.site_url`)                                                                            |
| `-site-id`    | *(from YAML)* | Override `settings.id`; useful for generating multiple output sites from one config                                                  |
| `-passwords`  | *(from YAML)* | Path to passwords file; overrides `settings.passwords` (see [Password Protection](CONFIGURATION.md#password-protection))             |
| `-css`        | *(from YAML)* | Path to custom CSS file; overrides `settings.css` (see [Custom CSS](CONFIGURATION.md#custom-css))                                    |
| `-clean`      | `false`       | Remove stale files from processed album directories after a run (requires `-resize`)                                                 |
| `-hero-only`  | `false`       | Regenerate the hero image only; skips all album processing and index/JSON generation (see [Hero Image](CONFIGURATION.md#hero-image)) |

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
| Auto sort     | `recurse: true`, no `photogen.txt`                           | All photos from root and subdirectories collected, then globally sorted by date                 |
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
bin/decode albums/my-site/my-album/index.enc.json | grep -B2 "IMG_0436"
```

## Decoding Encrypted Files (`decode`)

The `decode` tool decrypts `.enc.json` files produced by `photogen` and prints the
plaintext JSON. Useful for inspecting what photogen wrote without running the full
frontend.

```bash
bin/decode <path.enc.json>
bin/decode -passwords <pw-file> <path.enc.json>
```

`photogen` embeds the passwords file path in every `.enc.json` it writes, so in most
cases no flags are needed:

```bash
bin/decode albums/sample-pw-uganda/uganda/index.enc.json
bin/decode albums/sample-pw-all/albums.enc.json
```

If the passwords file has moved, or the file was generated without an embedded path,
pass `-passwords` explicitly:

```bash
bin/decode -passwords sample/config/passwords-uganda.yaml \
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
