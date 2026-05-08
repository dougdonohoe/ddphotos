<script lang="ts">
	import { page } from '$app/state';
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
</script>

<svelte:head>
	<title>Error - DD Photos</title>
</svelte:head>

<SecondaryPage>
	<div class="error-content">
		<h1>{page.status}</h1>
		<p>{page.error?.message || 'Something went wrong'}</p>
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
