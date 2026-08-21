/**
 * The "3 photos · 1 video · Apr 2024" line shown under an album, shared by the album
 * cards on the home page and the album page header so the two never drift.
 *
 * An album's videos are counted separately from its photos, but only when it has some:
 * a video-free album reads exactly as it always has ("1 photo", "12 photos"), and an
 * album that is all video drops the photo half entirely. An empty album is "0 photos".
 */
export function mediaCountText(photoCount: number, videoCount: number): string {
	const photos = `${photoCount} ${photoCount === 1 ? 'photo' : 'photos'}`;
	const videos = `${videoCount} ${videoCount === 1 ? 'video' : 'videos'}`;

	if (videoCount === 0) return photos;
	if (photoCount === 0) return videos;
	return `${photos} · ${videos}`;
}

/** mediaCountText with the album's date span appended, when it has one. */
export function albumMetaText(photoCount: number, videoCount: number, dateSpan?: string): string {
	const counts = mediaCountText(photoCount, videoCount);
	return dateSpan ? `${counts} · ${dateSpan}` : counts;
}
