import { error } from '@sveltejs/kit';
import type { AlbumSummary, SiteConfig } from '$lib/types';

export async function load({ fetch }) {
	const configRes = await fetch('/albums/config.json');
	if (!configRes.ok) {
		error(configRes.status, 'Failed to load site config');
	}
	const siteConfig: SiteConfig = await configRes.json();

	const albumsRes = await fetch(`/albums/${siteConfig.albumsFile}`);
	if (!albumsRes.ok) {
		error(albumsRes.status, 'Failed to load albums');
	}

	const siteId = siteConfig.siteId;

	if (siteConfig.albumsFile.endsWith('.enc.json')) {
		const encryptedBlob = await albumsRes.text();
		return { albums: null as AlbumSummary[] | null, encryptedBlob, siteId };
	}

	const albums: AlbumSummary[] = await albumsRes.json();
	return { albums, encryptedBlob: null as string | null, siteId };
}
