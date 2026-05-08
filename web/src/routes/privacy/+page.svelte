<script lang="ts">
	import Shield from 'lucide-svelte/icons/shield';
	import SecondaryPage from '$lib/components/SecondaryPage.svelte';

	let { data } = $props();
	const siteName = $derived(data.siteConfig?.siteName ?? 'DD Photos');
	const encrypted = $derived(data.siteConfig?.encrypted ?? false);
</script>

<svelte:head>
	<title>Privacy | {siteName}</title>
</svelte:head>

<SecondaryPage icon={Shield} iconColor="blue" title="Privacy" {siteName}>
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
		<a href="https://github.com/dougdonohoe/ddphotos" target="_blank" rel="noopener">GitHub↗</a>.
	</p>
</SecondaryPage>
