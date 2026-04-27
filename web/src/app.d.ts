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
	readonly VITE_BUILD_TIME: string;
	readonly VITE_DOCKER_IMAGE: string;
}

// Augments Vite's built-in ImportMeta interface to add type-safe access to
// import.meta.env variables. IntelliJ incorrectly reports this as unused because
// it doesn't recognize the Vite module augmentation pattern.
// noinspection JSUnusedLocalSymbols
interface ImportMeta {
	readonly env: ImportMetaEnv;
}

export {};
