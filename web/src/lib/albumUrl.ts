/**
 * URL for a file photogen publishes under /albums, given its path relative to /albums
 * (e.g. "antarctica/grid/foo.webp").
 *
 * WebPs and MP4s are served as immutable, cached for a year without revalidation, but
 * keep their names when photogen regenerates them. The version token from index.json or
 * albums.json changes when that happens, and as a query string it gives the new file a
 * URL no browser has cached. Without a version, the plain path.
 */
export function albumFileUrl(path: string, version?: string): string {
	const url = `/albums/${path}`;
	return version ? `${url}?v=${encodeURIComponent(version)}` : url;
}
