<script lang="ts">
	import { onMount } from 'svelte';

	let { mobileOnly = false }: { mobileOnly?: boolean } = $props();

	let show = $state(false);

	function scrollToTop() {
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	onMount(() => {
		const onScroll = () => { show = window.scrollY > 600; };
		window.addEventListener('scroll', onScroll, { passive: true });
		return () => window.removeEventListener('scroll', onScroll);
	});
</script>

{#if show}
	<button class="back-to-top" class:mobile-only={mobileOnly} onclick={scrollToTop} aria-label="Back to top">
		↑
	</button>
{/if}

<style>
	.back-to-top {
		position: fixed;
		bottom: 1.2rem;
		right: 1.5rem;
		z-index: 50;
		background: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: 50%;
		width: 44px;
		height: 44px;
		font-size: 1.4rem;
		color: var(--text-color);
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		padding-bottom: 2px;
		box-shadow: 0 2px 8px var(--shadow-color);
		opacity: 0.7;
		transition: opacity 0.2s, transform 0.2s;
	}

	@media (min-width: 769px) {
		.back-to-top {
			padding-bottom: 5px;
		}

		.back-to-top.mobile-only {
			display: none;
		}
	}

	.back-to-top:hover {
		opacity: 1;
		transform: scale(1.1);
	}
</style>
