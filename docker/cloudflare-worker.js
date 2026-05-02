// Cloudflare Pages Worker — handles photo permalink routing for DD Photos static deployments.
// Copied into the export root as _worker.js by `ddphotos export --cloudflare` (or export.sh --cloudflare).
// Without it, URLs like /albums/patagonia/15 return 404; the worker rewrites them to
// /albums/patagonia.html so SvelteKit can hydrate the album page and open the lightbox.
// See docs/DEPLOYMENT-SERVERS.md#cloudflare-pages-worker for details.
export default {
    async fetch(request, env) {
        const url = new URL(request.url);
        const path = url.pathname;

        // Photo permalink: /albums/slug/42 → serve /albums/slug.html
        // Equivalent to the CloudFront Function handler for S3+CloudFront deployments.
        const match = path.match(/^\/albums\/([^\/]+)\/\d+$/);
        if (match) {
            const newUrl = new URL(`/albums/${match[1]}.html`, url.origin);
            return env.ASSETS.fetch(newUrl.toString());
        }

        return env.ASSETS.fetch(request);
    }
};
