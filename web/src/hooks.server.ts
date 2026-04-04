// hooks.server.ts — build-time hook (not the dev server).
//
// handleFetch intercepts fetch() calls made in SvelteKit load functions (+page.ts)
// during `npm run build` prerendering. In practice only JSON files are fetched here
// (config.json, albums.json, index.json, etc.) — images are just strings in the
// rendered HTML and are never fetched server-side.
//
// Dev server asset serving (/albums/**) is handled separately in vite.config.ts.
// At runtime (Apache/Docker), neither applies — Apache serves everything directly.

import { readFileSync, existsSync } from 'fs';
import { join, resolve } from 'path';
import type { Handle, HandleFetch } from '@sveltejs/kit';

// Resolve albums dir the same way vite.config.ts does (repo-root-relative default).
function resolveAlbumsDir(): string {
	const albumsDir = process.env.DDPHOTOS_ALBUMS_DIR ?? 'albums';
	const siteId = process.env.DDPHOTOS_SITE_ID ?? 'sample';
	// __dirname is web/src; repo root is two levels up
	const root = resolve(import.meta.dirname, '..', '..');
	const base = albumsDir.startsWith('/') ? albumsDir : resolve(root, albumsDir);
	return join(base, siteId);
}
const albumsDir = resolveAlbumsDir();

// During prerender, SvelteKit fetch('/albums/...') calls never hit a real server.
// Intercept them and read directly from the filesystem instead.
export const handleFetch: HandleFetch = async ({ request, fetch }) => {
	const url = new URL(request.url);
	if (url.pathname.startsWith('/albums/')) {
		const filePath = join(albumsDir, url.pathname.slice('/albums/'.length));
		if (existsSync(filePath)) {
			const body = readFileSync(filePath);
			const ext = filePath.split('.').pop() ?? '';
			const contentType =
				ext === 'json' ? 'application/json' :
				ext === 'webp' ? 'image/webp' :
				'application/octet-stream';
			return new Response(body, { headers: { 'Content-Type': contentType } });
		}
	}
	return fetch(request);
};

export const handle: Handle = async ({ event, resolve }) => resolve(event);
