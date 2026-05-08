<script lang="ts">
	import { browser } from '$app/environment';
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';

	let pathname = $derived(browser ? window.location.pathname : '');
</script>

<svelte:head>
	<title>404 - Not Found</title>
</svelte:head>

<SecondaryPage>
	<div class="error-content">
		<h1>404</h1>
		<p>Page <span class="missing-path">{pathname}</span> not found.</p>
		<a href="/" class="back-link"><ArrowLeft size={16} aria-hidden="true" />Back to albums</a>
	</div>
</SecondaryPage>
