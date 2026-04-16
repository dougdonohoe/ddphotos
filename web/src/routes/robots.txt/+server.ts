export const prerender = true;

import type { RequestHandler } from '@sveltejs/kit';
import type { SiteConfig } from '$lib/types';

export const GET: RequestHandler = async ({ fetch }) => {
	const res = await fetch('/albums/config.json');
	const config: SiteConfig = await res.json();
	const allow = config.allowCrawling === true;
	const body = allow
		? `User-agent: *\nAllow: /\nSitemap: ${config.siteUrl}/albums/sitemap.xml\n`
		: `User-agent: *\nDisallow: /\n`;

	return new Response(body, {
		headers: { 'Content-Type': 'text/plain' }
	});
}
