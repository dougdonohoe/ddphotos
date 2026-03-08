<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head>
	<title>Error - {import.meta.env.VITE_SITE_NAME}</title>
</svelte:head>

<main>
	<h1>{page.status}</h1>
	<p>{page.error?.message || 'Something went wrong'}</p>
	<a href="/">Back to albums</a>
</main>

<style>
	main {
		max-width: 600px;
		margin: 4rem auto;
		padding: 2rem;
		text-align: center;
	}

	h1 {
		font-size: 4rem;
		margin: 0 0 1rem 0;
		color: var(--text-muted);
	}

	p {
		font-size: 1.25rem;
		margin: 0 0 2rem 0;
	}

	a {
		color: var(--link-color);
	}
</style>
