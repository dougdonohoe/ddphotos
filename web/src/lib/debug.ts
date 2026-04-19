// Debug utility for tracing frontend code flow during development.
// Enable by setting VITE_DEBUG=1 before starting the dev server.
// All outputs are no-ops in production builds.
//
// SSR context (load functions): console.log goes directly to the terminal.
// Browser context: console.log goes to DevTools; sendBeacon relays to the terminal.

import { browser } from '$app/environment';

const enabled = import.meta.env.DEV && !!import.meta.env.VITE_DEBUG;

function format(arg: unknown): string {
	if (typeof arg === 'object' && arg !== null) return JSON.stringify(arg, null, 2);
	return String(arg);
}

export function debug(...args: unknown[]): void {
	if (!enabled) return;
	const ctx = browser ? 'browser' : 'ssr';
	const message = `[${ctx}] ${args.map(format).join(' ')}`;
	console.log(`[debug] ${message}`);
	if (!browser) return;
	navigator.sendBeacon('/api/debug', new Blob([JSON.stringify({ message })], { type: 'application/json' }));
}
