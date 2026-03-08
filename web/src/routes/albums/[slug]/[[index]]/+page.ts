import { error } from '@sveltejs/kit';

export async function load({ params, fetch }) {
	const [albumRes, albumsRes] = await Promise.all([
		fetch(`/albums/${params.slug}/index.json`),
		fetch('/albums/albums.json')
	]);
	if (!albumRes.ok) {
		error(albumRes.status, `Album "${params.slug}" not found`);
	}
	const album = await albumRes.json();
	const albumMeta = albumsRes.ok
		? (await albumsRes.json()).find((a: any) => a.slug === params.slug)
		: null;
	const photoIndex = params.index ? parseInt(params.index) - 1 : null;
	return { album, slug: params.slug, dateSpan: albumMeta?.dateSpan ?? '', description: albumMeta?.description ?? '', photoIndex };
}
