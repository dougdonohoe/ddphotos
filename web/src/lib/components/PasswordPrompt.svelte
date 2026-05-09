<script lang="ts">
	interface Props {
		prefix?: string;
		name: string;
		hint?: string;
		onunlock: (password: string) => void;
		shakeCount?: number;
	}

	import PasswordIcon from 'lucide-svelte/icons/key-square';

	let { prefix, name, hint, onunlock, shakeCount = 0 }: Props = $props();
	let password = $state('');
	let shaking = $state(false);
	let prevShakeCount = $state(0);

	$effect(() => {
		if (shakeCount > prevShakeCount) {
			prevShakeCount = shakeCount;
			password = '';
			shaking = true;
			setTimeout(() => {
				shaking = false;
			}, 500);
		}
	});

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (password.trim()) {
			onunlock(password.trim());
		}
	}
</script>

<div class="overlay">
	<div class="card" class:shake={shaking}>
		<div class="pw-icon">
			<PasswordIcon size={40} strokeWidth={1.75} aria-hidden="true" />
		</div>
		<h2>{prefix ? prefix + ' ' : ''}<span class="name">{name}</span><br>requires a password.</h2>
		<form onsubmit={handleSubmit}>
			<!-- svelte-ignore a11y_autofocus — intentional: this is an explicit password dialog -->
			<input
				type="password"
				placeholder="Password"
				bind:value={password}
				autocomplete="current-password"
				autofocus
			/>
			{#if hint}
				<p class="hint">Hint: <i>{hint}</i></p>
			{/if}
			<button type="submit">Unlock</button>
		</form>
	</div>
</div>

<style>
	.overlay {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 50vh;
		padding: 2rem;
	}

	.card {
		background: var(--bg-secondary);
		border-radius: 12px;
		box-shadow: 0 4px 24px var(--shadow-color);
		padding: 2.5rem 2rem;
		width: 100%;
		min-width: 360px;
		max-width: 360px;
		text-align: center;
	}

	.pw-icon {
		color: var(--text-muted);
		margin-bottom: 1rem;
	}

	h2 {
		margin: 0 0 1.5rem;
		font-size: 1.2rem;
		color: var(--text-color);
	}

	h2 .name {
		color: var(--text-color-2nd);
		font-weight: bold;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.hint {
		margin: 0;
		font-size: 0.85rem;
		color: var(--text-muted);
		text-align: left;
	}

	input[type='password'] {
		width: 100%;
		padding: 0.6rem 0.8rem;
		font-size: 1rem;
		border: 1px solid var(--border-color);
		border-radius: 6px;
		background: var(--bg-color);
		color: var(--text-color);
		box-sizing: border-box;
	}

	input[type='password']:focus {
		outline: none;
		border-color: var(--link-color);
	}

	button {
		padding: 0.6rem 1rem;
		font-size: 1rem;
		background: var(--link-color);
		color: white;
		border: none;
		border-radius: 6px;
		cursor: pointer;
		transition: opacity 0.15s;
	}

	button:hover {
		opacity: 0.85;
	}

	@keyframes shake {
		0%,
		100% {
			transform: translateX(0);
		}
		20% {
			transform: translateX(-8px);
		}
		40% {
			transform: translateX(8px);
		}
		60% {
			transform: translateX(-6px);
		}
		80% {
			transform: translateX(6px);
		}
	}

	.card.shake {
		animation: shake 0.45s ease;
	}
</style>
