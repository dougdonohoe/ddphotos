<script lang="ts">
	import { theme } from '$lib/theme';
	import { onMount } from 'svelte';
	import ThemeToggle from '$lib/components/ThemeToggle.svelte';
	import { footerReady } from '$lib/stores';
	import { clearStoredKeys } from '$lib/crypto';
	import LogOut from 'lucide-svelte/icons/log-out';
	import Info from 'lucide-svelte/icons/info';
	import X from 'lucide-svelte/icons/x';
	import { page } from '$app/state';

	let { children, data } = $props();

	const hasHero = $derived(!!data.siteConfig?.heroImage && page.url.pathname === '/');
	const hasEncryption = $derived(!!data.siteConfig?.encrypted);
	// True when the current page has encrypted content not yet decrypted.
	// Home page uses siteData (post-refactor); album page still uses encryptedBlob directly.
	const pageEncrypted = $derived(
		!!(page.data.siteData?.albums?.encrypted || page.data.albumData?.album?.encrypted)
	);

	onMount(() => {
		// Set initial theme on mount
		document.documentElement.setAttribute('data-theme', $theme);
	});

	function formatBuildTime(iso: string): string {
		const d = new Date(iso);
		const month = d.getMonth() + 1;
		const day = d.getDate();
		const year = d.getFullYear();
		const time = d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true });
		return `${month}/${day}/${year} at ${time}`;
	}

	const builtOn = formatBuildTime(import.meta.env.VITE_BUILD_TIME);
	const gitDescribe = import.meta.env.VITE_GIT_DESCRIBE as string;
	const gitBranch = import.meta.env.VITE_GIT_BRANCH as string;
	const dockerImage = import.meta.env.VITE_DOCKER_IMAGE;
	const gitRepoSlug = import.meta.env.VITE_GIT_REPO_SLUG as string;
	const gitRepoUrl = import.meta.env.VITE_GIT_REPO_URL as string;
	const showBranch = gitBranch && gitBranch !== 'main';

	let showAbout = $state(false);

	function openAbout() { showAbout = true; }
	function closeAbout() { showAbout = false; }
	function handleOverlayClick(e: MouseEvent) {
		if (e.target === e.currentTarget) closeAbout();
	}
	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') closeAbout();
	}

	function logout() {
		clearStoredKeys();
		window.location.replace('/');
	}
</script>

<svelte:head>
	<link rel="icon" href="/favicon.ico" />
	<!-- Consumed by the inline script in app.html to scope cover CSS vars to the current build. -->
	<meta name="ddp-site-id" content={data.siteConfig?.siteId ?? ''} />
	<!-- Consumed by the inline script in app.html to set the default theme on first visit. -->
	<meta name="ddp-default-theme" content={data.siteConfig?.defaultTheme ?? 'dark'} />
</svelte:head>

