import { decodeHTML } from 'entities/decode';

/**
 * Strip HTML tags and decode entities, for slots that can only hold plain text.
 *
 * Owner-authored HTML (site title/subtitle/overview, album descriptions, photogen.txt
 * captions) is rendered with `{@html}` where it appears as body text, but the same
 * strings also feed `alt`, `aria-label` and `<meta>` attributes, where markup would be
 * shown verbatim. Those slots run through here first.
 *
 * Entities are decoded so `&amp;` reads as `&`: sync escapes every & < > in an upstream
 * description, and handwritten captions use entities too. Decoding comes after the strip,
 * so an escaped `&lt;b&gt;` stays as text rather than becoming a tag that gets removed. The
 * result may hold a raw `<`, which is safe in every caller: Svelte escapes attribute values,
 * and PhotoSwipe sets `alt` as a DOM property.
 *
 * Not a sanitizer: it makes trusted HTML readable as text, and is not a defense against
 * untrusted input.
 *
 * Accepts undefined so callers can pass an optional field (a photo has no description
 * unless photogen.txt gave it one) straight through.
 */
export function stripTags(html: string | undefined): string {
	return html ? decodeHTML(html.replace(/<[^>]*>/g, '')) : '';
}
