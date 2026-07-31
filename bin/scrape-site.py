#!/usr/bin/env python3
"""
Reconstruct a DD Photos source config directory from a deployed site.

A deployed site publishes everything needed to rebuild its inputs: config.json, albums.json,
html.json, custom.css, and a per-album index.json with slugs, titles, descriptions, covers,
per-photo captions and the exact photo order. The one thing it does not publish is the original
full-resolution photos — the largest published variant is full/<name>.webp (1600px long edge).

Photos are saved as PNG by default: decoding the WebP recovers its exact pixels, and PNG stores
them without further loss, so nothing degrades beyond the resize the site already published.
PNG is also the most widely readable option. Use -format webp to keep the downloaded bytes as-is
(far smaller), or -format jpeg if something downstream needs JPEG (adds a lossy generation).

photogen strips metadata when it publishes, and it reads photo dates only from EXIF, so the
downloaded photos would otherwise rebuild with no dates at all (no dateSpan on the album cards).
Each photo's `datetime` from index.json is therefore written into the saved file as EXIF.

Usage:
  bin/scrape-site.py <url> <dest-dir>            # dry run, writes nothing
  bin/scrape-site.py <url> <dest-dir> -doit      # actually download
  bin/scrape-site.py <url> <dest-dir> -doit -format webp -force -limit 2

Requires: rich, Pillow  (uv pip install -r requirements.txt)

See docs/SCRAPE.md for the full story, including what cannot be recovered.
"""

import argparse
import io
import json
import os
import struct
import sys
import urllib.error
import urllib.request
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

from PIL import Image
from rich.console import Console
from rich.progress import track

console = Console()

USER_AGENT = "ddphotos-scrape-site/1.0"
DOWNLOAD_WORKERS = 8

# Output formats, mapped to the file extension photogen will see. png first: it is the default.
FORMATS = {"png": "png", "jpeg": "jpg", "webp": "webp"}
JPEG_QUALITY = 95

# Deploy credentials cannot be recovered from a deployed site, so we emit a stub that mirrors
# config/site.example.env for the user to fill in.
SITE_ENV_STUB = """\
# Deploy infrastructure — consumed by bin/deploy-photos.sh
#
# None of this is recoverable from a deployed site; fill in your own values.

# If using rsync
#RSYNC_HOST=user@your-server.example.com
#RSYNC_DEST=/path/to/your/web/root/

# If using s3
#S3_BUCKET=your-s3-bucket

# If using CloudFront - to invalidate cache
#CLOUDFRONT_ID=YOUR_CLOUDFRONT_DISTRIBUTION_ID
"""


class ScrapeError(Exception):
    """Fatal problem that should stop the run with a readable message."""


# ── EXIF dates ────────────────────────────────────────────────────────────────
#
# photogen reads a photo's date only from EXIF (readDateTaken in pkg/photogen/exif.go looks at
# DateTimeOriginal, DateTimeDigitized, then DateTime) and strips all metadata when it publishes,
# so the WebPs we download normally arrive with no date. Without one, photogen emits no dateSpan
# and the album cards lose their dates. index.json still carries every photo's datetime, so we
# put it back.
#
# A WebP is a RIFF container, and the extended format has a slot for a raw TIFF/EXIF block, so
# the date can be added by rewriting the chunk list — the compressed image data is copied through
# untouched. See https://developers.google.com/speed/webp/docs/riff_container.


def build_exif_block(taken: datetime) -> bytes:
    """Build a minimal little-endian TIFF/EXIF block carrying just DateTimeOriginal.

    Layout: header (8) -> IFD0 (18) -> Exif IFD (18) -> the 20-byte date string.
    """
    stamp = taken.strftime("%Y:%m:%d %H:%M:%S").encode("ascii") + b"\x00"  # 20 bytes
    exif_ifd_offset, date_offset = 26, 44

    header = b"II*\x00" + struct.pack("<I", 8)
    # IFD0: one entry, the pointer to the Exif sub-IFD (tag 0x8769, type LONG).
    ifd0 = struct.pack("<H", 1) + struct.pack("<HHII", 0x8769, 4, 1, exif_ifd_offset)
    ifd0 += struct.pack("<I", 0)
    # Exif IFD: one entry, DateTimeOriginal (tag 0x9003, type ASCII).
    exif_ifd = struct.pack("<H", 1) + struct.pack("<HHII", 0x9003, 2, len(stamp), date_offset)
    exif_ifd += struct.pack("<I", 0)

    return header + ifd0 + exif_ifd + stamp


