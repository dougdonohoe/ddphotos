<script lang="ts">
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
</script>

<svelte:head>
	<title>404 - Not Found</title>
</svelte:head>

<SecondaryPage>
	<div class="error-content">
		<h1>404</h1>
		<p>Page not found</p>
		<a href="/">Back to albums</a>
	</div>
</SecondaryPage>

<style>
	.error-content {
		text-align: center;
	}

	.error-content h1 {
		font-size: 4rem;
		color: var(--text-muted);
	}

	.error-content p {
		font-size: 1.25rem;
	}
</style>
