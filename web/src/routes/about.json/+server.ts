export const prerender = true;

import type { RequestHandler } from '@sveltejs/kit';

export const GET: RequestHandler = async () => {
	const data = {
		builtOn: (import.meta.env.VITE_BUILD_TIME as string) ?? '',
		gitDescribe: (import.meta.env.VITE_GIT_DESCRIBE as string) ?? '',
		gitBranch: (import.meta.env.VITE_GIT_BRANCH as string) ?? '',
		dockerImage: (import.meta.env.VITE_DOCKER_IMAGE as string) ?? '',
		gitRepoSlug: (import.meta.env.VITE_GIT_REPO_SLUG as string) ?? '',
		gitRepoUrl: (import.meta.env.VITE_GIT_REPO_URL as string) ?? ''
	};

	return new Response(JSON.stringify(data, null, 2), {
		headers: { 'Content-Type': 'application/json' }
	});
};
