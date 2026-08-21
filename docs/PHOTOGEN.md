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

| Mode      | Default output location                                                            | Override                                       |
|-----------|------------------------------------------------------------------------------------|------------------------------------------------|
| Docker    | `albums/` inside the `ddphotos` script directory (i.e. `~/my-ddphotos/albums/`)    | `--dir` pre-command flag can change script dir |
| Developer | `albums/` at the repo root (set by `DDPHOTOS_ALBUMS_DIR` in `config/defaults.env`) | `-out` flag or `DDPHOTOS_ALBUMS_DIR` env var   |

To run with defaults:

```bash
ddphotos photogen                         # Docker mode (default flags: -resize -index -clean -doit)
bin/photogen -resize -index -clean -doit  # developer mode
```

## CLI Flags

| Flag                | Default       | Description                                                                                                                                          |
|---------------------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-config-dir`       | `config`      | Directory containing `albums.yaml` and optional supporting files                                                                                     |
| `-doit`             | `false`       | Write files; without this, runs in dry-run mode                                                                                                      |
| `-resize`           | `false`       | Generate resized WebP image variants                                                                                                                 |
| `-index`            | `false`       | Generate JSON index files and sitemap.xml                                                                                                            |
| `-out`              | *(from env)*  | Albums directory override (overrides `DDPHOTOS_ALBUMS_DIR`)                                                                                          |
| `-limit N`          | `0` (all)     | Limit photos per album (useful during development)                                                                                                   |
| `-force`            | `false`       | Regenerate files even if they already exist; also re-reads photo metadata instead of using the [cache](#metadata-cache)                              |
| `-workers N`        | `0` (auto)    | Concurrent resize workers (auto = NumCPU/2, min 2)                                                                                                   |
| `-album`            | `""` (all)    | Comma-separated album slugs to process                                                                                                               |
| `-site-url`         | *(from YAML)* | Sitemap base URL override (overrides `settings.site_url`)                                                                                            |
| `-site-id`          | *(from YAML)* | Override `settings.id`; useful for generating multiple output sites from one config                                                                  |
| `-passwords`        | *(from YAML)* | Path to passwords file; overrides `settings.passwords` (see [Password Protection](CONFIGURATION.md#password-protection))                             |
| `-css`              | *(from YAML)* | Path to custom CSS file; overrides `settings.css` (see [Custom CSS](CONFIGURATION.md#custom-css))                                                    |
| `-customization`    | *(auto)*      | Path to a customization file; overrides the default `<config-dir>/customization.yaml` (see [customization.yaml](CONFIGURATION.md#customizationyaml)) |
| `-no-customization` | `false`       | Ignore `customization.yaml` even when present                                                                                                        |
| `-clean`            | `false`       | Remove stale files from processed album directories after a run (requires `-resize`)                                                                 |
| `-hero-only`        | `false`       | Regenerate the hero image only; skips all album processing and index/JSON generation (see [Hero Image](CONFIGURATION.md#hero-image))                 |

## Metadata Cache

Re-running `photogen` after adding a few photos should only cost what those new photos
need. To make that true, photogen keeps a cache at `<albums-dir>/.build/metadata-cache.json`.

It records two things, both keyed by the source file's modification time and size:

- **Photo metadata** (dimensions, orientation, EXIF date). Without the cache every photo
  is decoded on every run just to recover four values, even when nothing needs resizing.
- **Stamps for fixed-name outputs** (`cover.jpg`, `hero.jpg`). These cannot use the normal
  "output already exists, skip it" rule, because the same filename is produced from a
  source that may have been swapped, so they used to be re-encoded on every run. The stamp
  records which source and settings produced the file, making the skip safe.

Anything that changes is picked up automatically: editing or replacing a source photo,
pointing an album at a different `cover`, changing the hero `image` or `crop`, or deleting
a generated file all trigger a regeneration. The cache only ever suppresses work that would
have produced an identical file.

Notes:

- The cache is shared by every site ID under the same albums directory, since photo
  metadata does not depend on which site is being generated.
- It lives outside `<albums-dir>/<id>/`, so it is never deployed, never served, and never
  touched by `-clean`.
- It is written in dry-run mode too. It is a local performance artifact rather than site
  output, so writing it does not conflict with `-doit`, and it means repeated dry-runs are
  fast as well.
- `-force` re-reads everything and rewrites the cache. To discard it entirely, delete
  `<albums-dir>/.build/metadata-cache.json`; it is rebuilt on the next run.

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

The name is everything up to the first space, so a **name containing spaces must be
double-quoted**. Without quotes the first word becomes the name and the rest of the
filename is folded into the caption:

```
"Doug and Cindy Chicago.jpg" A cool trip to Chicago
doug-and-cindy-chicago.jpg A cool trip to Chicago
```

Quoting applies to subfolder placeholder names too (e.g. `"Ski 2007"`). Quotes inside
a caption need no escaping.

Descriptions are stored in `index.json` and used as:

- Hover caption overlay in the grid (desktop)
- Always-visible caption in the grid (mobile)
- Caption overlaid on the photo in the lightbox
- `alt` text on grid and lightbox images, and the grid tile's `aria-label`, in each
  case with any HTML tags stripped (see below)

To also use the file for **sort order** (instead of EXIF date), set
`manual_sort_order: true` on the album entry in `albums.yaml`. Photos not
listed in `photogen.txt` are sorted by date and appended at the end.

### HTML in captions

`photogen.txt` is written by the site owner, so captions may contain **inline HTML** and
it is rendered rather than escaped, the same way `site_title_html` and album descriptions
are:

```
Patagonia-042 First view of <b>Torres del Paine</b> at sunrise.
Patagonia-107 Route notes are on <a href="https://example.com/pass">my blog</a>.
```

Supported: `<b>`, `<strong>`, `<i>`, `<em>`, `<u>`, `<small>`, `<code>`, `<span>`, `<br>`
and `<a>`.

**Block elements such as `<div>`, `<p>`, `<ul>` and `<h1>` are not supported** and will
break the caption layout. There is no sanitizer; a caption is inserted as written.

Two behaviors differ by location:

- **Links are clickable in the lightbox only**, and always open in a new tab so the
  lightbox is not torn down. You do not need to write `target="_blank"` yourself.
- **In the grid**, a caption renders its formatting but is not interactive: the whole
  tile is a single click target that opens the lightbox. A link there is styled but
  inert.

Because `alt` text and the tile's `aria-label` cannot render markup, they use the caption
with its tags stripped.

## Video

Videos placed alongside photos in a source folder become album items: the grid shows a
still frame with a play badge and the duration, and clicking one opens it in the lightbox
with normal HTML5 controls.

Supported source extensions: **`.mov`, `.mp4`, `.m4v`**.

In the lightbox the video starts muted with the browser's native controls, and **space
toggles play/pause**. Arrow keys and Escape keep their usual meaning, so a video slide
navigates like any other. Zoom is deliberately disabled on video slides (button, double-tap,
pinch and the `z` key): zooming scales the element, which would enlarge the browser's control
bar along with the picture and push play and scrub off-screen.

### Why videos are always re-encoded

Phone video is routinely **HEVC in a `.mov` container**, which does not play in Chrome or
Firefox. Re-encoding is therefore a correctness requirement, not a size optimization:
copying the original through would produce an album that plays for some visitors and not
others. Every video is transcoded to:

| Part      | Encoding                                                                            |
|-----------|-------------------------------------------------------------------------------------|
| Container | MP4, `-movflags +faststart` so playback starts before the file finishes downloading |
| Video     | H.264 High profile, `yuv420p`, CRF 23, long edge capped at 1280                     |
| Audio     | AAC-LC 128k stereo, kept when present                                               |

One measured example: a ten-second 1080p iPhone clip went from 17 MB in to 3 MB out. The
output size varies a great deal with the content, so see [Size limits](#size-limits)
before assuming a number.

### Output layout

Each video produces three files, in addition to its `index.json` entry:

```
albums/<site-id>/<album>/
    video/<name>.mp4      # the transcoded, playable video
    grid/<name>.webp      # poster frame, 600px  (identical ladder to a photo)
    full/<name>.webp      # poster frame, 1600px
