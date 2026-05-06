#!/usr/bin/env python3
"""
Generate docs/deploy-tree.svg — colored directory tree diagram for DEPLOY.md.
Blue = from build/, green = from albums/, gray = present in build but ignored.

Usage:
  bin/gen-deploy-tree.py
  .venv/bin/python3 bin/gen-deploy-tree.py

Requires: rich  (uv pip install rich)
"""

import re
import subprocess
from pathlib import Path
from rich.align import Align
from rich.console import Console
from rich.columns import Columns
from rich.padding import Padding
from rich.terminal_theme import TerminalTheme
from rich.text import Text
from rich.tree import Tree

REPO_ROOT = Path(__file__).parent.parent
OUTPUT     = REPO_ROOT / "docs" / "deploy-tree.svg"
OUTPUT_PNG = OUTPUT.with_suffix(".png")

# ── Color palette ─────────────────────────────────────────────────────────────

BUILD  = "bold cornflower_blue"  # files from build/<site-id>/
ALBUMS = "bold green3"           # files from albums/<site-id>/
FADED  = "#999999"               # present in build but excluded from sync
DIR    = "bold"                  # dirs receiving files from both sources
ARROW  = "bold #007700"          # dark green for sync labels and arrow
PATH   = "bold #8844cc"          # purple for path targets (/, /albums/)

# ── Light theme: white background, near-black default text ────────────────────

LIGHT_THEME = TerminalTheme(
    (255, 255, 255),  # background
    (40, 40, 40),     # foreground (tree branch characters etc.)
    [
        (0, 0, 0), (180, 0, 0), (0, 160, 0), (160, 120, 0),
        (0, 60, 180), (120, 0, 120), (0, 120, 120), (180, 180, 180),
    ],
    [
        (80, 80, 80), (255, 60, 60), (0, 220, 0), (220, 180, 0),
        (60, 120, 255), (180, 60, 180), (0, 180, 180), (240, 240, 240),
    ],
)


def t(label: str, style: str) -> Text:
    return Text(label, style=style)


# ── Source trees ──────────────────────────────────────────────────────────────

def build_tree() -> Tree:
    root = Tree(t("build/<site-id>/", BUILD))
    root.add(t("index.html", BUILD))
    root.add(t("privacy.html", BUILD))
    root.add(t("albums.html", BUILD))
    root.add(t("about.json", BUILD))
    root.add(t("404.html", BUILD))
    root.add(t("robots.txt", BUILD))
    root.add(t("sitemap.xml (copy from albums)", ALBUMS))
    root.add(t("*.png, *.ico", BUILD))
    root.add(t("_app/", BUILD))
    albums = root.add(t("albums/", BUILD))
    albums.add(t("antarctica.html", BUILD))
    albums.add(t("hawaii.html", BUILD))
    albums.add(t("albums.json  (ignored)", FADED))
    albums.add(t("config.json  (ignored)", FADED))
    albums.add(t("html.json    (ignored)", FADED))
    ant = albums.add(t("antarctica/", FADED))
    ant.add(t("index.json   (ignored)", FADED))
    return root


def albums_tree() -> Tree:
    root = Tree(t("albums/<site-id>/", ALBUMS))
    root.add(t("albums.json", ALBUMS))
    root.add(t("config.json", ALBUMS))
    root.add(t("hero.jpg", ALBUMS))
    root.add(t("html.json", ALBUMS))
    root.add(t("sitemap.xml", ALBUMS))
    ant = root.add(t("antarctica/", ALBUMS))
    ant.add(t("cover.jpg", ALBUMS))
    ant.add(t("index.json", ALBUMS))
    full = ant.add(t("full/", ALBUMS))
    full.add(t("*.webp", ALBUMS))
    grid = ant.add(t("grid/", ALBUMS))
    grid.add(t("*.webp", ALBUMS))
    return root


# ── Merged result tree ────────────────────────────────────────────────────────

