import adapter from '@sveltejs/adapter-static';
import { readdirSync } from 'fs';

/**
 * Return /albums/<slug> prerender entries for every album directory found under
 * static/albums/ (which may be a symlink to the active album set).
 *
 * This is necessary for encrypted builds: when all albums require a password the
 * SvelteKit crawler never finds links to album pages, so without explicit entries
 * those pages would not be pre-rendered.  Providing entries ensures each album gets
 * its own <slug>.html with the correct page skeleton; after JS hydration the
 * password prompt renders and the test (or user) can unlock the album normally.
 */
function albumEntries() {
	try {
		return readdirSync('static/albums', { withFileTypes: true })
			.filter((d) => d.isDirectory())
			.map((d) => `/albums/${d.name}`);
	} catch {
		return [];
	}
}

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: null,
			precompress: false,
			strict: true
		}),
		paths: {
			relative: false
		},
		prerender: {
			entries: ['*', ...albumEntries()]
		}
	}
};

export default config;
