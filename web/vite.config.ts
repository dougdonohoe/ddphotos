import { readFileSync, existsSync, createReadStream, statSync } from 'fs';
import { execSync } from 'child_process';
import { resolve, join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Parse a shell-style KEY=VALUE env file and apply entries to process.env.
// Skips blank lines and comments. Strips optional surrounding quotes from values.
// Existing env vars are never overwritten (first-write wins).
function loadEnvFile(path: string) {
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

// Load repo-wide defaults
function loadDefaultsEnv() {
	const path = resolve(__dirname, '..', 'config', 'defaults.env');
	if (!existsSync(path)) return;
	loadEnvFile(path);
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

function gitInfo(cmd: string): string {
	try {
		return execSync(cmd, { encoding: 'utf-8' }).trim();
	} catch {
		return 'unknown';
	}
}
process.env.VITE_GIT_DESCRIBE ??= gitInfo('git describe --tags --long --dirty --always');
process.env.VITE_GIT_BRANCH ??= gitInfo('git rev-parse --abbrev-ref HEAD');
process.env.VITE_DOCKER_IMAGE ??= '';

// Parse git remote URL into { slug: "owner/repo", url: "https://github.com/owner/repo" }.
// Handles both https://github.com/owner/repo[.git] and git@github.com:owner/repo[.git].
function gitRemote(): { slug: string; url: string } {
	const raw = gitInfo('git remote get-url origin');
	const match = raw.match(/[:/]([^/:]+\/[^/]+?)(?:\.git)?$/);
	if (!match) return { slug: raw, url: raw };
	const slug = match[1];
	const host = raw.includes('github.com') ? 'https://github.com' : 'https://' + (raw.match(/[@/]([^/:@]+\.com)/)?.[1] ?? 'github.com');
	return { slug, url: `${host}/${slug}` };
}
if (!process.env.VITE_GIT_REPO_SLUG) {
	const remote = gitRemote();
	process.env.VITE_GIT_REPO_SLUG = remote.slug;
	process.env.VITE_GIT_REPO_URL = remote.url;
}

// When VITE_HTTPS=1, load @vitejs/plugin-basic-ssl to serve the dev server over HTTPS.
// This is needed for mobile testing via LAN IP (crypto.subtle requires a secure context).
// Normal dev runs are unaffected.
const httpsPlugin = process.env.VITE_HTTPS
	? [(await import('@vitejs/plugin-basic-ssl')).default()]
	: [];

export default defineConfig({
	server: {
		host: true // Listen on all interfaces (allows phone access via IP)
	},
	plugins: [
		...httpsPlugin,
		sveltekit(),
		{
			name: 'albums-dev-server',
			configureServer(server) {
				// Log every HTTP request when VITE_LOG_REQUESTS=1 (useful for diagnosing
				// full-page-reload vs. client-side navigation on mobile).
				if (process.env.VITE_LOG_REQUESTS) {
					server.middlewares.use((req, _res, next) => {
						const ts = new Date().toISOString().slice(11, 23); // HH:MM:SS.mmm
						console.log(`[${ts}] ${req.method} ${req.url}`);
						next();
					});
				}

				// Debug logging endpoint: receives messages from debug() in src/lib/debug.ts
				// and prints them to the terminal. Active when VITE_DEBUG=1.
				server.middlewares.use('/api/debug', (req, res, next) => {
					if (req.method !== 'POST') return next();
					let body = '';
					req.on('data', (chunk: Buffer) => (body += chunk));
					req.on('end', () => {
						try {
							const { message } = JSON.parse(body);
							const ts = new Date().toISOString().slice(11, 23);
							console.log(`[${ts}] [debug] ${message}`);
						} catch {}
						res.writeHead(200, { 'Content-Type': 'application/json' });
						res.end('{"ok":true}');
					});
				});

				// Serve DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID at /albums/** during dev.
				server.middlewares.use('/albums', (req, res, next) => {
					const filePath = join(albumsDir, decodeURIComponent(req.url ?? '/'));
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
