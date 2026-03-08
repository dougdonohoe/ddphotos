<script lang="ts">
	let { data } = $props();

	const siteName = import.meta.env.VITE_SITE_NAME;
	const siteUrl = import.meta.env.VITE_SITE_URL;
	const siteDesc = import.meta.env.VITE_SITE_DESCRIPTION;
</script>

<svelte:head>
	<title>{siteName}</title>
	<meta property="og:title" content={siteName} />
	<meta property="og:description" content={siteDesc} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={siteUrl} />
	<meta property="og:image" content="{siteUrl}/albums/{data.albums[0].coverJpeg}" />
	<meta property="og:site_name" content={siteName} />
	<meta name="twitter:card" content="summary_large_image" />
	<link rel="canonical" href={siteUrl} />
</svelte:head>

<main>
	<h1>{siteName}</h1>
	<div class="albums">
		{#each data.albums as album}
			<a href="/albums/{album.slug}" class="album-card">
				<img src="/albums/{album.cover}" alt={album.title} />
				<div class="album-info">
					<h2>{album.title}</h2>
					{#if album.description}
						<p class="description">{album.description}</p>
					{/if}
					<p class="meta">{album.count} photos · {album.dateSpan}</p>
				</div>
			</a>
		{/each}
	</div>
</main>

<style>
	main {
		max-width: 1200px;
		margin: 0 auto;
		padding: 2rem;
	}

	h1 {
		margin-bottom: 2rem;
	}

	@media (max-width: 480px) {
		h1 {
			font-size: 1.8rem;
		}
	}

	.albums {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 1.5rem;
	}

	.album-card {
		text-decoration: none;
		color: inherit;
		border-radius: 8px;
		overflow: hidden;
		background: var(--bg-secondary);
		box-shadow: 0 2px 8px var(--shadow-color);
		transition: transform 0.2s, box-shadow 0.2s;
		display: flex;
		flex-direction: column;
	}

	.album-card:hover {
		transform: translateY(-4px);
		box-shadow: 0 4px 16px var(--shadow-color);
	}

	.album-card img {
		width: 100%;
		aspect-ratio: 3 / 2;
		object-fit: cover;
		background: var(--bg-secondary);
	}

	.album-info {
		padding: 1rem 1rem 0.5rem;
		display: flex;
		flex-direction: column;
		flex: 1;
	}

	.album-info h2 {
		margin: 0 0 0.5rem 0;
		font-size: 1.25rem;
	}

	.album-info p {
		margin: 0;
		color: var(--text-muted);
		font-size: 0.9rem;
	}

	.album-info .meta {
		margin-top: auto;
		padding-top: 0.4rem;
		text-align: right;
		font-style: italic;
		font-size: 0.85rem;
	}

	.album-info .description {
		margin-top: 0.3rem;
		font-size: 0.95rem;
		color: var(--text-color-2nd);
		opacity: 0.8;
	}
</style>
