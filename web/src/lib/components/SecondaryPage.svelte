<script lang="ts">
	import type { Snippet } from 'svelte';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';

	let {
		icon: Icon,
		iconColor = 'blue',
		title,
		siteName,
		backHref = '/',
		children
	}: {
		// lucide-svelte uses legacy Svelte 4 class components; no non-deprecated Svelte type bridges both
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		icon: any;
		iconColor?: 'blue' | 'amber' | 'red' | 'gray';
		title: string;
		siteName: string;
		backHref?: string;
		children: Snippet;
	} = $props();
</script>

<main>
	<div class="card">
		<div class="card-header">
			<div class="card-icon card-icon--{iconColor}">
				<Icon size={32} aria-hidden="true" />
			</div>
			<div>
				<div class="card-site-name">{siteName}</div>
				<div class="card-title">{title}</div>
			</div>
		</div>
		<div class="card-body">
			{@render children()}
			<a href={backHref} class="card-back">
				<ArrowLeft size={12} aria-hidden="true" />Back to albums
			</a>
		</div>
	</div>
</main>

<style>
	main {
		max-width: 560px;
		margin: 0 auto;
		padding: 48px 24px 15px;
	}

	.card {
		border: 1px solid var(--border-color);
		border-radius: 12px;
		padding: 32px 36px;
		background: var(--bg-secondary);
	}

	.card-header {
		display: flex;
		align-items: flex-start;
		gap: 14px;
		margin-bottom: 20px;
		padding-bottom: 16px;
		border-bottom: 1px solid var(--border-color);
	}

	.card-icon {
		width: 44px;
		height: 44px;
		border-radius: 10px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	:global(.card-icon--blue)  { background: rgba(59, 130, 246, 0.1);  color: rgb(59, 130, 246); }
	:global(.card-icon--amber) { background: rgba(245, 158, 11, 0.1);  color: rgb(245, 158, 11); }
	:global(.card-icon--red)   { background: rgba(239, 68, 68, 0.1);   color: rgb(239, 68, 68); }
	:global(.card-icon--gray)  { background: rgba(107, 114, 128, 0.1); color: rgb(107, 114, 128); }

	.card-site-name {
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--text-muted);
		margin-bottom: 2px;
	}

	.card-title {
		font-size: 1.5rem;
		font-weight: 500;
		color: var(--text-color);
	}

	.card-body {
		font-size: 0.9rem;
		line-height: 1.7;
		color: var(--text-color);
	}

	.card-body :global(p),
	.card-body :global(ul) {
		margin: 0 0 0.75rem 0;
	}

	.card-body :global(ul) {
		padding-left: 1.5rem;
	}

	.card-body :global(li) {
		margin-bottom: 0.35rem;
	}

	.card-body :global(a) {
		text-decoration: none;
	}

	.card-body :global(a:hover) {
		text-decoration: underline;
	}

	.card-back {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		margin-top: .25rem;
		font-size: 12px;
		color: var(--link-color);
		text-decoration: none;
	}

	.card-back:hover {
		text-decoration: underline;
	}

	@media (max-width: 480px) {
		main {
			padding: 48px 16px 10px;
		}

		.card {
			padding: 24px 20px;
		}
	}
</style>
