<script lang="ts">
	import { theme, toggleTheme } from '#lib/theme.ts';
	import Sun from 'lucide-svelte/icons/sun';
	import Moon from 'lucide-svelte/icons/moon';
</script>

<button class="theme-toggle" onclick={toggleTheme} aria-label="Toggle theme">
	{#if $theme === 'dark'}
		<Sun size={16} aria-hidden="true" />
	{:else}
		<Moon size={16} aria-hidden="true" />
	{/if}
</button>

<style>
	.theme-toggle {
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

	.theme-toggle:hover {
		transform: scale(1.1);
	}

	.theme-toggle:focus:not(:focus-visible) {
		outline: none;
	}
</style>