def exif_has_date(block: bytes) -> bool:
    """Report whether a TIFF/EXIF block already carries a date photogen would read.

    Mirrors readDateTaken in pkg/photogen/exif.go: DateTimeOriginal (0x9003) or
    DateTimeDigitized (0x9004) in the Exif sub-IFD, or DateTime (0x0132) in IFD0.
    """
    if block[:6] == b"Exif\x00\x00":  # some encoders write the JPEG APP1 prefix here
        block = block[6:]
    if block[:2] == b"II":
        order = "<"
    elif block[:2] == b"MM":
        order = ">"
    else:
        return False

    def read_ifd(offset: int, tags: set[int]) -> tuple[bool, int | None]:
        """Return (found a wanted tag, Exif sub-IFD offset if present)."""
        if not 0 < offset < len(block) - 2:
            return False, None
        count = struct.unpack_from(order + "H", block, offset)[0]
        exif_offset = None
        for i in range(count):
            entry = offset + 2 + i * 12
            if entry + 12 > len(block):
                break
            tag, _, _, value = struct.unpack_from(order + "HHII", block, entry)
            if tag in tags:
                return True, exif_offset
            if tag == 0x8769:
                exif_offset = value
        return False, exif_offset

    try:
        ifd0_offset = struct.unpack_from(order + "I", block, 4)[0]
        found, sub_ifd = read_ifd(ifd0_offset, {0x0132})
        if found:
            return True
        return sub_ifd is not None and read_ifd(sub_ifd, {0x9003, 0x9004})[0]
    except struct.error:
        return False


def iter_riff_chunks(payload: bytes):
    """Yield (fourcc, data) for each chunk in a RIFF payload (the bytes after 'WEBP')."""
    pos = 0
    while pos + 8 <= len(payload):
        fourcc = payload[pos:pos + 4]
        size = struct.unpack("<I", payload[pos + 4:pos + 8])[0]
        yield fourcc, payload[pos + 8:pos + 8 + size]
        pos += 8 + size + (size & 1)  # chunks are padded to an even length


def riff_chunk(fourcc: bytes, data: bytes) -> bytes:
    padding = b"\x00" if len(data) & 1 else b""
    return fourcc + struct.pack("<I", len(data)) + data + padding


def bitstream_dimensions(fourcc: bytes, data: bytes) -> tuple[int, int]:
    """Read canvas width/height out of a VP8 (lossy) or VP8L (lossless) bitstream header."""
    if fourcc == b"VP8 ":
        if len(data) < 10 or data[3:6] != b"\x9d\x01\x2a":
            raise ValueError("not a keyframe VP8 bitstream")
        width, height = struct.unpack("<HH", data[6:10])
        return width & 0x3FFF, height & 0x3FFF
    if fourcc == b"VP8L":
        if len(data) < 5 or data[0] != 0x2F:
            raise ValueError("not a VP8L bitstream")
        bits = struct.unpack("<I", data[1:5])[0]
        return (bits & 0x3FFF) + 1, ((bits >> 14) & 0x3FFF) + 1
    raise ValueError(f"unexpected image chunk {fourcc!r}")


