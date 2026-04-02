import { error } from '@sveltejs/kit';
import type { AlbumSummary } from '$lib/types';

export async function load({ fetch, parent }) {
	const { siteConfig } = await parent();

	const albumsRes = await fetch(`/albums/${siteConfig.albumsFile}`);
	if (!albumsRes.ok) {
		error(albumsRes.status, 'Failed to load albums');
	}

	const siteId = siteConfig.siteId;
	const siteHint = siteConfig.siteHint;

	if (siteConfig.albumsFile.endsWith('.enc.json')) {
		const encryptedBlob = await albumsRes.text();
		return { albums: null as AlbumSummary[] | null, encryptedBlob, siteId, siteHint };
	}

	const albums: AlbumSummary[] = await albumsRes.json();
	return { albums, encryptedBlob: null as string | null, siteId, siteHint };
}
