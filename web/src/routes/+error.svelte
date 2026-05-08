<script lang="ts">
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';

	let pathname = $derived(browser ? window.location.pathname : '');
</script>

<svelte:head>
	<title>{page.status} - DD Photos</title>
</svelte:head>

<SecondaryPage>
	<div class="error-content">
		<h1>{page.status}</h1>
		{#if page.status === 404}
			<p>Page <span class="missing-path">{pathname}</span> not found.</p>
		{:else}
			<p>{page.error?.message || 'Something went wrong'}</p>
		{/if}
		<a href="/" class="back-link"><ArrowLeft size={16} aria-hidden="true" />Back to albums</a>
	</div>
</SecondaryPage>
