<script lang="ts">
	import { page } from '$app/state';
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
	import FileQuestion from 'lucide-svelte/icons/file-question';
	import AlertCircle from 'lucide-svelte/icons/alert-circle';

	let { status, message }: { status: number; message?: string } = $props();

	const siteName = $derived(page.data.siteConfig?.siteName ?? 'DD Photos');
	const icon = $derived(status === 404 ? FileQuestion : AlertCircle);
	const iconColor = $derived(status === 404 ? 'amber' : 'red') as 'amber' | 'red';
	const errorTitle = $derived(status === 404 ? '404 Not Found' : `${status} Error`);
</script>

<svelte:head>
	<title>{errorTitle} - {siteName}</title>
</svelte:head>

<SecondaryPage {icon} {iconColor} title={errorTitle} {siteName}>
	{#if status === 404}
		<p>It seems <span id="missing-path" class="missing-path"></span> was not found. Maybe that film
		hasn't been developed yet?</p>
	{:else}
		<p>Doh! {message || 'Something went wrong'}</p>
	{/if}
	<script>
		const el = document.getElementById('missing-path');
		if (el) el.textContent = location.pathname;
	</script>
</SecondaryPage>
