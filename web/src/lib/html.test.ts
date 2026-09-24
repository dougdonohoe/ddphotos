import { describe, it, expect } from 'vitest';
import { stripTags } from '$lib/html';

describe('stripTags', () => {
	it('removes tags', () => {
		expect(stripTags('A <b>bold</b> <a href="/x">link</a>')).toBe('A bold link');
	});

	it('accepts undefined and empty', () => {
		expect(stripTags(undefined)).toBe('');
		expect(stripTags('')).toBe('');
	});

	// Captions are HTML, and sync escapes every & < > in an upstream description. The
	// plain-text slots (alt, aria-label, meta description) must show the characters, not
	// the entities, or a screen reader says "amp".
	it('decodes entities', () => {
		expect(stripTags('Tom &amp; Jerry')).toBe('Tom & Jerry');
		expect(stripTags('1 &lt; 2 &gt; 0')).toBe('1 < 2 > 0');
		expect(stripTags('Caf&eacute; &quot;Le Bar&quot; &#39;s')).toBe('Café "Le Bar" \'s');
		expect(stripTags('&#x1F600; &#8212;')).toBe('😀 —');
	});

	// Decoding after stripping, not before: escaped markup is text someone wrote, and
	// decoding first would turn it into a tag and delete it.
	it('keeps escaped markup as text', () => {
		expect(stripTags('<i>use &lt;b&gt; for bold</i>')).toBe('use <b> for bold');
	});
});
