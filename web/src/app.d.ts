// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

interface ImportMetaEnv {
	readonly VITE_SITE_NAME: string;
	readonly VITE_SITE_URL: string;
	readonly VITE_SITE_DESCRIPTION: string;
	readonly VITE_COPYRIGHT_OWNER: string;
	readonly VITE_COPYRIGHT_YEAR: string;
	readonly VITE_BUILD_TIME: string;
	readonly VITE_ALLOW_CRAWLING: string;
}

interface ImportMeta {
	readonly env: ImportMetaEnv;
}

export {};
