// Types mirroring the Go structs in pkg/photogen/json.go.
// IMPORTANT: Keep in sync with Go types — if you change a JSON field in Go, update here too.

export interface PhotoSrc {
	grid: string;
	full: string;
}

export interface Photo {
	id: string;
	fileName: string;
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
