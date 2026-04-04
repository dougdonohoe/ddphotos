import { readFileSync, existsSync, createReadStream, statSync } from 'fs';
import { resolve, join, dirname } from 'path';
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

// Load defaults.env (lower priority than site.env — only fills gaps)
function loadDefaultsEnv() {
	const path = resolve(__dirname, '..', 'config', 'defaults.env');
	if (!existsSync(path)) return;
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
loadDefaultsEnv();

// Full path to the active site's album data: DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID
// Paths in defaults.env are repo-root-relative; absolute paths are used as-is.
function resolveAlbumsDir(): string {
	const albumsDir = process.env.DDPHOTOS_ALBUMS_DIR ?? 'albums';
	const siteId = process.env.DDPHOTOS_SITE_ID ?? 'sample';
	const root = resolve(__dirname, '..');
	const base = albumsDir.startsWith('/') ? albumsDir : resolve(root, albumsDir);
	return join(base, siteId);
}
const albumsDir = resolveAlbumsDir();

process.env.VITE_BUILD_TIME = new Date().toISOString();

export default defineConfig({
	server: {
		host: true // Listen on all interfaces (allows phone access via IP)
	},
	plugins: [
		sveltekit(),
		{
			name: 'albums-dev-server',
			configureServer(server) {
				// Serve DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID at /albums/** during dev.
				server.middlewares.use('/albums', (req, res, next) => {
					const filePath = join(albumsDir, req.url ?? '/');
					let stat;
					try { stat = statSync(filePath); } catch { return next(); }
					if (!stat.isFile()) return next();
					createReadStream(filePath).pipe(res);
				});
				// Reload browser when album data changes.
				server.watcher.add(albumsDir);
				server.watcher.on('change', (path) => {
					if (path.startsWith(albumsDir)) {
						server.ws.send({ type: 'full-reload' });
					}
				});
			}
		}
	]
});
