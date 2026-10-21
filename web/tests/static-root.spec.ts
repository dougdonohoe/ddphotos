import { test, expect } from '@playwright/test';

// Files in the site's config/static directory are served from the site root: copied into
// the build output for Apache/nginx, served straight from config/static by the dev server.
// The sample config ships humans.txt; a site without one skips.

test('config/static files are served from the site root', async ({ request }) => {
	const res = await request.get('/humans.txt');
	test.skip(res.status() === 404, 'no humans.txt in config/static');
	expect(res.status()).toBe(200);
	expect(res.headers()['content-type']).toMatch(/^text\/plain/);
	expect(await res.text()).not.toContain('<html');
});
