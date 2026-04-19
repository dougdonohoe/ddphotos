import { writable } from 'svelte/store';

// Set to true once albums are ready on encrypted sites, so the footer fades in with them.
// Starts false; the layout combines this with $page.data to show the footer immediately on
// unencrypted sites without waiting for this store.
export const footerReady = writable(false);