```

The poster frame is extracted with ffmpeg and then run through the **same** WebP pipeline
as every other image, so it picks up the same quality settings and metadata stripping. In
an encrypted album all three outputs share one HMAC-derived stem, computed from the
original filename, so nothing leaks the source name and the clip and its posters still
line up.

The poster is taken one second in, or at the midpoint of anything shorter than two
seconds, which skips the fade-in and autofocus hunting most clips open with.

Videos appear in `index.json` with `kind`, `duration` and an extra `src` entry; a still
carries none of the three:

```json
{
  "fileName": "beach.mov",
  "width": 1280, "height": 720,
  "kind": "video",
  "duration": 10.17,
  "src": { "grid": "grid/beach.webp", "full": "full/beach.webp", "video": "video/beach.mp4" }
}
```

`width`/`height` are the **displayed** dimensions. Phone video stores rotation as a display
matrix rather than rotated pixels, so a portrait clip probes as 1920x1080 with a rotation of
±90; photogen swaps the two, otherwise portrait clips would lay out as landscape boxes in
the grid.

### Captions, ordering and covers

Captions work exactly as for photos: name the clip in `photogen.txt`, with or without its
extension. The date used for sorting comes from the container's `creation_time` tag rather
than EXIF.

A video **can** be an album cover (its poster is used for `cover.jpg`), but it **cannot**
be the site hero image, which must be a still.

### Live Photos and other same-name pairs

A photo's ID is its filename with the extension removed, so two files in one folder whose
names differ only by extension collide. `photogen` **fails the run** and names the pair:

```
/photos/iceland: 1 duplicate photo ID(s) — these source files differ only by extension,
so they would produce the same output file. Rename one of each pair:
    "img_1234": IMG_1234.HEIC, IMG_1234.MOV
