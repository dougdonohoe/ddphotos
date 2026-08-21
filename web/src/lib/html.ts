/**
 * Strip HTML tags, for slots that can only hold plain text.
 *
 * Owner-authored HTML (site title/subtitle/overview, album descriptions, photogen.txt
 * captions) is rendered with `{@html}` where it appears as body text, but the same
 * strings also feed `alt`, `aria-label` and `<meta>` attributes, where markup would be
 * shown verbatim. Those slots run through here first.
 *
 * Not a sanitizer: it makes trusted HTML readable as text, and is not a defense against
 * untrusted input.
 *
 * Accepts undefined so callers can pass an optional field (a photo has no description
 * unless photogen.txt gave it one) straight through.
 */
export function stripTags(html: string | undefined): string {
	return html ? html.replace(/<[^>]*>/g, '') : '';
}
