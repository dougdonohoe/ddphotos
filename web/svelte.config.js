import adapter from '@sveltejs/adapter-static';
import { readdirSync } from 'fs';
import { join, resolve } from 'path';
import { fileURLToPath } from 'url';

const __dirname = fileURLToPath(new URL('.', import.meta.url));

/**
 * Return /albums/<slug> prerender entries for every album directory found under
 * DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID.
 *
 * This is necessary for encrypted builds: when all albums require a password the
 * SvelteKit crawler never finds links to album pages, so without explicit entries
 * those pages would not be pre-rendered.  Providing entries ensures each album gets
 * its own <slug>.html with the correct page skeleton; after JS hydration the
 * password prompt renders and the test (or user) can unlock the album normally.
 */
function albumEntries() {
	try {
		const albumsDir = process.env.DDPHOTOS_ALBUMS_DIR ?? 'albums';
		const siteId = process.env.DDPHOTOS_SITE_ID ?? 'sample';
		const root = resolve(__dirname, '..');
		const base = albumsDir.startsWith('/') ? albumsDir : resolve(root, albumsDir);
		const dir = join(base, siteId);
		return readdirSync(dir, { withFileTypes: true })
			.filter((d) => d.isDirectory())
			.map((d) => `/albums/${d.name}`);
	} catch {
		return [];
	}
}

const siteId = process.env.DDPHOTOS_SITE_ID ?? 'sample';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: `../build/${siteId}`,
			assets: `../build/${siteId}`,
			fallback: '200.html',
			precompress: false,
			strict: true
		}),
		paths: {
			relative: false
		},
		prerender: {
			entries: ['*', ...albumEntries()],
			handleHttpError: ({ path, message }) => {
				// Album assets (/albums/**) are served at runtime, not pre-rendered.
				// Ignore 404s the crawler encounters for images, JSON, etc.
				if (path.startsWith('/albums/')) return;
				throw new Error(message);
			}
		}
	}
};

export default config;
