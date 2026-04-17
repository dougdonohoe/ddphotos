// Encryption utilities matching pkg/photogen/encrypt.go.
// Uses PBKDF2-SHA256 (100k iterations) for key derivation and AES-256-GCM for
// encryption — the same parameters as the Go implementation.

const PBKDF2_ITERATIONS = 100_000;
const KEY_LENGTH = 256; // AES-256

function base64ToBytes(b64: string): Uint8Array<ArrayBuffer> {
	const binary = atob(b64);
	const buffer = new ArrayBuffer(binary.length);
	const bytes = new Uint8Array(buffer);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes;
}

// Decrypt a JSON blob produced by photogen's EncryptJSON.
// Throws if the password is wrong or the data is malformed.
export async function decryptJSON(encryptedBlob: string, password: string): Promise<unknown> {
	const { salt, iv, data } = JSON.parse(encryptedBlob) as {
		salt: string;
		iv: string;
		data: string;
	};
	const keyMaterial = await crypto.subtle.importKey(
		'raw',
		new TextEncoder().encode(password),
		'PBKDF2',
		false,
		['deriveKey']
	);
	const key = await crypto.subtle.deriveKey(
		{
			name: 'PBKDF2',
			salt: base64ToBytes(salt),
			iterations: PBKDF2_ITERATIONS,
			hash: 'SHA-256'
		},
		keyMaterial,
		{ name: 'AES-GCM', length: KEY_LENGTH },
		false,
		['decrypt']
	);
	const plaintext = await crypto.subtle.decrypt(
		{ name: 'AES-GCM', iv: base64ToBytes(iv) },
		key,
		base64ToBytes(data)
	);
	return JSON.parse(new TextDecoder().decode(plaintext));
}

// Returns the decrypted value, or null if the password is wrong or data is malformed.
export async function tryDecrypt(
	encryptedBlob: string,
	password: string
): Promise<unknown | null> {
	try {
		return await decryptJSON(encryptedBlob, password);
	} catch {
		return null;
	}
}

// localStorage key for the site-wide password, scoped to siteId.
export function siteKey(siteId: string): string {
	return `ddp_site_${siteId}`;
}

// localStorage key for a per-album password, scoped to siteId.
export function albumKey(siteId: string, slug: string): string {
	return `ddp_album_${siteId}_${slug}`;
}

export function getStoredPassword(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

export function storePassword(key: string, password: string): void {
	try {
		localStorage.setItem(key, password);
	} catch {
		// Ignore (e.g. private browsing with storage full)
	}
}

// localStorage key tracking the siteId of the current build.
// Used to detect when the user switches between different photogen builds (e.g. during
// development) so stale cover URLs (which have build-specific HMAC filenames) are cleared.
const SITE_ID_KEY = 'ddp_site_id';

// localStorage key for a per-album cover URL, scoped to siteId so different builds
// (with different HMAC keys and therefore different image filenames) don't share cover cache.
function coverKey(siteId: string, slug: string): string {
	return `ddp_cover_${siteId}_${slug}`;
}

export function storeAlbumCover(siteId: string, slug: string, url: string): void {
	try {
		localStorage.setItem(coverKey(siteId, slug), url);
	} catch {
		// Ignore
	}
}

export function getAlbumCover(siteId: string, slug: string): string | null {
	try {
		return localStorage.getItem(coverKey(siteId, slug));
	} catch {
		return null;
	}
}

// Call once per page load with the siteId and optional keyId from config.json.
// Clears all stale per-site entries (covers, passwords) when either value changes:
// siteId change = user switched to a different build; keyId change = HMAC key rotated,
// which renames all image files and makes cached cover URLs invalid.
export function syncSiteId(siteId: string, keyId?: string): void {
	try {
		const token = keyId ? `${siteId}:${keyId}` : siteId;
		const stored = localStorage.getItem(SITE_ID_KEY);
		if (stored === token) return;
		const toRemove: string[] = [];
		for (let i = 0; i < localStorage.length; i++) {
			const k = localStorage.key(i);
			if (k?.startsWith('ddp_cover_') || k?.startsWith('ddp_album_') || k?.startsWith('ddp_site_')) {
				toRemove.push(k);
			}
		}
		toRemove.forEach((k) => localStorage.removeItem(k));
		localStorage.setItem(SITE_ID_KEY, token);
	} catch {
		// localStorage not available
	}
}

// Try all stored ddp_album_{siteId}_* passwords against encryptedBlob.
// Returns { result, password } on the first match, or null.
// Used by the home page to silently unlock albums.enc.json when the user has
// already unlocked an individual album in this session.
export async function tryStoredAlbumPasswords(
	encryptedBlob: string,
	siteId: string
): Promise<{ result: unknown; password: string } | null> {
	const prefix = `ddp_album_${siteId}_`;
	try {
		for (let i = 0; i < localStorage.length; i++) {
			const key = localStorage.key(i);
			if (key?.startsWith(prefix)) {
				const password = localStorage.getItem(key);
				if (password) {
					const result = await tryDecrypt(encryptedBlob, password);
					if (result !== null) {
						return { result, password };
					}
				}
			}
		}
	} catch {
		// localStorage not available
	}
	return null;
}