def webp_with_exif(raw: bytes, taken: datetime) -> bytes:
    """Return `raw` with an EXIF chunk carrying `taken`, re-encoding nothing.

    Returns the input unchanged if it already carries a usable date (some older sites publish
    photos with metadata intact) or if it is not a WebP we recognize. An existing EXIF block
    with no date is replaced, since a dateless one would shadow the date we are adding.
    """
    if raw[:4] != b"RIFF" or raw[8:12] != b"WEBP":
        return raw

    chunks = list(iter_riff_chunks(raw[12:]))
    if any(fourcc == b"EXIF" and exif_has_date(data) for fourcc, data in chunks):
        return raw
    chunks = [(fourcc, data) for fourcc, data in chunks if fourcc != b"EXIF"]

    # A simple-format file (a lone VP8/VP8L chunk) has nowhere to put EXIF, so it has to be
    # promoted to the extended format by prepending a VP8X header describing the canvas.
    if not any(fourcc == b"VP8X" for fourcc, _ in chunks):
        image = next((c for c in chunks if c[0] in (b"VP8 ", b"VP8L")), None)
        if image is None:
            return raw
        try:
            width, height = bitstream_dimensions(*image)
        except (ValueError, struct.error):
            return raw
        has_alpha = any(fourcc == b"ALPH" for fourcc, _ in chunks)
        # VP8L can carry alpha internally, without a separate ALPH chunk.
        if image[0] == b"VP8L" and len(image[1]) >= 5:
            has_alpha = has_alpha or bool((struct.unpack("<I", image[1][1:5])[0] >> 28) & 1)
        flags = (0x10 if has_alpha else 0) | 0x08  # ALPHA, EXIF
        vp8x = struct.pack("<I", flags) + struct.pack("<I", width - 1)[:3]
        vp8x += struct.pack("<I", height - 1)[:3]
        chunks.insert(0, (b"VP8X", vp8x))
    else:
        chunks = [
            (fourcc, bytes([data[0] | 0x08]) + data[1:]) if fourcc == b"VP8X" else (fourcc, data)
            for fourcc, data in chunks
        ]

    # The container spec puts EXIF after the image data and before XMP.
    xmp = [c for c in chunks if c[0] == b"XMP "]
    body = [c for c in chunks if c[0] != b"XMP "]
    payload = b"".join(riff_chunk(f, d) for f, d in body + [(b"EXIF", build_exif_block(taken))] + xmp)

    return b"RIFF" + struct.pack("<I", len(payload) + 4) + b"WEBP" + payload


def convert_photo(raw: bytes, fmt: str, taken: datetime | None) -> bytes:
    """Re-encode a downloaded WebP as `fmt`, carrying `taken` as EXIF.

    Decoding a lossy WebP is deterministic and reproduces its exact pixels, so writing them to
    PNG loses nothing further. JPEG re-encodes and does lose a little, which is why it is not
    the default. WebP is passed through untouched — only an EXIF chunk is added.
    """
    if fmt == "webp":
        return webp_with_exif(raw, taken) if taken is not None else raw

    image = Image.open(io.BytesIO(raw))
    if fmt == "jpeg" and image.mode not in ("RGB", "L"):
        image = image.convert("RGB")  # JPEG cannot store an alpha channel

    options: dict = {}
    if taken is not None:
        block = build_exif_block(taken)
        # Pillow writes these bytes as the container's metadata payload verbatim. A JPEG APP1
        # segment must start with the "Exif\0\0" identifier; a PNG eXIf chunk must not.
        options["exif"] = b"Exif\x00\x00" + block if fmt == "jpeg" else block
    if fmt == "jpeg":
        options.update(quality=JPEG_QUALITY, subsampling=0)

    out = io.BytesIO()
    image.save(out, fmt.upper(), **options)
    return out.getvalue()


def parse_datetime(value: str) -> datetime | None:
    """Parse index.json's RFC3339 UTC datetime; returns None for the empty/undated case."""
    if not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


# ── Fetching ──────────────────────────────────────────────────────────────────


