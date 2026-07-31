# Rebuilding a Site from Its Deployed Copy (`scrape-site.py`)

`bin/scrape-site.py` turns a deployed DD Photos site back into a source directory you can build
with `photogen`. Point it at a URL and a destination folder, and it writes a `config/` directory
and a `photos/` directory that rebuild the site.

This is useful for running someone else's site locally, and for recovering a config directory that
was lost while the deployed site still exists.

```bash
bin/scrape-site.py https://photos.example.com ~/work/mysite            # dry run, writes nothing
bin/scrape-site.py https://photos.example.com ~/work/mysite -doit      # download
```

Requires `rich` and `Pillow` (`uv pip install -r requirements.txt`).

## Flags

| Flag       | Default | Description                                               |
|------------|---------|-----------------------------------------------------------|
| `-doit`    | `false` | Write files; without this, runs in dry-run mode           |
| `-force`   | `false` | Re-download photos that are already present               |
| `-limit N` | all     | Cap photos per album, useful for a quick smoke test       |
| `-format`  | `png`   | Photo output format: `png`, `jpeg`, or `webp` (see below) |

Re-running without `-force` skips photos already on disk, so an interrupted run can be resumed.

## Photo format

Sites publish their photos as WebP, but not everything reads WebP, so photos are saved as **PNG**
by default. Decoding a lossy WebP is deterministic and reproduces its exact pixels, and PNG stores
them without further loss — so the only quality difference from the live site is the resize the
site already published. PNG files are roughly 7x larger than the WebP they came from.

| `-format`       | Fidelity                                                                           | Size (sample site) |
|-----------------|------------------------------------------------------------------------------------|--------------------|
| `png` (default) | Bit-identical to the published photo                                               | 170 MB             |
| `jpeg`          | Re-encoded at quality 95, no chroma subsampling; deviates by up to ~25 per channel | 52 MB              |
| `webp`          | The downloaded bytes, untouched                                                    | 28 MB              |

Use `-format webp` when disk space matters and whatever consumes the photos handles WebP;
`photogen` itself does.

## What it writes

```
<dest>/
├── config/
│   ├── albums.yaml       # site settings, bases, album entries
│   ├── custom.css        # only if the site has one
│   ├── defaults.env      # albums output dir + site id
│   └── site.env          # stub — deploy credentials are not recoverable
└── photos/
    ├── hero.jpg          # only if the site has one
    └── <slug>/
        ├── photogen.txt  # per-photo captions, in the site's photo order
        ├── <name>.png    # or .jpg / .webp, per -format
        └── <subdir>/     # only for recursive albums
```

Album descriptions are written inline in `albums.yaml` rather than into a
[descriptions file](CONFIGURATION.md), and every album gets `manual_sort_order: true` so the
rebuild reproduces the live site's photo order exactly.

## Building the result

The generated `albums.yaml` uses a relative base:

```yaml
bases:
  photos: photos
```

Relative bases resolve against the **working directory**, so `photogen` must run with `<dest>` as
its current directory:

```bash
cd ~/work/mysite
ddphotos photogen                 # Docker mode
ddphotos run
```

In developer mode, build a binary and run it from `<dest>`:

```bash
go build -o ~/work/mysite/photogen ./cmd/photogen
cd ~/work/mysite && ./photogen -config-dir config -resize -index -doit
```

## What it recovers

Everything the site publishes, which is nearly all the configuration:

| Recovered                                                   | From                                |
|-------------------------------------------------------------|-------------------------------------|
| Site id, name, URL, description, copyright, crawling, theme | `config.json`                       |
| Site title/subtitle/overview HTML                           | `html.json`                         |
| Custom CSS                                                  | `custom.css`                        |
| Hero banner                                                 | `hero.jpg`                          |
| Album slugs, names, descriptions, covers                    | `albums.json` and each `index.json` |
| Photo order, captions, subfolder structure                  | each `index.json`                   |
| Photo dates                                                 | each `index.json` (see below)       |

## What it cannot recover

- **Original photos.** Only the resized variants are published; the largest is `full/`, capped at
  1600px on the long edge. The rebuilt site looks right but its photos are that size. Everything
  else about them is faithful, including dates and captions, and the default PNG output adds no
  loss beyond that resize.
- **Passwords.** A password-protected album's `index.json` is encrypted, so those albums are
  skipped with a warning and left out of `albums.yaml`. If the whole site is password protected
  (`albums.enc.json`), the tool stops. The passwords and HMAC key are never recoverable.
- **`bases:` paths.** The real source directories live on the author's machine and are not
  published. The generated config uses a self-contained `photos/` directory instead, named after
  each album's slug rather than its original directory.
- **The hero's source image and crop anchor.** Only the finished 1600x250 banner is published, so
  it is reused as-is with `crop: center`.
- **`site.env`.** Deploy credentials are never published; a commented stub is written instead.

### About photo dates

`photogen` strips metadata when it publishes (smaller files, no GPS leak) and reads photo dates
only from EXIF, so downloaded photos would otherwise rebuild with no dates at all and the album
cards would lose their `dateSpan`. Each photo's `datetime` from `index.json` is therefore written
into the saved file as EXIF `DateTimeOriginal`, whichever format you choose. With `-format webp`
this is added to the RIFF container as its own chunk, so the compressed image data is copied
through untouched and nothing is re-encoded.

## Fidelity

Rebuilding [the sample site](https://ddphotos.donohoe.info) from its deployed copy reproduces
`albums.json` and `html.json` byte for byte, and every album's `index.json` matches on slug, title,
description, `dateSpan`, cover, photo order, ids, captions, dates, and orientation — in all three
formats. The two fields that differ are `width`/`height`, which now describe the 1600px variant
rather than the original, and `sourcePath`, which reflects the new local directory layout.
