// Types mirroring the Go structs in pkg/photogen/json.go.
// IMPORTANT: Keep in sync with Go types — if you change a JSON field in Go, update here too.

export interface PhotoSrc {
	grid: string;
	full: string;
	video?: string; // transcoded MP4, e.g. "video/clip.mp4"; videos only
}

export interface Photo {
	id: string;
	fileName: string;
	sourcePath: string; // relative path from album source base directory to the original source file
	width: number;
	height: number;
	orientation: string;
	datetime: string; // ISO 8601 datetime (camera local time, normalized to UTC); empty string if no EXIF date
	description?: string;
	kind?: 'video'; // omitted for stills; for a video, src.grid/src.full are the poster frame
	duration?: number; // seconds; videos only
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
	count: number; // total media items (photos + videos)
	videoCount?: number; // how many of `count` are videos; omitted when none are
	cover?: string;
	coverJpeg?: string;
	dateSpan: string;
	description?: string;
	encrypted?: boolean;
}

// Mirrors Go's NavLink in pkg/photogen/albums_config.go.
export interface NavLink {
	label: string;
	href: string;
	id?: string;
	newTab?: boolean;
}

// Mirrors Go's SiteConfig in pkg/photogen/json.go.
export interface SiteConfig {
	siteId: string;
	albumsFile: string;
	siteName: string;
	siteUrl: string;
	siteDescription: string;
	copyrightOwner: string;
	copyrightYear: number;
	allowCrawling?: boolean;
	keyId?: string;
	siteHint?: string;
	albumHints?: Record<string, string>;
	encrypted?: boolean; // true if any password is configured (site or per-album); false for a key-only passwords file
	heroImage?: string;
	customCss?: string;
	defaultTheme?: string;
	htmlFile?: string; // "html.json" or "html.enc.json" when HTML fields are configured
	albumNav?: NavLink[]; // replaces the album page's "← Albums" link when set
}

// Mirrors Go's SiteHTMLContent in pkg/photogen/json.go.
export interface SiteHtmlContent {
	siteTitleHtml?: string;
	siteSubtitleHtml?: string;
	siteOverviewHtml?: string;
}

// Wraps a value that may arrive encrypted or already decoded.
// Discriminate on the `encrypted` field to access the appropriate variant.
export type MaybeEncrypted<T> =
	| { encrypted: false; data: T }
	| { encrypted: true; blob: string; hint?: string };

// Data loaded by the home page load function.
export interface SiteData {
	siteId: string;
	albums: MaybeEncrypted<AlbumSummary[]>;
	html: MaybeEncrypted<SiteHtmlContent> | null;
}

// Data loaded by the album page load function.
export interface AlbumData {
	siteId: string;
	slug: string;
	albumTitle: string;
	dateSpan: string;
	description: string;
	photoIndex: number | null;
	album: MaybeEncrypted<AlbumIndex>;
}
