import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'dark' | 'light';

const storedTheme = browser ? (localStorage.getItem('ddp_theme') as Theme) : null;
// Read the theme already applied by the inline script in app.html (sourced from the
// ddp-default-theme meta tag). This ensures the store matches what's on screen,
// preventing a flash when the configured default is not 'dark'.
const appliedTheme: Theme = browser
	? ((document.documentElement.getAttribute('data-theme') as Theme) ?? 'dark')
	: 'dark';

export const theme = writable<Theme>(storedTheme || appliedTheme);

theme.subscribe((value) => {
	if (browser) {
		localStorage.setItem('ddp_theme', value);
		document.documentElement.setAttribute('data-theme', value);
	}
});

export function toggleTheme() {
	theme.update((t) => (t === 'dark' ? 'light' : 'dark'));
}
