// hooks.server.ts — build-time hook (not the dev server).
//
// Album data is always served dynamically, never as static files:
//   dev:     Vite middleware in vite.config.ts serves /albums/** from DDPHOTOS_ALBUMS_DIR
//   build:   handleFetch below intercepts fetch() calls in load functions (JSON only)
//   runtime: Apache/Docker mounts the albums directory and serves everything directly
//
// handleFetch intercepts fetch('/albums/...') calls made in SvelteKit load functions
// during `npm run build` pre-rendering. In practice only JSON files are fetched here
// (config.json, albums.json, index.json, etc.) — images are strings in the rendered
// HTML, not server-side fetches, so the prerender crawler encounters them as 404s
// (suppressed via handleError below and handleHttpError in svelte.config.js).

import { readFileSync, existsSync } from 'fs';
import { join, resolve } from 'path';
import type { Handle, HandleFetch, HandleServerError } from '@sveltejs/kit';

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

// Suppress the noisy [404] log that SvelteKit's default handleError emits when the
// prerender crawler follows <img src="/albums/..."> cover URLs on the home page.
// Those assets are served at runtime (Apache/Docker) and are never pre-rendered;
// the 404 is expected and already silenced in svelte.config.js handleHttpError.
// For real errors, replicate the default: log status + path (+ stack for non-404).
export const handleError: HandleServerError = ({ status, error, event }) => {
	if (status === 404 && event.url.pathname.startsWith('/albums/')) return;
	const line = `\n\x1b[1;31m[${status}] ${event.request.method} ${event.url.pathname}\x1b[0m`;
	console.error(status === 404 ? line : `${line}\n${(error as Error)?.stack ?? error}`);
};
