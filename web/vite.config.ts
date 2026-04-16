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

// Load the site-specific env file containing deploy and test variables.
// Resolution order:
//   1. $SITE_ENV (explicit override)
//   2. config/site.env (real config, not committed)
//   3. sample/config/site.env (fallback for out-of-the-box dev/tooling, with a warning)
function loadSiteEnv() {
	let path: string;
	if (process.env.SITE_ENV) {
		path = resolve(process.env.SITE_ENV);
	} else {
		const defaultPath = resolve(__dirname, '../config/site.env');
		const samplePath = resolve(__dirname, '../sample/config/site.env');
		if (existsSync(defaultPath)) {
			path = defaultPath;
		} else if (existsSync(samplePath)) {
			console.warn(`Warning: config/site.env not found, falling back to sample/config/site.env`);
			path = samplePath;
		} else {
			console.error(`Error: config/site.env not found. Set SITE_ENV=/path/to/site.env or copy config/site.example.env.`);
			process.exit(1);
		}
	}
	loadEnvFile(path);
}
loadSiteEnv();

// Load repo-wide defaults (lower priority than site.env — only fills gaps left by loadSiteEnv).
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
process.env.VITE_GIT_DESCRIBE = gitInfo('git describe --tags --long --dirty --always');
process.env.VITE_GIT_BRANCH = gitInfo('git rev-parse --abbrev-ref HEAD');

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
const remote = gitRemote();
process.env.VITE_GIT_REPO_SLUG = remote.slug;
process.env.VITE_GIT_REPO_URL = remote.url;

// When DEV_HTTPS=1, load @vitejs/plugin-basic-ssl to serve the dev server over HTTPS.
// This is needed for mobile testing via LAN IP (crypto.subtle requires a secure context).
// Normal dev runs are unaffected.
const httpsPlugin = process.env.DEV_HTTPS
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
				// Log every HTTP request when LOG_REQUESTS=1 (useful for diagnosing
				// full-page-reload vs. client-side navigation on mobile).
				if (process.env.LOG_REQUESTS) {
					server.middlewares.use((req, _res, next) => {
						const ts = new Date().toISOString().slice(11, 23); // HH:MM:SS.mmm
						console.log(`[${ts}] ${req.method} ${req.url}`);
						next();
					});
				}

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
