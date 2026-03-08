import { readFileSync, existsSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const __dirname = dirname(fileURLToPath(import.meta.url));

function loadSiteEnv() {
	const path = process.env.SITE_ENV
		? resolve(process.env.SITE_ENV)
		: resolve(__dirname, '../config/site.env');
	if (!existsSync(path)) {
		console.error(`Error: ${path} not found. Set SITE_ENV=/path/to/site.env or copy config/site.example.env.`);
		process.exit(1);
	}
	for (const line of readFileSync(path, 'utf-8').split('\n')) {
		const trimmed = line.trim();
		if (!trimmed || trimmed.startsWith('#')) continue;
		const eq = trimmed.indexOf('=');
		if (eq < 0) continue;
		const key = trimmed.slice(0, eq).trim();
		let val = trimmed.slice(eq + 1).trim();
		if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
			val = val.slice(1, -1);
		}
		if (!(key in process.env)) process.env[key] = val;
	}
}
loadSiteEnv();
process.env.VITE_BUILD_TIME = new Date().toISOString();

export default defineConfig({
	server: {
		host: true // Listen on all interfaces (allows phone access via IP)
	},
	plugins: [
		sveltekit(),
		// Custom plugin to trigger browser reload when static files change.
		// By default, Vite doesn't reload the browser when files in static/ are modified
		// (e.g., albums.json). This plugin watches for changes in the static directory
		// and sends a full-reload signal to the browser via WebSocket.
		{
			name: 'static-reload',
			configureServer(server) {
				server.watcher.on('change', (path) => {
					if (path.includes('/static/')) {
						server.ws.send({ type: 'full-reload' });
					}
				});
			}
		}
	]
});