```

This matters most for **Apple Live Photos**, which export as a `.HEIC` and a `.MOV`
sharing one stem. Rename one side (`IMG_1234-clip.MOV`) to publish both, or delete the
one you do not want.

The check is not specific to video: two stills such as `photo.jpg` and `photo.png` collide
the same way. Video simply makes the situation common, because a folder exported from
Photos now has clips in it that `photogen` will pick up.

### ffmpeg

Video support needs `ffmpeg` and `ffprobe`. Photo-only sites never need them and are
unaffected.

- **Docker:** nothing to do. The first run that encounters a video downloads a pinned
  static build into the `ddphotos-ffmpeg` Docker volume, where it persists for later runs.
  Run `ddphotos install-ffmpeg` to fetch it ahead of time, or `--force` to reinstall.
- **Native:** `brew install ffmpeg` (macOS) or `sudo apt-get install ffmpeg` (Linux).
  Anything on `PATH` is used as-is and nothing is downloaded.

A dry run downloads ffmpeg too. Reporting what a run *would* do means knowing each clip's
dimensions, duration and rotation, and that comes from `ffprobe`, so the tools have to be
present before there is anything to report. The download is a one-time cost that a later
real run would have paid anyway.

ffmpeg is deliberately **not** bundled in the Docker image. The Debian package pulls in 200
packages and 448 MB, nearly all of it SDL2/X11/Wayland required by `ffplay`, with no way to
opt out; every photo-only user would carry that. Downloading at first use on the user's own
machine also avoids redistributing a GPL `libx264` build.

Override the lookup with `DDPHOTOS_FFMPEG_DIR` (a directory holding both executables) or
`DDPHOTOS_FFMPEG` / `DDPHOTOS_FFPROBE` (exact paths).

### Size limits

Videos are large, and one deploy target caps them: **Cloudflare Pages rejects any single
asset over 25 MiB**. S3/CloudFront, rsync and Surge have no such limit.

How much footage fits in 25 MiB depends heavily on the content, because CRF targets a
quality level rather than a bitrate. Two clips from the sample site, both at the settings
above:

| Clip                      | Bitrate  | 25 MiB is about |
|---------------------------|----------|-----------------|
| Penguins, 640x480, static | 1.8 Mbps | 115 seconds     |
| Ocean waves, 1280x720     | 5.4 Mbps | 40 seconds      |

So treat "about a minute" as the planning figure and do not rely on it: water, foliage,
rain and fast pans all cost far more than a locked-off shot. photogen prints a warning
naming any file that crosses the limit, because the resulting deploy failure otherwise
happens far from its cause. The warning is printed when a video is transcoded, so a re-run
that skips an up-to-date file stays quiet — the file is still too large.

See [DEPLOYMENT-SERVERS.md](DEPLOYMENT-SERVERS.md).

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

**Cover photo**: when `cover` is set on a recursive album, use the source-relative path
(e.g. `cover: craigs/img001.jpg`). If omitted, the first collected photo is used.
The source-relative path is in the `sourcePath` field of `index.json`. To find it from an
original filename, grep the decoded index or use `bin/search-cover.sh` (see below).

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
bin/decode --passwords <pw-file> <path.enc.json>
```

`photogen` embeds the passwords file path in every `.enc.json` it writes, so in most
cases no flags are needed:

```bash
bin/decode albums/sample-pw-uganda/uganda/index.enc.json
bin/decode albums/sample-pw-all/albums.enc.json
```

If the passwords file has moved, or the file was generated without an embedded path,
pass `--passwords` explicitly:

```bash
bin/decode --passwords sample/config/passwords-uganda.yaml \
  albums/sample-pw-uganda/uganda/index.enc.json
```

The correct password is selected automatically from the filename:

| File              | Password used                                    |
|-------------------|--------------------------------------------------|
| `albums.enc.json` | Site-wide password (`site.password`)             |
| `html.enc.json`   | Site-wide password (`site.password`)             |
| `index.enc.json`  | Per-album password for the parent directory slug |

## Finding a Cover Photo (`search-cover.sh`)

When browsing the site, and you want to set a photo as an album cover, you need its
source-relative path for the `cover:` field in `albums.yaml`. The easiest way to get it is
to right-click the photo, copy the image URL, and pass it to `bin/search-cover.sh`:

```bash
bin/search-cover.sh <url>
```

The script parses the album slug and image path from the URL, locates the album's
`index.json` (or `index.enc.json` for encrypted albums — decoded automatically via
`cmd/decode`), and searches for the matching `src` entry to print the `id` and `sourcePath`.

The search is scoped to `DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID` (defaults from
`config/defaults.env`). Override to search a different site:

```bash
DDPHOTOS_SITE_ID=sample-pw-all bin/search-cover.sh http://localhost:5173/albums/uganda/full/1996ae71-5ada-d233-8f26-53e46fac4f64.webp
```

Output:

```
Searching...
  Album:  uganda
  Index:  /Users/donohoe/work/ddphotos/albums/sample-pw-all/uganda/index.enc.json
  Source: full/1996ae71-5ada-d233-8f26-53e46fac4f64.webp

Found:
  id:         subfolder_img_840_d
  sourcePath: uganda/subfolder/img_840_d.jpg

Use for cover:
  cover: subfolder/img_840_d.jpg
```
