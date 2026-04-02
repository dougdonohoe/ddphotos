// Enable pre-rendering for all pages (required for static adapter)
export const prerender = true;

// Trailing slashes are handled via the .htaccess file
export const trailingSlash = 'ignore';

import { error } from '@sveltejs/kit';
import type { SiteConfig } from '$lib/types';

export async function load({ fetch }) {
	const res = await fetch('/albums/config.json');
	if (!res.ok) {
		error(res.status, 'Failed to load site config');
	}
	const siteConfig: SiteConfig = await res.json();
	return { siteConfig };
}
