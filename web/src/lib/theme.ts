import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'dark' | 'light';

const storedTheme = browser ? (localStorage.getItem('theme') as Theme) : null;
const defaultTheme: Theme = 'dark';

export const theme = writable<Theme>(storedTheme || defaultTheme);

theme.subscribe((value) => {
	if (browser) {
		localStorage.setItem('theme', value);
		document.documentElement.setAttribute('data-theme', value);
	}
});

export function toggleTheme() {
	theme.update((t) => (t === 'dark' ? 'light' : 'dark'));
}