<div class="app">
	<!--
		Custom CSS is injected here in the body (not <svelte:head>) so it lands after
		the Svelte <style data-sveltekit> block in document order, giving it cascade
		priority over scoped component styles and :root custom property declarations.
	-->
	{#if data.siteConfig?.customCss}
		<link rel="stylesheet" href="/albums/{data.siteConfig.customCss}" />
	{/if}
	<div
		class="top-controls"
		class:ready={!pageEncrypted || $footerReady}
		class:over-hero={hasHero}
	>
		{#if hasEncryption}
			<button class="control-btn" onclick={logout} aria-label="Log out">
				<LogOut size={16} aria-hidden="true" />
			</button>
		{/if}
		<ThemeToggle />
	</div>
	{@render children()}
	<footer class:ready={!pageEncrypted || $footerReady}>
		<div>{#if data.siteConfig?.copyrightYear && data.siteConfig.copyrightYear < new Date().getFullYear()}
				Copyright © {data.siteConfig.copyrightYear}-{new Date().getFullYear()}. {data.siteConfig?.copyrightOwner}.
			{:else}
				Copyright © {new Date().getFullYear()}. {data.siteConfig?.copyrightOwner}.
			{/if}</div>
		<div class="built-with">
			<button class="about-btn" onclick={openAbout} aria-label="About this site"><Info size={16} aria-hidden="true" /></button>
			Built with joy by <a class="footer-link" href="https://github.com/dougdonohoe/ddphotos" target="_blank" rel="noopener">DD Photos</a>
				· <a class="footer-link" href="/privacy">Privacy</a>
		</div>
	</footer>

	{#if showAbout}
		<div class="modal-overlay" role="presentation" onclick={handleOverlayClick}>
			<div class="modal" role="dialog" aria-modal="true" aria-labelledby="about-title">
				<div class="modal-header">
					<span id="about-title">About this site</span>
					<button class="modal-close" onclick={closeAbout} aria-label="Close"><X size={20} aria-hidden="true" /></button>
				</div>
				<dl class="modal-body">
					<dt>Built</dt>
					<dd>{builtOn}</dd>
					{#if dockerImage}
						<dt>Image</dt>
						<dd>{dockerImage}</dd>
					{/if}
					<dt>Version</dt>
					<dd>{gitDescribe}</dd>
					{#if showBranch}
						<dt>Branch</dt>
						<dd>{gitBranch}</dd>
					{/if}
					<dt>Source</dt>
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
					<dd><a href={gitRepoUrl} target="_blank" rel="noopener">{gitRepoSlug}</a></dd>
				</dl>
			</div>
		</div>
	{/if}
</div>

<svelte:window onkeydown={handleKeydown} />

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

	.top-controls {
		position: absolute;
		top: 0.7rem;
		right: 1rem;
		z-index: 20;
		opacity: 0;
		display: flex;
		gap: 0.3rem;
		align-items: center;
		border-radius: 20px;
		padding: 3px 6px;
	}

	.top-controls.ready {
		opacity: 1;
		transition: opacity 400ms ease-out;
	}

	.top-controls.over-hero {
		background: rgba(0, 0, 0, 0.4);
	}

	.control-btn {
		background: none;
		border: none;
		border-radius: 50%;
		width: 30px;
		height: 30px;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-color);
		transition: transform 0.2s;
		flex-shrink: 0;
		padding: 0;
	}

	.control-btn:hover {
		transform: scale(1.1);
	}

	.control-btn:focus:not(:focus-visible) {
		outline: none;
	}

	.top-controls.over-hero .control-btn {
		color: #fff;
	}

	.top-controls.over-hero :global(.theme-toggle) {
		color: #fff;
	}

	footer {
		text-align: center;
		padding: 1rem 1rem 4.5rem;
		color: var(--text-muted);
		font-size: 0.85rem;
		opacity: 0;
	}

	footer.ready {
		opacity: 1;
		transition: opacity 400ms ease-out;
	}

	.built-with {
		margin-top: 0.35rem;
	}

	.footer-link {
		color: #5a8ec0;
		text-decoration: none;
	}

	:global(:root[data-theme='light']) .footer-link {
		color: var(--link-color);
	}

	.footer-link:hover {
		text-decoration: underline;
	}

	.about-btn {
		background: none;
		border: none;
		padding: 0;
		margin-right: .3rem;
		cursor: pointer;
		color: #5a8ec0;
		display: inline-flex;
		align-items: center;
		vertical-align: -2px;
	}

	:global(:root[data-theme='light']) .about-btn {
		color: var(--link-color);
	}

	.about-btn:hover {
		opacity: 0.75;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.6);
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	:global(:root) .modal-overlay {
		background: rgba(200, 200, 200, 0.4);
	}

	:global(:root[data-theme='light']) .modal-overlay {
		background: rgba(0, 0, 0, 0.6);
	}

	.modal {
		background: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: 8px;
		width: 400px;
		max-width: calc(100vw - 2rem);
		box-shadow: 0 8px 32px var(--shadow-color);
	}

	.modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 1rem 0.75rem;
		border-bottom: 1px solid var(--border-color);
		font-weight: 600;
		font-size: 1.4rem;
		color: var(--text-color);
	}

	.modal-close {
		background: none;
		border: none;
		cursor: pointer;
		color: var(--text-muted);
		padding: 0.15rem 0.3rem;
		border-radius: 4px;
		transition: color 0.15s;
	}

	.modal-close:hover {
		color: var(--text-color);
	}

	.modal-body {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.5rem 1.25rem;
		padding: 1rem;
		margin: 0;
		font-size: 1.15rem;
	}

	.modal-body dt {
		color: var(--text-muted);
		font-weight: 700;
		white-space: nowrap;
	}

	.modal-body dt::after {
		content: ':';
	}

	.modal-body dd {
		margin: 0;
		color: var(--text-color);
		word-break: break-word;
	}

	@media (max-width: 480px) {
		.modal-body {
			font-size: 0.95rem;
		}
	}

	.modal-body a {
		color: var(--link-color);
	}
</style>
