// Whether the viewer has hidden lightbox captions with the top-bar toggle.
//
// Deliberately a session choice, not a preference: kept in sessionStorage, so it carries
// across albums and reloads in the same tab but every new tab starts with captions on.
// clearStoredKeys only touches localStorage, so logout and build changes leave it alone.
// Read lazily on first use (keeping this module free of $app/environment and testable
// under plain Vitest), with a module-level copy for browsers where storage throws.
export const CAPTIONS_HIDDEN_KEY = 'ddp_captions_hidden';

let hidden: boolean | null = null;

function load(): boolean {
	try {
		return sessionStorage.getItem(CAPTIONS_HIDDEN_KEY) === '1';
	} catch {
		// Storage unavailable (or not a browser): captions default to shown.
		return false;
	}
}

export function getCaptionsHidden(): boolean {
	hidden ??= load();
	return hidden;
}

export function setCaptionsHidden(value: boolean): void {
	hidden = value;
	try {
		// Removed rather than stored as '0' when shown, so the default leaves no trace.
		if (value) sessionStorage.setItem(CAPTIONS_HIDDEN_KEY, '1');
		else sessionStorage.removeItem(CAPTIONS_HIDDEN_KEY);
	} catch {
		// Storage unavailable: the module-level copy still carries it for this page.
	}
}
