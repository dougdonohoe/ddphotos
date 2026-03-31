<script lang="ts">
	interface Props {
		title: string;
		onunlock: (password: string) => void;
		shakeCount?: number;
	}

	let { title, onunlock, shakeCount = 0 }: Props = $props();
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
		<div class="lock-icon">
			<svg
				xmlns="http://www.w3.org/2000/svg"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.75"
				stroke-linecap="round"
				stroke-linejoin="round"
				width="40"
				height="40"
				aria-hidden="true"
			>
				<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
				<path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
			</svg>
		</div>
		<h2>{title}</h2>
		<form onsubmit={handleSubmit}>
			<!-- svelte-ignore a11y_autofocus — intentional: this is an explicit password dialog -->
			<input
				type="password"
				placeholder="Password"
				bind:value={password}
				autocomplete="current-password"
				autofocus
			/>
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
		max-width: 360px;
		text-align: center;
	}

	.lock-icon {
		color: var(--text-muted);
		margin-bottom: 1rem;
	}

	h2 {
		margin: 0 0 1.5rem;
		font-size: 1.2rem;
		color: var(--text-color);
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
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
