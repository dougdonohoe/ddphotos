// Cloudflare Pages Worker — handles URL routing for DD Photos static deployments.
// Copied into the export root as _worker.js by `ddphotos export --cloudflare` (or export.sh --cloudflare).
// Handles photo permalinks (/albums/slug/42 → /albums/slug.html), photo permalink trailing slash
// redirects (308), and root-level SPA fallback (/nope → index.html).
// See docs/DEPLOYMENT-SERVERS.md#cloudflare-pages-worker for details.
export default {
    async fetch(request, env) {
        const url = new URL(request.url);
        const path = url.pathname;

        // Trailing slash on photo permalinks: /albums/slug/42/ → 308 → /albums/slug/42
        const trailingSlash = path.match(/^(\/albums\/[^\/]+\/\d+)\/$/);
        if (trailingSlash) {
            return Response.redirect(new URL(trailingSlash[1], url.origin).toString(), 308);
        }

        // Photo permalink: /albums/slug/42 → serve /albums/slug.html
        // Equivalent to the CloudFront Function handler for S3+CloudFront deployments.
        const match = path.match(/^\/albums\/([^\/]+)\/\d+$/);
        if (match) {
            const newUrl = new URL(`/albums/${match[1]}.html`, url.origin);
            return env.ASSETS.fetch(newUrl.toString());
        }

        // Root-level SPA fallback: unknown single-segment paths (e.g. /nope) → index.html
        // Mirrors the CloudFront Function behavior; client-side router handles the 404 display.
        if (!path.includes('.') && path.indexOf('/', 1) === -1) {
            return env.ASSETS.fetch(new URL('/index.html', url.origin).toString());
        }

        return env.ASSETS.fetch(request);
    }
};
