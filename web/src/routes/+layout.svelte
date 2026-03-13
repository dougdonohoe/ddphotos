<script lang="ts">
	import { theme } from '$lib/theme';
	import { onMount } from 'svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';

	let { children } = $props();

	onMount(() => {
		// Set initial theme on mount
		document.documentElement.setAttribute('data-theme', $theme);
	});

	function ordinal(n: number): string {
		const s = ['th', 'st', 'nd', 'rd'];
		const v = n % 100;
		return n + (s[(v - 20) % 10] ?? s[v] ?? s[0]);
	}

	function formatBuildTime(iso: string): string {
		const d = new Date(iso);
		const month = d.getMonth() + 1;
		const day = d.getDate();
		const year = d.getFullYear();
		const time = d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true });
		return `${month}/${day}/${year} at ${time}`;
	}

	const builtOn = formatBuildTime(import.meta.env.VITE_BUILD_TIME);
</script>

<svelte:head>
	<link rel="icon" href="/favicon.ico" />
</svelte:head>

<div class="app">
	<div class="theme-toggle-wrap">
		<ThemeToggle />
	</div>
	{@render children()}
	<footer>
		<div>Copyright © {import.meta.env.VITE_COPYRIGHT_YEAR}-{new Date().getFullYear()}. {import.meta.env.VITE_COPYRIGHT_OWNER}.</div>
		<div class="built-with">Built {builtOn} with <a href="https://github.com/dougdonohoe/ddphotos" target="_blank" rel="noopener">ddphotos</a>.</div>
	</footer>
</div>

<style>
	:global(:root) {
		--bg-color: #1a1a1a;
		--bg-secondary: #2a2a2a;
		--text-color: #f0f0f0;
		--text-color-2nd: #d0b81e;
		--text-muted: #999;
		--border-color: #333;
		--shadow-color: rgba(0, 0, 0, 0.3);
		--link-color: #88b4e7;
		--img-placeholder: #282828; /* dark grey, distinct from #1a1a1a page background */
	}

	:global(:root[data-theme='light']) {
		--bg-color: #ffffff;
		--bg-secondary: #f5f5f5;
		--text-color: #1a1a1a;
		--text-color-2nd: #a49326;
		--text-muted: #666;
		--border-color: #ddd;
		--shadow-color: rgba(0, 0, 0, 0.1);
		--link-color: #0066cc;
		--img-placeholder: #f0f0f0; /* light grey, distinct from #ffffff page background */
	}

	:global(body) {
		margin: 0;
		padding: 0;
		background-color: var(--bg-color);
		color: var(--text-color);
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
		transition: background-color 0.2s, color 0.2s;
	}

	:global(a) {
		color: var(--link-color);
	}

	.app {
		position: relative;
		min-height: 100vh;
	}

	.theme-toggle-wrap {
		position: absolute;
		top: 0.7rem;
		right: 1rem;
		z-index: 10;
	}

	footer {
		text-align: center;
		padding: 1rem 1rem;
		color: var(--text-muted);
		font-size: 0.85rem;
	}

	.built-with {
		margin-top: 0.35rem;
	}

	:global(:root) .built-with a {
		color: #5a8ec0;
	}

	:global(:root[data-theme='light']) .built-with a {
		color: var(--link-color);
	}
</style>