def fetch_bytes(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.read()
    except urllib.error.HTTPError as e:
        raise ScrapeError(f"GET {url} failed: HTTP {e.code} {e.reason}") from e
    except urllib.error.URLError as e:
        raise ScrapeError(f"GET {url} failed: {e.reason}") from e


def fetch_json(url: str):
    """Fetch and parse JSON.

    Some hosts serve the SPA HTML shell with a 200 instead of a 404 for missing files, so a
    successful response is not proof the file exists — insist that it parses as JSON.
    """
    raw = fetch_bytes(url)
    try:
        return json.loads(raw)
    except json.JSONDecodeError as e:
        raise ScrapeError(
            f"GET {url} did not return JSON (the server likely serves an HTML page for "
            f"missing files): {e}"
        ) from e


def normalize_base_url(url: str) -> str:
    """Reduce any URL from the site to its scheme://host[:port] root.

    Accepts https://host, https://host/, https://host/albums/config.json, and so on.
    """
    if "://" not in url:
        url = "https://" + url
    parts = urlsplit(url)
    if not parts.netloc:
        raise ScrapeError(f"cannot parse URL: {url}")
    return urlunsplit((parts.scheme, parts.netloc, "", "", ""))


# ── albums.yaml emission ──────────────────────────────────────────────────────


def yaml_scalar(value: str) -> str:
    """Quote a scalar for YAML using double quotes, escaping what needs escaping."""
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    escaped = escaped.replace("\r", "\\r").replace("\n", "\\n").replace("\t", "\\t")
    return f'"{escaped}"'


def yaml_block(key: str, value: str, indent: str) -> list[str]:
    """Render a folded block scalar (>-), which keeps long HTML and prose readable.

    A folded scalar rejoins its lines with single spaces, so it cannot represent runs of
    whitespace or leading/trailing space. Values that would not survive that round trip fall
    back to a quoted scalar, which is uglier on one long line but exact.
    """
    collapsed = " ".join(value.split())
    if not collapsed:
        return []
    if collapsed != value:
        return [f"{indent}{key}: {yaml_scalar(value)}"]
    lines = [f"{indent}{key}: >-"]
    # Wrap at a comfortable width; folded scalars rejoin the lines with single spaces.
    words, current = collapsed.split(" "), ""
    for word in words:
        candidate = f"{current} {word}".strip()
        if current and len(candidate) > 95:
            lines.append(f"{indent}  {current}")
            current = word
        else:
            current = candidate
    if current:
        lines.append(f"{indent}  {current}")
    return lines


def render_albums_yaml(config: dict, html: dict, albums: list[dict], base_name: str) -> str:
    """Build albums.yaml text by hand so key order, comments, and block scalars are ours."""
    out: list[str] = [
        "# Generated by bin/scrape-site.py from a deployed DD Photos site.",
        "#",
        f"# Source: {config.get('siteUrl') or '(unknown)'}",
        "#",
        f"# The '{base_name}' base below is a relative path, and relative bases resolve against the",
        "# working directory (not this config dir), so run photogen from the parent of this",
        "# directory:  cd <dest> && photogen -config-dir config",
        "#",
        "# Photos come from the site's 1600px published variants, not the originals — those are",
        "# never published. Deploy credentials and any passwords are not recoverable;",
        "# see docs/SCRAPE.md.",
        "",
        "settings:",
        f"  id: {config['siteId']}",
        f"  site_name: {yaml_scalar(config.get('siteName', ''))}",
    ]
    if config.get("siteUrl"):
        out.append(f"  site_url: {config['siteUrl']}")
    out.append(f"  site_description: {yaml_scalar(config.get('siteDescription', ''))}")
    out.append(f"  copyright_owner: {yaml_scalar(config.get('copyrightOwner', ''))}")
    out.append(f"  copyright_year: {config.get('copyrightYear', 0)}")
    out.append(f"  allow_crawling: {str(bool(config.get('allowCrawling'))).lower()}")

    # config.json omits defaultTheme when it is the built-in default (dark).
    if config.get("defaultTheme"):
        out.append(f"  default_theme: {config['defaultTheme']}")
    if config.get("customCss"):
        out.append("  css: custom.css")

    for json_key, yaml_key in (
        ("siteTitleHtml", "site_title_html"),
        ("siteSubtitleHtml", "site_subtitle_html"),
        ("siteOverviewHtml", "site_overview_html"),
    ):
        if html.get(json_key):
            out.extend(yaml_block(yaml_key, html[json_key], "  "))

    if config.get("heroImage"):
        out.extend([
            "  hero:",
            "    image: hero.jpg",
            f"    base: {base_name}",
            # The source image and its crop anchor are not published; only the finished
            # 1600x250 banner is, and re-cropping it at the same size is a no-op.
            "    crop: center",
        ])

    out.extend([
        "",
        "bases:",
        f"  {base_name}: {base_name}",
        "",
        "albums:",
    ])

    for i, album in enumerate(albums):
        if i:
            out.append("")
        out.append(f"  - slug: {album['slug']}")
        out.append(f"    name: {yaml_scalar(album['title'])}")
        out.append(f"    base: {base_name}")
        out.append(f"    source: {album['slug']}")
        if album.get("cover"):
            out.append(f"    cover: {album['cover']}")
        out.append("    manual_sort_order: true")
        if album.get("recurse"):
            out.append("    recurse: true")
        if album.get("description"):
            out.extend(yaml_block("description", album["description"], "    "))

    return "\n".join(out) + "\n"


# ── photogen.txt emission ─────────────────────────────────────────────────────


def stem(name: str) -> str:
    return name.rsplit(".", 1)[0] if "." in name else name


def caption_line(photo_stem: str, description: str) -> str:
    """One photogen.txt line: '<stem> <description>', or a bare stem when uncaptioned.

    The file is line-based, so newlines and tabs become spaces. Interior spacing is otherwise
    left alone, since captions are reproduced verbatim in the rebuilt site.
    """
    text = (description or "").translate(str.maketrans("\r\n\t", "   ")).strip()
    return f"{photo_stem} {text}" if text else photo_stem


def plan_photogen_files(photos: list[dict]) -> dict[str, list[str]]:
    """Group photos by their subdirectory and return {reldir: photogen.txt lines}.

    Photos in the album root go in "". For recursive albums, each subdirectory gets its own
    photogen.txt with bare stems, and the parent gets a bare subfolder-name line at the position
    where that subfolder's photos first appear — the placeholder form photogen's
    expandManualOrder understands.
    """
    files: dict[str, list[str]] = {"": []}

    for photo in photos:
        reldir = photo["reldir"]

        # Make sure every ancestor of this photo's directory is referenced from its own parent,
        # in the order the directories are first reached.
        segments = reldir.split("/") if reldir else []
        for depth in range(len(segments)):
            child = segments[depth]
            parent = "/".join(segments[:depth])
            entries = files.setdefault(parent, [])
            if child not in entries:
                entries.append(child)

        files.setdefault(reldir, []).append(
            caption_line(stem(photo["filename"]), photo.get("description", ""))
        )

    return {reldir: lines for reldir, lines in files.items() if lines}


# ── Album collection ──────────────────────────────────────────────────────────


def album_reldir(source_path: str) -> str:
    """Strip the leading album-directory segment from a sourcePath.

    photogen writes sourcePath as '<album dir basename>/<path within it>', so
    'uganda/subfolder/img.jpg' means the photo lives in subfolder/ inside the album.
    """
    parts = source_path.split("/")
    return "/".join(parts[1:-1]) if len(parts) > 2 else ""


def album_filename(source_path: str, full_path: str, ext: str) -> str:
    """Pick the on-disk name for a downloaded photo, with the output format's extension.

    The name comes from sourcePath, not from src.full: for a photo in a subdirectory photogen
    prefixes the published name with the sanitized subdirectory path (subfolder/img_840_d.jpg is
    published as subfolder_img_840_d.webp). Saving under the published name would make the next
    run prefix it a second time. sourcePath carries the original, unprefixed name.
    """
    name = (source_path or full_path).rsplit("/", 1)[-1]
    return f"{stem(name)}.{ext}"


def collect_album(base_url: str, summary: dict, limit: int | None, ext: str) -> dict:
    """Fetch an album's index.json and turn it into everything we need to write it out."""
    slug = summary["slug"]
    index = fetch_json(f"{base_url}/albums/{slug}/index.json")

    photos = []
    for entry in index.get("photos", []):
        full = entry.get("src", {}).get("full")
        if not full:
            console.print(f"  [yellow]WARN[/] {slug}: photo {entry.get('id')} has no full variant, skipping")
            continue
        source_path = entry.get("sourcePath", "")
        photos.append({
            "url": f"{base_url}/albums/{slug}/{full}",
            "filename": album_filename(source_path, full, ext),
            "reldir": album_reldir(source_path),
            "description": entry.get("description", ""),
            "grid": entry.get("src", {}).get("grid", ""),
            "taken": parse_datetime(entry.get("datetime", "")),
        })

    if limit is not None:
        photos = photos[:limit]

    # The album cover is published as a grid/ path; map it back to the file we download.
    cover = ""
    if index.get("cover"):
        for photo in photos:
            if photo["grid"] == index["cover"]:
                cover = "/".join(filter(None, [photo["reldir"], photo["filename"]]))
                break

    return {
        "slug": slug,
        "title": index.get("title") or summary.get("title", slug),
        "description": index.get("description") or summary.get("description", ""),
        "cover": cover,
        "recurse": any(p["reldir"] for p in photos),
        "photos": photos,
    }


# ── Downloading ───────────────────────────────────────────────────────────────


Job = tuple[str, Path, "datetime | None"]


def download_photos(jobs: list[Job], force: bool, fmt: str) -> tuple[int, int]:
    """Download (url, dest, taken) triples in parallel. Returns (downloaded, skipped)."""
    pending: list[Job] = []
    skipped = 0
    for candidate in jobs:
        _, target, _ = candidate
        if not force and target.exists() and target.stat().st_size > 0:
            skipped += 1
            continue
        pending.append(candidate)

    failures: list[str] = []

    def fetch_one(job: Job) -> None:
        url, dest, taken = job
        dest.parent.mkdir(parents=True, exist_ok=True)
        tmp = dest.with_suffix(dest.suffix + ".part")
        try:
            raw = fetch_bytes(url)
            # The hero banner is published as a JPEG rather than a WebP photo, and has no date;
            # it is stored as downloaded.
            if dest.name != "hero.jpg":
                raw = convert_photo(raw, fmt, taken)
            tmp.write_bytes(raw)
            tmp.replace(dest)
        except Exception as e:  # noqa: BLE001 - report and keep going
            tmp.unlink(missing_ok=True)
            failures.append(f"{url}: {e}")

    if pending:
        with ThreadPoolExecutor(max_workers=DOWNLOAD_WORKERS) as pool:
            # fetch_one returns nothing; consuming the results is what drives the bar.
            for _ in track(
                pool.map(fetch_one, pending),
                description=f"Downloading {len(pending)} photos",
                total=len(pending),
                console=console,
            ):
                pass

    if failures:
        console.print(f"[red]{len(failures)} download(s) failed:[/]")
        for failure in failures[:10]:
            console.print(f"  {failure}")
        if len(failures) > 10:
            console.print(f"  ... and {len(failures) - 10} more")

    return len(pending) - len(failures), skipped


# ── Main ──────────────────────────────────────────────────────────────────────


def scrape(url: str, dest: Path, doit: bool, force: bool, limit: int | None, fmt: str) -> int:
    base_url = normalize_base_url(url)
    console.print(f"Site: [bold]{base_url}[/]")

    config = fetch_json(f"{base_url}/albums/config.json")
    if not config.get("siteId"):
        raise ScrapeError(f"{base_url}/albums/config.json has no siteId — is this a DD Photos site?")

    albums_file = config.get("albumsFile", "albums.json")
    html_file = config.get("htmlFile", "")
    if albums_file.endswith(".enc.json") or html_file.endswith(".enc.json"):
        raise ScrapeError(
            "this site is encrypted at the site level (a password is required to read its album "
            "list). Passwords are out of scope for this tool."
        )

    console.print(f"Site ID: [bold]{config['siteId']}[/]  ({config.get('siteName', '')})")

    summaries = fetch_json(f"{base_url}/albums/{albums_file}")
    html = fetch_json(f"{base_url}/albums/{html_file}") if html_file else {}

    albums, skipped_albums = [], []
    for summary in summaries:
        if summary.get("encrypted"):
            skipped_albums.append(summary["slug"])
            continue
        albums.append(collect_album(base_url, summary, limit, FORMATS[fmt]))

    if not albums:
        raise ScrapeError("no readable albums found (every album is password protected)")

    # ── Assemble the full list of files to write ──────────────────────────────

    config_dir, photos_dir = dest / "config", dest / "photos"
    base_name = "photos"

    downloads: list[Job] = []
    for album in albums:
        for photo in album["photos"]:
            target = photos_dir / album["slug"]
            if photo["reldir"]:
                target = target / photo["reldir"]
            downloads.append((photo["url"], target / photo["filename"], photo["taken"]))

    if config.get("heroImage"):
        downloads.append((f"{base_url}/albums/hero.jpg", photos_dir / "hero.jpg", None))

    # ── Report ────────────────────────────────────────────────────────────────

    console.print()
    for album in albums:
        recurse = " [dim](recursive)[/]" if album["recurse"] else ""
        console.print(f"  {album['slug']:<32} {len(album['photos']):>4} photos{recurse}")
    for slug in skipped_albums:
        console.print(f"  [yellow]{slug:<32}    skipped — password protected[/]")

    total_photos = sum(len(a["photos"]) for a in albums)
    console.print()
    console.print(
        f"{len(albums)} album(s), {total_photos} photo(s), "
        f"{len(downloads)} file(s) to download into [bold]{dest}[/]"
    )
    note = {
        "png": "lossless, but roughly 7x the downloaded size",
        "jpeg": f"re-encoded at quality {JPEG_QUALITY}, so slightly lossy",
        "webp": "saved exactly as downloaded",
    }[fmt]
    console.print(f"Photo format: [bold]{fmt}[/] [dim]({note})[/]")

    if not doit:
        console.print()
        console.print("[yellow]Dry run — nothing written. Re-run with -doit to download.[/]")
        return 0

    # ── Write ─────────────────────────────────────────────────────────────────

    config_dir.mkdir(parents=True, exist_ok=True)
    photos_dir.mkdir(parents=True, exist_ok=True)

    console.print()
    downloaded, skipped = download_photos(downloads, force, fmt)
    console.print(f"Downloaded {downloaded} file(s), skipped {skipped} already present")

    for album in albums:
        album_dir = photos_dir / album["slug"]
        for reldir, lines in plan_photogen_files(album["photos"]).items():
            target = album_dir / reldir if reldir else album_dir
            target.mkdir(parents=True, exist_ok=True)
            (target / "photogen.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")

    (config_dir / "albums.yaml").write_text(
        render_albums_yaml(config, html, albums, base_name), encoding="utf-8"
    )
    (config_dir / "site.env").write_text(SITE_ENV_STUB, encoding="utf-8")

    # photogen reads config/defaults.env (relative to cwd) for the output location. Docker mode
    # sets these in the environment instead, and env wins, so writing it is harmless there.
    (config_dir / "defaults.env").write_text(
        "# Default albums output directory (relative to this site directory) and site ID.\n"
        "DDPHOTOS_ALBUMS_DIR=albums\n"
        f"DDPHOTOS_SITE_ID={config['siteId']}\n",
        encoding="utf-8",
    )

    if config.get("customCss"):
        css = fetch_bytes(f"{base_url}/albums/custom.css")
        (config_dir / "custom.css").write_bytes(css)

    # ── Next steps ────────────────────────────────────────────────────────────

    console.print()
    console.print("[bold]Wrote:[/]")
    console.print(f"  {config_dir / 'albums.yaml'}")
    console.print(f"  {config_dir / 'site.env'}  [dim](stub — deploy credentials are not recoverable)[/]")
    console.print(f"  {config_dir / 'defaults.env'}")
    if config.get("customCss"):
        console.print(f"  {config_dir / 'custom.css'}")
    console.print(f"  {photos_dir}/<slug>/  [dim](photos + photogen.txt captions)[/]")

    console.print()
    console.print(
        "[bold]Note:[/] photos come from the site's 1600px published variants, "
        "not the originals."
    )
    console.print()
    console.print("[bold]Build it (Docker mode):[/]")
    console.print(f"  cd {dest}")
    console.print("  ddphotos photogen")
    console.print("  ddphotos run")
    console.print()
    console.print("[bold]Build it (developer mode, from a DD Photos checkout):[/]")
    console.print(f"  go build -o {dest / 'photogen'} ./cmd/photogen")
    console.print(f"  cd {dest} && ./photogen -config-dir config -resize -index -doit")
    console.print(
        "  [dim]The 'photos' base is relative, so photogen must run with this directory as cwd.[/]"
    )
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Reconstruct a DD Photos config directory from a deployed site.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="Dry run by default; pass -doit to download.",
    )
    parser.add_argument("url", help="site URL, e.g. https://photos.example.com")
    parser.add_argument("dest", help="destination directory, e.g. ~/work/mysite")
    parser.add_argument("-doit", action="store_true", help="actually write files (default: dry run)")
    parser.add_argument("-force", action="store_true", help="re-download photos that already exist")
    parser.add_argument("-limit", type=int, metavar="N", help="cap photos per album (for testing)")
    parser.add_argument(
        "-format",
        choices=list(FORMATS),
        default="png",
        help="photo output format (default: png, which is lossless but large; "
             "webp keeps the downloaded bytes, jpeg re-encodes)",
    )
    args = parser.parse_args()

    dest = Path(os.path.expanduser(args.dest)).resolve()
    try:
        return scrape(args.url, dest, args.doit, args.force, args.limit, getattr(args, "format"))
    except ScrapeError as e:
        console.print(f"[red]Error:[/] {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())
