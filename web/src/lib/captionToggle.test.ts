import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// The module caches the choice on first read, so each test imports a fresh copy.
async function freshModule() {
	vi.resetModules();
	return import('$lib/captionToggle');
}

describe('caption toggle', () => {
	let store: Map<string, string>;

	beforeEach(() => {
		store = new Map();
		vi.stubGlobal('sessionStorage', {
			getItem: (k: string) => store.get(k) ?? null,
			setItem: (k: string, v: string) => void store.set(k, v),
			removeItem: (k: string) => void store.delete(k)
		});
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('defaults to captions shown', async () => {
		const { getCaptionsHidden } = await freshModule();
		expect(getCaptionsHidden()).toBe(false);
	});

	it('reads a hidden choice from the session', async () => {
		store.set('ddp_captions_hidden', '1');
		const { getCaptionsHidden } = await freshModule();
		expect(getCaptionsHidden()).toBe(true);
	});

	it('records hiding, and clears the key on showing again', async () => {
		const { getCaptionsHidden, setCaptionsHidden } = await freshModule();
		setCaptionsHidden(true);
		expect(getCaptionsHidden()).toBe(true);
		expect(store.get('ddp_captions_hidden')).toBe('1');
		setCaptionsHidden(false);
		expect(getCaptionsHidden()).toBe(false);
		expect(store.has('ddp_captions_hidden')).toBe(false);
	});

	it('keeps working when storage throws', async () => {
		const denied = () => {
			throw new Error('denied');
		};
		vi.stubGlobal('sessionStorage', { getItem: denied, setItem: denied, removeItem: denied });
		const { getCaptionsHidden, setCaptionsHidden } = await freshModule();
		expect(getCaptionsHidden()).toBe(false);
		setCaptionsHidden(true);
		expect(getCaptionsHidden()).toBe(true);
	});
});
