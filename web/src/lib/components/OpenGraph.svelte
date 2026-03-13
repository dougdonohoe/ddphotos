<script lang="ts">
	interface Props {
		title: string;
		description: string;
		url: string;
		image: string;
	}

	let { title, description, url, image }: Props = $props();
</script>

<svelte:head>
	<title>{title}</title>
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={url} />
	<meta property="og:image" content={image} />
	<meta property="og:site_name" content={import.meta.env.VITE_SITE_NAME} />
	<meta name="twitter:card" content="summary_large_image" />
	<link rel="canonical" href={url} />
</svelte:head>