def result_tree() -> Tree:
    root = Tree(t("Web root /", DIR))
    root.add(t("index.html", BUILD))
    root.add(t("privacy.html", BUILD))
    root.add(t("albums.html", BUILD))
    root.add(t("about.json", BUILD))
    root.add(t("404.html", BUILD))
    root.add(t("robots.txt", BUILD))
    root.add(t("sitemap.xml", BUILD))
    root.add(t("*.png, *.ico", BUILD))
    root.add(t("_app/", BUILD))
    albums = root.add(t("albums/", DIR))
    albums.add(t("antarctica.html", BUILD))
    albums.add(t("hawaii.html", BUILD))
    albums.add(t("albums.json", ALBUMS))
    albums.add(t("config.json", ALBUMS))
    albums.add(t("hero.jpg", ALBUMS))
    albums.add(t("html.json", ALBUMS))
    albums.add(t("sitemap.xml", ALBUMS))
    ant = albums.add(t("antarctica/", ALBUMS))
    ant.add(t("cover.jpg", ALBUMS))
    ant.add(t("index.json", ALBUMS))
    full = ant.add(t("full/", ALBUMS))
    full.add(t("*.webp", ALBUMS))
    grid = ant.add(t("grid/", ALBUMS))
    grid.add(t("*.webp", ALBUMS))
    return root


# ── SVG post-processing: strip terminal chrome ────────────────────────────────

def strip_chrome(svg: str, top_margin: int = 10, extra_left: int = 20) -> str:
    """Remove title bar and window buttons; add white bg; adjust margins."""
    m = re.search(r'<g transform="translate\((\d+),\s*(\d+)\)" clip-path', svg)
    if not m:
        return svg
    content_x, chrome_y = int(m.group(1)), int(m.group(2))
    delta_y = chrome_y - top_margin
    new_x   = content_x + extra_left

    # Replace everything between </defs> and the content group with a plain
    # white background rect — removes the dark frame, title, and buttons.
    m_defs = re.search(r'</defs>\s*\n', svg)
    if m_defs:
        svg = (
            svg[: m_defs.end()]
            + f'\n    <rect fill="white" x="0" y="0" width="100%" height="100%"/>\n    '
            + svg[m.start() :]
        )

    # Shift content group: left by extra_left, up by delta_y
    svg = svg.replace(
        f'translate({content_x}, {chrome_y})',
        f'translate({new_x}, {top_margin})',
    )

    # Update viewBox: shrink height, widen for extra left margin
    svg = re.sub(
        r'viewBox="0 0 (\S+) (\S+)"',
        lambda mo: (
            f'viewBox="0 0 {float(mo.group(1)) + extra_left:.1f}'
            f' {float(mo.group(2)) - delta_y:.1f}"'
        ),
        svg,
    )

    return svg


# ── Render ────────────────────────────────────────────────────────────────────

console = Console(record=True, width=82)

console.print(Columns(
    [Padding(build_tree(), (0, 6, 0, 0)), albums_tree()],
    equal=True,
    expand=False,
))

console.print()
pass_line = Text("  ")
pass_line.append("Pass 1: sync →", style=ARROW)
pass_line.append(" /", style=PATH)
pass_line.append("           ")
pass_line.append("Pass 2: sync →", style=ARROW)
pass_line.append(" /albums/", style=PATH)
console.print(pass_line)
arrow_line = Text("                       ")
arrow_line.append("⬇", style=ARROW)
console.print(arrow_line)
console.print()

console.print(Padding(result_tree(), (0, 0, 0, 8)))

console.print()
legend = Text()
legend.append("  ■ ", style=BUILD)
legend.append("from build/   ")
legend.append("■ ", style=ALBUMS)
legend.append("from albums/   ")
legend.append("■ ", style=FADED)
legend.append("in build, ignored during sync")
console.print(Align(legend, align="center"))

svg = console.export_svg(theme=LIGHT_THEME)
svg = strip_chrome(svg)
OUTPUT.write_text(svg)
print(f"Written: {OUTPUT}")
subprocess.run(["rsvg-convert", "-f", "png", "-o", str(OUTPUT_PNG), str(OUTPUT)], check=True)
print(f"Written: {OUTPUT_PNG}")
