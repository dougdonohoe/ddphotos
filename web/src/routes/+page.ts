import { error } from '@sveltejs/kit';
import type { AlbumSummary, SiteHtmlContent } from '$lib/types';

export async function load({ fetch, parent }) {
	const { siteConfig } = await parent();

	const albumsRes = await fetch(`/albums/${siteConfig.albumsFile}`);
	if (!albumsRes.ok) {
		error(albumsRes.status, 'Failed to load albums');
	}

	const siteId = siteConfig.siteId;
	const siteHint = siteConfig.siteHint;

	// Load the HTML content file (html.json or html.enc.json) if configured.
	let siteHtml: SiteHtmlContent | null = null;
	let encryptedHtmlBlob: string | null = null;
	if (siteConfig.htmlFile) {
		const htmlRes = await fetch(`/albums/${siteConfig.htmlFile}`);
		if (htmlRes.ok) {
			if (siteConfig.htmlFile.endsWith('.enc.json')) {
				encryptedHtmlBlob = await htmlRes.text();
			} else {
				siteHtml = await htmlRes.json();
			}
		}
	}

	if (siteConfig.albumsFile.endsWith('.enc.json')) {
		const encryptedBlob = await albumsRes.text();
		return { albums: null as AlbumSummary[] | null, encryptedBlob, siteId, siteHint, siteHtml, encryptedHtmlBlob };
	}

	const albums: AlbumSummary[] = await albumsRes.json();
	return { albums, encryptedBlob: null as string | null, siteId, siteHint, siteHtml, encryptedHtmlBlob };
}
