<script lang="ts">
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';

	let { status, message }: { status: number; message?: string } = $props();
</script>

<svelte:head>
	<title>{status} - DD Photos</title>
</svelte:head>

<SecondaryPage>
	<div class="error-content">
		<h1>{status}</h1>
		{#if status === 404}
			<p>Page <span id="missing-path" class="missing-path"></span> not found.</p>
		{:else}
			<p>{message || 'Something went wrong'}</p>
		{/if}
		<a href="/" class="back-link"><ArrowLeft size={16} aria-hidden="true" />Back to albums</a>
	</div>
	<script>
		const el = document.getElementById('missing-path');
		if (el) el.textContent = location.pathname;
	</script>
</SecondaryPage>
