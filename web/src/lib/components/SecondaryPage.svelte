<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<main>
	{@render children()}
</main>

<style>
	main {
		max-width: 600px;
		margin: 0 auto;
		padding: 1rem 2rem 2rem;
	}

	main :global(h1) {
		font-size: 2rem;
		margin: 0 0 1.5rem 0;
	}

	main :global(p),
	main :global(ul) {
		line-height: 1.2;
		margin: 0 0 1rem 0;
	}

	main :global(ul) {
		padding-left: 1.5rem;
	}

	main :global(li) {
		margin-bottom: 0.5rem;
	}

	main :global(a) {
		text-decoration: none;
	}

	main :global(a:hover) {
		text-decoration: underline;
	}

	main :global(.back-link) {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
	}
</style>
