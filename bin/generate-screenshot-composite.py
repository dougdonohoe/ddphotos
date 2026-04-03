#!/usr/bin/env python3
"""
Generate images/screenshots.png — a 2×2 composite of site screenshots.

Layout:
  home-dark    home-light
  album-dark   lightbox-dark

Usage:
  bin/generate-screenshot-composite.py [--input DIR] [--output FILE] [--scale N]

Requires: Pillow  (uv pip install Pillow)
"""

import argparse
import sys
from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter

# ── Tunable parameters ────────────────────────────────────────────────────────

PADDING = 24            # outer margin on all sides (px)
GAP     = 20            # gap between images (px)
BG      = (255, 255, 255)

# Drop shadow
SHADOW_OFFSET = (10, 10)   # (dx, dy) in px; 45° down-right
SHADOW_BLUR    = 7         # Gaussian blur radius; lower = tighter shadow
SHADOW_OPACITY = 0.5       # 0–1
SHADOW_COLOR   = (0, 0, 0)

# Inner stroke — Photoshop: 2px inside, black, opacity=20%
STROKE_WIDTH   = 2
STROKE_OPACITY = 0.20
STROKE_COLOR   = (0, 0, 0)

# ── Grid order ────────────────────────────────────────────────────────────────

GRID = [
    ["home-dark",  "home-light"],
    ["album-dark", "lightbox-dark"],
]

# ── Helpers ───────────────────────────────────────────────────────────────────

def add_stroke(img):
    """Draw a STROKE_WIDTH inner border on the image."""
    img = img.convert("RGBA")
    draw = ImageDraw.Draw(img)
    w, h = img.size
    alpha = round(STROKE_OPACITY * 255)
    color = STROKE_COLOR + (alpha,)
    for i in range(STROKE_WIDTH):
        draw.rectangle([i, i, w - 1 - i, h - 1 - i], outline=color)
    return img


def add_shadow(img):
    """
    Composite img over a drop shadow.
    Returns a new RGBA image sized to contain both the image and shadow bleed.
    """
    sdx, sdy = SHADOW_OFFSET
    iw, ih = img.size

    # How far the blurred shadow bleeds beyond the image in each direction
    bl = max(0, SHADOW_BLUR - sdx)   # bleed left
    br = max(0, SHADOW_BLUR + sdx)   # bleed right
    bt = max(0, SHADOW_BLUR - sdy)   # bleed top
    bb = max(0, SHADOW_BLUR + sdy)   # bleed bottom

    canvas_w = iw + bl + br
    canvas_h = ih + bt + bb

    # Shadow layer: solid rect at (image position + shadow offset), then blur
    shadow = Image.new("RGBA", (canvas_w, canvas_h), (0, 0, 0, 0))
    alpha = round(SHADOW_OPACITY * 255)
    solid = Image.new("RGBA", (iw, ih), SHADOW_COLOR + (alpha,))
    shadow.paste(solid, (bl + sdx, bt + sdy))
    shadow = shadow.filter(ImageFilter.GaussianBlur(radius=SHADOW_BLUR))

    # Composite: shadow first, then image on top
    result = Image.new("RGBA", (canvas_w, canvas_h), (0, 0, 0, 0))
    result.paste(shadow, (0, 0), shadow)
    result.paste(img, (bl, bt), img)

    return result


def make_cell(path, scale):
    """Load, scale, stroke, and shadow one screenshot. Returns RGBA cell image."""
    img = Image.open(path).convert("RGBA")
    if scale != 1.0:
        new_w = round(img.width  * scale)
        new_h = round(img.height * scale)
        img = img.resize((new_w, new_h), Image.LANCZOS)
    img = add_stroke(img)
    return add_shadow(img)


# ── Main ──────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Generate 2×2 screenshot composite")
    parser.add_argument("--input",  default="images/screenshots",
                        help="Directory containing input PNGs (default: images/screenshots)")
    parser.add_argument("--output", default="images/screenshots.png",
                        help="Output file (default: images/screenshots.png)")
    parser.add_argument("--scale",  type=float, default=0.0,
                        help="Scale factor for each image (default: auto-fit to ~1626px wide)")
    args = parser.parse_args()

    in_dir   = Path(args.input)
    out_path = Path(args.output)

    names = [name for row in GRID for name in row]
    paths = {name: in_dir / f"{name}.png" for name in names}
    missing = [str(p) for p in paths.values() if not p.exists()]
    if missing:
        for m in missing:
            print(f"ERROR: missing {m}", file=sys.stderr)
        sys.exit(1)

    # Determine scale from the first image
    sample    = Image.open(paths[names[0]])
    iw, ih    = sample.size
    sdx, sdy  = SHADOW_OFFSET
    bl        = max(0, SHADOW_BLUR - sdx)
    br        = max(0, SHADOW_BLUR + sdx)
    cell_overhead_w = bl + br   # px added to each cell by shadow bleed

    if args.scale > 0:
        scale = args.scale
    else:
        # Auto: solve for scale so canvas width ≈ 1626px
        target_w = 1626
        scale = (target_w - 2 * PADDING - GAP - 2 * cell_overhead_w) / (2 * iw)

    # Build cells (all source images are the same size, so all cells will be too)
    print(f"Scale: {scale:.4f}  ({round(iw*scale)}×{round(ih*scale)} per image)")
    cells = {name: make_cell(paths[name], scale) for name in names}

    cell_w, cell_h = next(iter(cells.values())).size
    ncols = len(GRID[0])
    nrows = len(GRID)

    canvas_w = 2 * PADDING + (ncols - 1) * GAP + ncols * cell_w
    canvas_h = 2 * PADDING + (nrows - 1) * GAP + nrows * cell_h

    canvas = Image.new("RGB", (canvas_w, canvas_h), BG)

    for r, row in enumerate(GRID):
        for c, name in enumerate(row):
            x = PADDING + c * (cell_w + GAP)
            y = PADDING + r * (cell_h + GAP)
            cell = cells[name]
            canvas.paste(cell, (x, y), cell)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(str(out_path), "PNG")
    print(f"Saved {out_path}  ({canvas_w}×{canvas_h})")


if __name__ == "__main__":
    main()
