import { error } from '@sveltejs/kit';
import { browser } from '$app/environment';
import type { AlbumSummary, SiteHtmlContent, MaybeEncrypted, SiteData } from '$lib/types';

export async function load({ fetch, parent, url }) {
	// browser guard prevents url.searchParams access during pre-rendering
	if (browser && url.searchParams.has('boom')) {
		error(500, 'Whoops, the lens cap was on!');
	}

	const { siteConfig } = await parent();

	const albumsRes = await fetch(`/albums/${siteConfig.albumsFile}`);
	if (!albumsRes.ok) {
		error(albumsRes.status, 'Failed to load albums');
	}

	let html: MaybeEncrypted<SiteHtmlContent> | null = null;
	if (siteConfig.htmlFile) {
		const htmlRes = await fetch(`/albums/${siteConfig.htmlFile}`);
		if (htmlRes.ok) {
			if (siteConfig.htmlFile.endsWith('.enc.json')) {
				html = { encrypted: true, blob: await htmlRes.text() };
			} else {
				html = { encrypted: false, data: await htmlRes.json() as SiteHtmlContent };
			}
		}
	}

	let albums: MaybeEncrypted<AlbumSummary[]>;
	if (siteConfig.albumsFile.endsWith('.enc.json')) {
		albums = { encrypted: true, blob: await albumsRes.text(), hint: siteConfig.siteHint };
	} else {
		albums = { encrypted: false, data: await albumsRes.json() as AlbumSummary[] };
	}

	const siteData: SiteData = { siteId: siteConfig.siteId, albums, html };
	return { siteData };
}
