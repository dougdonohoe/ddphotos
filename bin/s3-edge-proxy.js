//
// Test-only stand-in for the CloudFront edge, used by bin/s3-test.sh.
//
// A local S3 server serves keys and nothing else, so a site deployed to it 404s on every
// clean URL: /albums/antarctica is stored as albums/antarctica.html. In production a
// CloudFront Function does that rewriting at the viewer-request stage. This proxy runs
// that exact function -- docker/cloudfront-function.js, loaded verbatim so the file stays
// deployable as-is -- in front of the origin, which is what lets the post-deploy server
// and Playwright tests run against the deployed bytes.
//
// Index and error documents are left to the origin's own website support, matching the
// CloudFront custom error responses that map 403/404 to /404.html.
//
// Usage: node bin/s3-edge-proxy.js --port 3903 --origin localhost:3902 --bucket my-bucket
//

const fs = require('fs');
const http = require('http');
const path = require('path');
const vm = require('vm');

/** @param {string} name */
function arg(name) {
    const i = process.argv.indexOf('--' + name);
    return i === -1 ? '' : process.argv[i + 1] || '';
}

// get args
const port = Number(arg('port'));
const [originHost, originPortText] = arg('origin').split(':');
const originPort = Number(originPortText);
const bucket = arg('bucket');
if (!port || !originHost || !originPort || !bucket) {
    console.error('Usage: node bin/s3-edge-proxy.js --port N --origin HOST:PORT --bucket NAME');
    process.exit(1);
}

// Load the CloudFront Function. It is written for CloudFront's runtime, which has no
// module system, so evaluate the file and pull `handler` out of the sandbox. The third
// argument is the filename used in stack traces.
const fnPath = path.join(__dirname, '..', 'docker', 'cloudfront-function.js');

/**
 * What handler() hands back: the request with a rewritten `uri`, or a response of its own.
 *
 * @typedef {Object} CfResult
 * @property {string} [uri] the key to fetch from the origin
 * @property {number} [statusCode] set when the function answers the request itself
 * @property {string} [statusDescription]
 * @property {Object} [headers] response headers, as {name: {value: string}}
 */

/** @type {{handler?: function(Object): CfResult}} */
const sandbox = {};
vm.runInNewContext(fs.readFileSync(fnPath, 'utf8'), sandbox, fnPath);
const handler = sandbox.handler;
if (!handler) {
    console.error(`No handler() defined in ${fnPath}`);
    process.exit(1);
}

// Create the HTTP server
http.createServer((req, res) => {
    const url = new URL(req.url || '/', 'http://localhost');
    const result = handler({ request: { uri: url.pathname, querystring: url.search.slice(1) } });

    // The function answers some requests itself (redirects) instead of rewriting the URI
    if (result.statusCode) {
        const headers = {};
        for (const [name, header] of Object.entries(result.headers || {})) {
            headers[name] = header.value;
        }
        res.writeHead(result.statusCode, headers);
        res.end();
        return;
    }

    // The origin resolves the bucket from the Host header
    const originHeaders = { ...req.headers };
    originHeaders.host = bucket;
    const originPath = (result.uri || url.pathname) + url.search;
    // IntelliJ's bundled Node stubs reject this options object
    // `tsc --checkJs` against @types/node accepts it, as does Node itself.
    // noinspection JSCheckFunctionSignatures
    const upstream = http.request({
        host: originHost,
        port: originPort,
        path: originPath,
        method: req.method,
        headers: originHeaders
    }, (originRes) => {
        res.writeHead(originRes.statusCode || 502, originRes.headers);
        originRes.pipe(res);
    });

    upstream.on('error', (err) => {
        res.writeHead(502, { 'content-type': 'text/plain' });
        res.end(`edge proxy: ${err.message}\n`);
    });
    req.pipe(upstream);
}).listen(port, () => console.log(`Edge proxy on :${port} -> ${originHost}:${originPort} (${bucket})`));
