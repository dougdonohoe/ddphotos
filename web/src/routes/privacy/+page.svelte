<script lang="ts">
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';

	let { data } = $props();
	const siteName = $derived(data.siteConfig?.siteName ?? 'DD Photos');
	const encrypted = $derived(data.siteConfig?.encrypted ?? false);
</script>

<svelte:head>
	<title>Privacy | {siteName}</title>
</svelte:head>

<main>
	<h1>{siteName} - Privacy</h1>

	<p>
		This site stores the following items in your browser's local storage:
	</p>
	<ul>
		<li><strong>Theme</strong> — remembers whether you last used light or dark mode.</li>
		<li><strong>Site ID</strong> — tracks the current site build so stale cached data is
			automatically cleared when the site is updated.</li>
	</ul>
	{#if encrypted}
	<p>
		Because this site uses password-protected albums, the following are also stored so you are
		not prompted on every visit:
	</p>
	<ul>
		<li><strong>Passwords</strong> — remembered site-wide and per-album passwords.</li>
		<li><strong>Album covers</strong> — cached cover image URLs for unlocked albums.</li>
	</ul>
	{/if}
	<p>
		All values stay in your browser and are never transmitted anywhere. No personal data is
		collected, no analytics are run, and no third-party tracking is used.
	</p>
	<p>
		To clear all stored data and return the site to its default state, visit
		<a href="/?clear">?clear</a>.
	</p>
	<p>
		DD Photos is open source. You can review the full source code on
		<a href="https://github.com/dougdonohoe/ddphotos" target="_blank" rel="noopener">GitHub</a>.
	</p>

	<a href="/" class="back-link"><ArrowLeft size={16} aria-hidden="true" />Back to albums</a>
</main>

<style>
	main {
		max-width: 600px;
		margin: 0 auto;
		padding: 1rem 2rem 2rem;
	}

	h1 {
		font-size: 2rem;
		margin: 0 0 1.5rem 0;
	}

	p,
	ul {
		line-height: 1.2;
		margin: 0 0 1rem 0;
	}

	ul {
		padding-left: 1.5rem;
	}

	li {
		margin-bottom: 0.5rem;
	}

	a {
		color: var(--link-color);
	}

	.back-link {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
	}
</style>
