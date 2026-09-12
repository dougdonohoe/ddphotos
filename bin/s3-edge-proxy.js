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

const args = {};
for (let i = 2; i < process.argv.length; i += 2) {
    args[process.argv[i].replace(/^--/, '')] = process.argv[i + 1];
}
const port = Number(args.port);
const [originHost, originPort] = String(args.origin).split(':');
const bucket = args.bucket;
if (!port || !originHost || !originPort || !bucket) {
    console.error('Usage: node bin/s3-edge-proxy.js --port N --origin HOST:PORT --bucket NAME');
    process.exit(1);
}

// Load the CloudFront Function. It is written for CloudFront's runtime, which has no
// module system, so evaluate the file and pull `handler` out of the sandbox.
const fnPath = path.join(__dirname, '..', 'docker', 'cloudfront-function.js');
const sandbox = {};
vm.runInNewContext(fs.readFileSync(fnPath, 'utf8'), sandbox, { filename: fnPath });
const handler = sandbox.handler;

http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
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
    const upstream = http.request(
        {
            host: originHost,
            port: originPort,
            path: result.uri + url.search,
            method: req.method,
            headers: { ...req.headers, host: bucket }
        },
        (originRes) => {
            res.writeHead(originRes.statusCode, originRes.headers);
            originRes.pipe(res);
        }
    );
    upstream.on('error', (err) => {
        res.writeHead(502, { 'content-type': 'text/plain' });
        res.end(`edge proxy: ${err.message}\n`);
    });
    req.pipe(upstream);
}).listen(port, () => console.log(`Edge proxy on :${port} -> ${originHost}:${originPort} (${bucket})`));
