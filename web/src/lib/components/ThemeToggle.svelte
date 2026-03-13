<script lang="ts">
	import { theme, toggleTheme } from '$lib/theme';
</script>

<button class="theme-toggle" onclick={toggleTheme} aria-label="Toggle theme">
	{#if $theme === 'dark'}
		☀️
	{:else}
		🌙
	{/if}
</button>

<style>
	.theme-toggle {
		background: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: 50%;
		width: 30px;
		height: 30px;
		font-size: 1.2rem;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: transform 0.2s;
		flex-shrink: 0;
	}

	.theme-toggle:hover {
		transform: scale(1.1);
	}
</style>
