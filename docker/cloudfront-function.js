// CloudFront Function (viewer-request stage) for a DD Photos site served from S3.
//
// It replaces the web server routing rules that Apache (.htaccess) and nginx (nginx.conf)
// provide, rewriting the request URI before S3 is ever contacted. Pair it with CloudFront
// custom error responses mapping 403 and 404 to /404.html with a 404 status, since a
// private bucket answers 403 for a key that does not exist.
//
// See docs/DEPLOYMENT-SERVERS.md. bin/s3-edge-proxy.js runs this same file in front of a
// local S3 server so bin/s3-test.sh can verify the routing it implements.

function handler(event) {
    const request = event.request;
    const uri = request.uri;

    // Root
    if (uri === '/') {
        request.uri = '/index.html';
        return request;
    }

    // Trailing slash -> 301 to the canonical path (/albums/slug/ and /albums/slug/42/).
    // The location is relative, so it resolves against whatever scheme and host the
    // viewer used.
    if (uri.length > 1 && uri.slice(-1) === '/') {
        return {
            statusCode: 301,
            statusDescription: 'Moved Permanently',
            headers: { location: { value: uri.slice(0, -1) } }
        };
    }

    // Photo permalink: /albums/slug/42 → /albums/slug.html
    const photoPermalink = uri.match(/^\/albums\/([^\/]+)\/\d+$/);
    if (photoPermalink) {
        request.uri = '/albums/' + photoPermalink[1] + '.html';
        return request;
    }

    // Extensionless paths → pre-rendered .html page.
    // Unknown paths produce a 403/404 from S3, caught by custom_error_response → 404.html.
    if (!uri.includes('.')) {
        request.uri = uri + '.html';
        return request;
    }

    return request;
}
