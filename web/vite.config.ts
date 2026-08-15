import { readFileSync, existsSync, createReadStream, statSync, cpSync } from 'fs';
import { execSync } from 'child_process';
import { resolve, join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, createLogger } from 'vite';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Content types for files served under /albums during dev.
// Keep in sync with ALBUM_MIME_TYPES in src/hooks.server.ts, which does the same job for
// the prerender pass.
const ALBUM_MIME_TYPES: Record<string, string> = {
	json: 'application/json',
	webp: 'image/webp',
	jpg: 'image/jpeg',
	jpeg: 'image/jpeg',
	mp4: 'video/mp4',
	xml: 'application/xml',
	css: 'text/css'
};

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

const siteId = process.env.DDPHOTOS_SITE_ID ?? 'sample';

// Full path to the active site's album data: DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID
// Paths in defaults.env are repo-root-relative; absolute paths are used as-is.
function resolveAlbumsDir(): string {
	const albumsDir = process.env.DDPHOTOS_ALBUMS_DIR ?? 'albums';
	const root = resolve(__dirname, '..');
	const base = albumsDir.startsWith('/') ? albumsDir : resolve(root, albumsDir);
	return join(base, siteId);
}
const albumsDir = resolveAlbumsDir();

// Build metadata written by photogen: albums/.build/<site-id>.json
const buildMetaPath = join(dirname(albumsDir), '.build', `${siteId}.json`);

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
	const host = raw.includes('github.com')
		? 'https://github.com'
		: 'https://' + (raw.match(/[@/]([^/:@]+\.com)/)?.[1] ?? 'github.com');
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

// When DDPHOTOS_HIDE_NETWORK_URL is set (Docker runs via docker/do-run.sh), the
// "Network" URL Vite prints is the container's internal IP, which isn't reachable
// from the host. Suppress that one line while keeping host:true so the forwarded
// port still works. Unset (e.g. `make web-npm-run-dev` on a dev box) keeps the
// Network URL, which is useful for phone/LAN access.
function makeLogger() {
	if (!process.env.DDPHOTOS_HIDE_NETWORK_URL) return undefined;
	const logger = createLogger();
	const origInfo = logger.info.bind(logger);
	logger.info = (msg, options) => {
		// eslint-disable-next-line no-control-regex
		if (/Network:/.test(msg.replace(/\x1b\[[0-9;]*m/g, ''))) return;
		origInfo(msg, options);
	};
	return logger;
}

export default defineConfig({
	customLogger: makeLogger(),
	server: {
		host: true // Listen on all interfaces (allows phone access via IP)
	},
	plugins: [
		...httpsPlugin,
		sveltekit(),
		{
			name: 'static-root-files',
			closeBundle() {
				if (!existsSync(buildMetaPath)) return;
				let meta: { configDir?: string };
				try {
					meta = JSON.parse(readFileSync(buildMetaPath, 'utf-8'));
				} catch {
					return;
				}
				if (!meta.configDir) return;
				const staticDir = join(meta.configDir, 'static');
				if (!existsSync(staticDir)) return;
				const buildDir = resolve(__dirname, '..', 'build', siteId);
				cpSync(staticDir, buildDir, { recursive: true });
				console.log(`[static-root] copied ${staticDir} → ${buildDir}`);
			}
		},
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
						} catch {
							// Malformed debug payload: ignore it and still return 200 below.
						}
						res.writeHead(200, { 'Content-Type': 'application/json' });
						res.end('{"ok":true}');
					});
				});

				// Serve DDPHOTOS_ALBUMS_DIR/DDPHOTOS_SITE_ID at /albums/** during dev.
				//
				// Content-Type and Range handling are both required for video, and only
				// here: Apache and nginx do this for free, so a bug at this layer looks
				// like a broken player rather than a broken dev server. Without a
				// Content-Type the browser will not treat the response as media, and
				// without Range support seeking does nothing and Safari refuses to play
				// the file at all.
				server.middlewares.use('/albums', (req, res, next) => {
					const filePath = join(albumsDir, decodeURIComponent(req.url ?? '/'));
					let stat;
					try {
						stat = statSync(filePath);
					} catch {
						return next();
					}
					if (!stat.isFile()) return next();

					const ext = filePath.split('.').pop()?.toLowerCase() ?? '';
					const contentType = ALBUM_MIME_TYPES[ext];
					if (contentType) res.setHeader('Content-Type', contentType);
					res.setHeader('Accept-Ranges', 'bytes');

					// "bytes=START-END", either end optional. Anything else is served whole.
					const match = /^bytes=(\d*)-(\d*)$/.exec(req.headers.range ?? '');
					if (match && (match[1] !== '' || match[2] !== '')) {
						const size = stat.size;
						let start = match[1] === '' ? size - Number(match[2]) : Number(match[1]);
						let end = match[1] === '' || match[2] === '' ? size - 1 : Number(match[2]);
						start = Math.max(0, start);
						end = Math.min(end, size - 1);

						if (start > end) {
							res.statusCode = 416; // Range Not Satisfiable
							res.setHeader('Content-Range', `bytes */${size}`);
							return res.end();
						}

						res.statusCode = 206;
						res.setHeader('Content-Range', `bytes ${start}-${end}/${size}`);
						res.setHeader('Content-Length', String(end - start + 1));
						return createReadStream(filePath, { start, end }).pipe(res);
					}

					res.setHeader('Content-Length', String(stat.size));
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
