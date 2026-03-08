import { error } from '@sveltejs/kit';

export async function load({ fetch }) {
	const response = await fetch('/albums/albums.json');
	if (!response.ok) {
		error(response.status, 'Failed to load albums');
	}
	const albums = await response.json();
	return { albums };
}
