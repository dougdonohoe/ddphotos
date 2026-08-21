# Custom CSS

DD Photos ships one styling hook: a single CSS file of your own, injected site-wide after
the built-in styles. It is enough to recolor the site, restyle album cards, or change how
captions look, without forking the code.

This page covers how to turn it on, how to iterate on it, and — the part that trips people
up — how to write a rule that actually wins against the built-in one.

- [Turning it on](#turning-it-on)
- [The edit/see-it loop](#the-editsee-it-loop)
- [Themes](#themes)
- [Level 1: redefine a custom property](#level-1-redefine-a-custom-property)
- [Level 2: override a rule](#level-2-override-a-rule)
- [Finding what to target](#finding-what-to-target)
- [Element reference](#element-reference)
- [Worked examples](#worked-examples)
- [Gotchas](#gotchas)

---

## Turning it on

Name your file in `albums.yaml` under `settings:`, as a path relative to the config
directory:

```yaml
settings:
  css: custom.css
```

`photogen` copies it to the site output as `custom.css`, and the frontend injects it on
every page:

```html
<link rel="stylesheet" href="/albums/custom.css" />
```

That `<link>` comes **after** the built-in styles, which matters for the cascade — see
[Level 2](#level-2-override-a-rule).

The file is never encrypted, even on a password-protected site. Do not put anything
private in a comment.

The `-css` flag overrides `settings.css` at run time, which is how the sample site and the
`custom-css` test variant inject
[`sample/config/custom-example.css`](../sample/config/custom-example.css) without
committing it into `albums.yaml`:

```bash
bin/photogen -css sample/config/custom-example.css -index -doit
```

Docker mode's `ddphotos init` writes a starter
[`config/custom.css`](../docker/init/config/custom.css) and already points `settings.css`
at it, so you can start editing immediately.

## The edit/see-it loop

The browser reads the **copy** in the albums output directory, not your config file, so an
edit is not live until `photogen` copies it across. The copy happens during index
generation, so you can skip resizing entirely and get a sub-second turnaround:

```bash
./ddphotos photogen -- -index -doit    # Docker mode
bin/photogen -index -doit              # developer mode
```

> Docker mode note: `ddphotos photogen` with no extra arguments adds `-doit` for you, but
> passing your own flags after `--` does not, so include `-doit` yourself. Without it, you
> get a dry run that reports `DRYRUN: would copy ... custom.css` and writes nothing.

Then reload the page. Both the dev server and a built site serve the file from
`/albums/custom.css`, so no restart or rebuild is needed. If your browser holds on to an
old copy, hard-refresh.

## Themes

The theme is an attribute on `<html>`, set by the theme toggle:

| Theme | Selector                    |
|-------|-----------------------------|
| Dark  | `:root[data-theme="dark"]`  |
| Light | `:root[data-theme="light"]` |

Dark is the default: the base values live on a bare `:root`, and the light theme redefines
them under `:root[data-theme='light']`. Write your own rules the same way — put the value
you want most of the time on `:root`, then override it for light:

```css
:root {
    --text-color-2nd: #2a9d8f;
}

:root[data-theme='light'] {
    --text-color-2nd: #1b6b62;
}
```

**Set both, or you will only change one theme.** All nine palette properties below are
defined twice by the built-in styles: once on `:root` and again on
`:root[data-theme='light']`. The second selector is more specific, so a custom rule that
only sets `:root` is overridden in light mode and appears to do nothing there. This is the
single most common reason a color override "only half works".

That trap is specific to the palette. For anything that is not one of those properties, a
rule with no theme selector applies to both themes, which is usually what you want.

## Level 1: redefine a custom property

This is the path to reach for first. The site's palette is a handful of CSS custom
properties on `:root`, and redefining one changes every place it is used at once. There is
no specificity fight, because you are not overriding a rule — you are changing an input to
one.

| Property            | Used for                                                                  |
|---------------------|---------------------------------------------------------------------------|
| `--bg-color`        | Page background                                                           |
| `--bg-secondary`    | Raised surfaces: album cards, modals, the back-to-top button              |
| `--text-color`      | Body text                                                                 |
| `--text-color-2nd`  | The accent (gold by default): album descriptions on cards and album pages |
| `--text-muted`      | De-emphasized text: photo counts, date ranges, the footer                 |
| `--border-color`    | Card and modal borders, dividers                                          |
| `--shadow-color`    | Box shadows                                                               |
| `--link-color`      | Default link color site-wide                                              |
| `--img-placeholder` | The block of color behind a photo before it fades in                      |

Two more are worth knowing about:

| Property        | Effect                                                                                                                         |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------|
| `--focus-color` | Outline color on a focused grid tile. Read with a `#0066cc` fallback and never defined, so it exists purely for you to set     |
| `--pswp-bg`     | Lightbox backdrop, PhotoSwipe's own variable. Set to `#000` on `.pswp` (not `:root`), so override it as `.pswp { --pswp-bg: }` |

## Level 2: override a rule

When no custom property covers what you want, you target a class directly. This is where
custom CSS gets confusing, and the reason is worth understanding rather than working around
by sprinkling `!important` everywhere.

### Why a rule can lose despite coming last

The site is built with Svelte, which **scopes** component styles by appending a generated
class to the selector. A rule written in the source as:

```css
.album-card {
    border-radius: 8px;
}
```

is compiled to:

```css
.album-card.svelte-1uha8ag {
    border-radius: 8px;
}
```

That is two classes, not one. Your `.album-card { ... }` is one class, so it loses on
specificity no matter that `custom.css` is injected afterward. Source order only breaks
ties between rules of *equal* specificity.

Not every built-in rule is scoped, though. Rules the source marks `:global(...)` compile
with no added class, and those you can override with a plain matching selector:

| Written in the source     | Compiles to                     | Beat it with                        |
|---------------------------|---------------------------------|-------------------------------------|
| `.album-card`             | `.album-card.svelte-HASH`       | `!important`, or add specificity    |
| `:global(.pswp-caption)`  | `.pswp-caption`                 | the same selector — you come later  |

A useful shortcut: everything belonging to the **lightbox** (`.pswp*`, `.pswp-caption`,
`.pswp-video`) is global and easy to override. Everything in the **page** (`.album-card`,
`.gallery`, `.photo`, `.photo-caption`, `.hero`) is scoped and needs help.

Two further wrinkles worth knowing:

- **Descendants cost nothing.** Svelte tags descendant parts of a selector with
  `:where(.svelte-HASH)`, which adds zero specificity. So `.photo:hover .photo-caption`
  compiles to `.photo.svelte-HASH:hover .photo-caption:where(.svelte-HASH)` — the extra
  class lands on `.photo` only.
- **Setting a property nobody set is free.** Specificity only decides between rules
  competing for the *same* property. If no built-in rule sets `display` on an element,
  your low-specificity rule setting `display` wins by default. This is why the caption
  example below needs no `!important`.

### How to win

In order of preference:

1. **Redefine a custom property** ([Level 1](#level-1-redefine-a-custom-property)). No
   contest at all.
2. **Use an `id`.** An id outranks any number of classes. `album_nav` links accept an `id:`
   in `customization.yaml` for exactly this reason — see
   [album_nav](CONFIGURATION.md#album_nav).
3. **Repeat the class.** `.album-card.album-card { ... }` is valid CSS that matches the
   same elements while counting as two classes, which ties the built-in
   `.album-card.svelte-HASH` — and a tie goes to whichever came last, which is you. Add a
   third repeat if you are up against a `:hover` rule.
4. **Reach for `!important`.** It always works, but it also means the next rule you write
   for the same element has to escalate too. Fine as a targeted tool, unpleasant as a habit.

### Watch the pseudo-classes

A `:hover` or `:focus-visible` rule carries one more class of specificity than its base
rule, so overriding the base is not enough — the hover rule still wins on hover. Repeat the
pseudo-class in your override:

```css
.pswp-caption a,
.pswp-caption a:hover,
.pswp-caption a:focus-visible {
    text-decoration-thickness: 3px;
}
```

Caption links are underlined at 1px and thicken to 2px on hover. Without the last two
selectors above, your 3px would apply at rest and then snap back to the built-in 2px the
moment the pointer touched it.

## Finding what to target

Open the site, right-click the thing you want to change, and choose **Inspect**. In the
Styles panel you will see the winning rule and its selector — including the
`.svelte-HASH` part if it is scoped, which tells you immediately which of the two cases
above you are in. Write your selector without the hash.

You can also test a rule live: edit it in DevTools until it looks right, then paste it into
`custom.css`. If a property shows struck through in DevTools, something more specific is
beating it.

## Element reference

The class names below are stable and safe to target. Anything not listed here is an
implementation detail that may be renamed.

**Scoped?** answers "will a plain one-class selector lose?" A `yes` means the built-in
rules carry at least one extra class of specificity — sometimes on the element itself
(`.album-card.svelte-HASH`), sometimes on an ancestor (`header.svelte-HASH
.site-subtitle`). Either way you need one of the tactics in
[How to win](#how-to-win). A `no` means a matching selector is enough, because
`custom.css` comes later.

**Targeting one album or one photo.** Two data attributes exist for this, and both add a
point of specificity for free:

| Attribute                  | On            | Example                              |
|----------------------------|---------------|--------------------------------------|
| `data-slug="<album-slug>"` | `.album-card` | `.album-card[data-slug='patagonia']` |
| `data-index="<0-based>"`   | `.photo`      | `.photo[data-index='0']`             |

**Site chrome (every page):**

| Selector                   | Element                                              | Scoped? |
|----------------------------|------------------------------------------------------|---------|
| `.top-controls`            | Container for the theme toggle and logout button     | yes     |
| `.theme-toggle`            | Dark/light toggle button                             | yes     |
| `.control-btn`             | Logout button on encrypted sites                     | yes     |
| `footer`                   | Site footer                                          | yes     |
| `.footer-link`             | Links in the footer (Privacy, source)                | yes     |
| `.about-btn`               | "About this site" button in the footer               | yes     |
| `.back-to-top`             | Floating back-to-top button                          | yes     |
| `.modal`, `.modal-overlay` | The About dialog                                     | yes     |

**Home page:**

| Selector                    | Element                                        | Scoped? |
|-----------------------------|------------------------------------------------|---------|
| `.hero`, `.hero-overlay`    | Hero banner and the text laid over it          | yes     |
| `.site-subtitle`            | `site_subtitle_html`                           | yes     |
| `.site-overview`            | `site_overview_html`                           | yes     |
| `.albums`                   | The album card grid                            | yes     |
| `.album-card`               | One album card                                 | yes     |
| `.album-info`               | Title/description/meta block inside a card     | yes     |
| `.album-info .description`  | Album description on a card                    | yes     |
| `.album-info .meta`         | Photo count and date range on a card           | yes     |

**Album page:**

| Selector              | Element                                          | Scoped? |
|-----------------------|--------------------------------------------------|---------|
| `.album-nav`          | Header nav (the `← Albums` link, or `album_nav`) | yes     |
| `header .description` | Album description under the title                | yes     |
| `header .meta`        | Photo count and date range under the title       | yes     |
| `.gallery`            | The justified photo grid                         | yes     |
| `.photo`              | One grid tile (a `<button>`)                     | yes     |
| `.photo-caption`      | Caption overlay on a grid tile                   | yes     |
| `.video-badge`        | Play badge and duration on a video tile          | yes     |

**Lightbox** (all global — a plain selector is enough):

| Selector                   | Element                                       |
|----------------------------|-----------------------------------------------|
| `.pswp`                    | The lightbox root                             |
| `.pswp__bg`                | The backdrop                                  |
| `.pswp__top-bar`           | Counter, zoom, copy-link and close controls   |
| `.pswp__button--arrow`     | Previous/next arrows                          |
| `.pswp__button--copy-link` | Copy-permalink button                         |
| `.pswp-caption`            | Caption under the photo                       |
| `.pswp-caption--video`     | Caption on a video slide (taller gradient)    |
| `.pswp-video`              | The `<video>` element                         |

**Password-protected sites:**

| Selector         | Element                                          | Scoped? |
|------------------|--------------------------------------------------|---------|
| `.overlay`       | Full-screen password prompt                      | yes     |
| `.hint`          | Password hint text in the prompt                 | yes     |
| `.ddp-lock-icon` | Lock icon on a locked album's card               | no      |

## Worked examples

### A class you invent yourself

The simplest case, and the one the starter `custom.css` demonstrates. The `*_html` settings
in `albums.yaml` accept arbitrary HTML, so you can introduce your own class and style it
with no specificity concerns at all — nothing else in the site defines it:

```yaml
settings:
  site_overview_html: 'Welcome to my <span class="accent">photo gallery</span>.'
```

```css
[data-theme="light"] .accent {
    color: #1a7f4b;
}

[data-theme="dark"] .accent {
    color: #4ade80;
}
```

This works for photo captions in `photogen.txt` too, which accept the same inline HTML.
See [HTML in captions](PHOTOGEN.md#html-in-captions).

### Recolor plus a shape change

[`sample/config/custom-example.css`](../sample/config/custom-example.css) does one of each.
The accent swap is a custom property, so it needs nothing special. The border radius fights
a scoped rule, so it needs `!important`:

```css
/* Swap the golden accent to teal, both themes */
:root {
    --text-color-2nd: #2a9d8f;
}

:root[data-theme='light'] {
    --text-color-2nd: #1b6b62;
}

/* Rounder album cards */
.album-card {
    border-radius: 16px !important;
}
```

### Styling captions differently in the grid and the lightbox

A caption is rendered in two places with two different class names, so each can be styled
on its own. This drops links from the grid entirely (they are not clickable there anyway,
since the whole tile opens the lightbox) while tinting them in the lightbox and holding the
underline at one weight, hover included:

```css
/* Grid overlay: drop the link, keep the surrounding prose */
.photo-caption a {
    display: none;
}

/* Lightbox: tint the links and stop the hover from thickening the underline */
.pswp-caption a,
.pswp-caption a:hover,
.pswp-caption a:focus-visible {
    color: #9fd3ff;
    text-decoration-thickness: 1px;
}
```

Both rules are lower specificity than the built-in ones, and both still work — for the two
different reasons from [Level 2](#level-2-override-a-rule). The grid rule sets `display`,
which no built-in rule sets, so there is nothing to lose to. The lightbox rules match a
`:global` selector exactly, so they win on source order, and the `:hover` and
`:focus-visible` selectors are repeated because the built-in hover rule would otherwise
outrank the base one and restore the 2px underline whenever the pointer was over the link.

Note that hiding a caption link is purely visual: the tile's `aria-label` still reads the
full caption text, so the visible and screen-reader text will differ.

### Always show grid captions

DD Photos reveals a grid caption on hover, and already shows it unconditionally on touch
devices via `@media (hover: none)`. To show it everywhere, only the resting `opacity` and
`transform` need overriding — the gradient, the white centered text, the positioning and
`pointer-events` are all already the default:

```css
.gallery .photo .photo-caption {
    opacity: 1;
    transform: none;
}
```

The base rule compiles to `.photo-caption.svelte-HASH`, which is two classes.
`.gallery .photo .photo-caption` is three, so it wins outright rather than by source order.

Because the caption has a `transition` on both properties, a change here fades in rather
than snapping. That also means DevTools shows the old value if you read the computed style
in the same tick you add the rule — wait for the transition before concluding it did not
work.

### Shift the hero's focal point

The hero is a hard 1600×250 crop, so a tall subject can end up out of frame. The image is
positioned by CSS rather than baked in, which means you can slide the visible window
without recropping:

```css
.hero {
    background-position: 10% center !important;
}
```

The `!important` is doing real work here: the built-in `.hero { background-position: center }`
compiles to two classes and would otherwise win. `.hero.hero { background-position: 10% center }`
is an equivalent fix without `!important`, per [How to win](#how-to-win) — both are
verified to work.

Note this only moves the crop window. To change the crop itself, use the `crop:` key on the
hero config, which takes `top`, `center` or `bottom`. See
[Hero Image](CONFIGURATION.md#hero-image).

### Fit an odd-shaped cover to its card

Album cards force covers into a 3:2 box with `object-fit: cover`, which center-crops. That
is right for most photos and wrong for a portrait or a panorama, where it cuts off the
subject. Switch that one album to `contain` so the whole image fits, letterboxed against
the card background:

```css
.album-card[data-slug='patagonia'] img {
    object-fit: contain;
}
```

`.album-card[data-slug='…'] img` and the built-in `.album-card.svelte-HASH img` have the
same specificity, so this wins on source order, no `!important` needed. Other album cards
are untouched.

## Gotchas

**Never write `.svelte-HASH` in a selector.** The hash is derived from the component's
style block, so it changes whenever that component's CSS changes — including on a routine
upgrade. A selector containing one silently stops matching.

**Grid captions do not receive clicks.** `.photo-caption` is `pointer-events: none` and
marked `inert`, because it lives inside the tile's `<button>`. Styling a link there to look
clickable will not make it clickable. See
[HTML in captions](PHOTOGEN.md#html-in-captions).

**The lightbox caption is positioned from JavaScript.** Its `bottom`, `left` and `right`
are measured and set inline on every slide change and resize to match the displayed photo's
box, so overriding those three in CSS will not stick. The same applies to `opacity` (it is
driven to `0` while a video plays and while zoomed) and to `padding-bottom` on a video
slide, which is set from the height of the browser's control bar. Colors, fonts, the
background gradient and the remaining padding are all yours.

**PhotoSwipe animates opacity.** Rules that set `opacity` on `.pswp__bg` or the top bar can
interfere with the open/close transitions.

**Mobile shows captions permanently.** Under `@media (hover: none)` grid captions are
always visible rather than revealed on hover, so check any caption change at a narrow
viewport too.

**Nothing validates your CSS.** `photogen` copies the file verbatim; a syntax error is
silently dropped by the browser at the point of the mistake. If a rule seems to do nothing,
check the ones above it in the file.

---

See also: [Custom CSS in Configuration](CONFIGURATION.md#custom-css) ·
[album_nav](CONFIGURATION.md#album_nav) ·
[HTML in captions](PHOTOGEN.md#html-in-captions)
