import { error } from '@sveltejs/kit';
import type { AlbumIndex, AlbumSummary, AlbumData, MaybeEncrypted, SiteConfig } from '$lib/types';

export async function load({ params, fetch }) {
	// Consult config.json first to determine which files exist before fetching them.
	//
	// In the Vite dev server, fetching a missing static file whose URL matches a
	// SvelteKit page route causes recursive SSR instead of a 404:
	//
	//   /albums/{slug}/index.json   → matches /albums/[slug]/[[index]]
	//   /albums/albums.json         → matches /albums/[slug]
	//
	// Each match re-runs this load function, which fetches the same missing URL again,
	// looping until the process runs out of memory.
	//
	// config.json always exists; albums.json exists iff the site is not site-encrypted.
	// Reading them in order lets us fetch only the index file that actually exists.
	const configRes = await fetch('/albums/config.json');
	if (!configRes.ok) error(500, 'Failed to load site config');
	const config = (await configRes.json()) as SiteConfig;
	const siteEncrypted = config.albumsFile.endsWith('.enc.json');

	let albumMeta: AlbumSummary | null = null;
	let album: MaybeEncrypted<AlbumIndex>;

	if (siteEncrypted) {
		// Site-wide encryption is active: all album indexes are in index.enc.json,
		// unlocked by either the site password or a per-album password.
		const indexRes = await fetch(`/albums/${params.slug}/index.enc.json`);
		if (!indexRes.ok) error(404, `Album "${params.slug}" not found`);
		album = { encrypted: true, blob: await indexRes.text(), hint: config.albumHints?.[params.slug] };
	} else {
		// albums.json is plain; check the encrypted flag for this album before deciding
		// which index file to fetch (index.json vs index.enc.json).
		const albumsRes = await fetch('/albums/albums.json');
		if (albumsRes.ok) {
			const albumsData = (await albumsRes.json()) as AlbumSummary[];
			albumMeta = albumsData.find((a) => a.slug === params.slug) ?? null;
		}

		const indexEncrypted = albumMeta?.encrypted ?? false;
		const indexRes = await fetch(`/albums/${params.slug}/${indexEncrypted ? 'index.enc.json' : 'index.json'}`);
		if (!indexRes.ok) error(404, `Album "${params.slug}" not found`);

		if (indexEncrypted) {
			album = { encrypted: true, blob: await indexRes.text(), hint: config.albumHints?.[params.slug] };
		} else {
			let data: AlbumIndex;
			try {
				data = await indexRes.json();
			} catch {
				// Response was 200 but not JSON — SPA hosts (e.g. Surge) return the HTML shell
				// for missing files instead of a real 404. Treat it as not found.
				error(404, `Album "${params.slug}" not found`);
			}
			album = { encrypted: false, data };
		}
	}

	const albumData: AlbumData = {
		siteId: config.siteId,
		slug: params.slug,
		albumTitle: albumMeta?.title ?? params.slug.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
		dateSpan: albumMeta?.dateSpan ?? '',
		description: albumMeta?.description ?? '',
		photoIndex: params.index ? parseInt(params.index) - 1 : null,
		album,
	};
	return { albumData };
}
