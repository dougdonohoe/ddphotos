// Types mirroring the Go structs in pkg/photogen/json.go.
// IMPORTANT: Keep in sync with Go types — if you change a JSON field in Go, update here too.

export interface PhotoSrc {
	grid: string;
	full: string;
}

export interface Photo {
	id: string;
	fileName: string;
	sourcePath?: string; // original relative path from album root (recursive subfolder photos only)
	width: number;
	height: number;
	orientation: string;
	date: string;
	description?: string;
	src: PhotoSrc;
}

export interface AlbumIndex {
	slug: string;
	title: string;
	description?: string;
	dateSpan?: string;
	cover?: string; // grid path of cover photo (e.g. "grid/foo.webp")
	photos: Photo[];
}

export interface AlbumSummary {
	slug: string;
	title: string;
	count: number;
	cover?: string;
	coverJpeg?: string;
	dateSpan: string;
	description?: string;
	encrypted?: boolean;
}

// Mirrors Go's SiteConfig in pkg/photogen/json.go.
export interface SiteConfig {
	siteId: string;
	albumsFile: string;
	siteHint?: string;
	albumHints?: Record<string, string>;
	encrypted?: boolean;
	heroImage?: string;
	customCss?: string;
}
