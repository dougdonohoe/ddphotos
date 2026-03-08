export const prerender = true;

export function GET() {
	const allow = import.meta.env.VITE_ALLOW_CRAWLING === 'true';
	const body = allow
		? `User-agent: *\nAllow: /\nSitemap: ${import.meta.env.VITE_SITE_URL}/albums/sitemap.xml\n`
		: `User-agent: *\nDisallow: /\n`;

	return new Response(body, {
		headers: { 'Content-Type': 'text/plain' }
	});
}
