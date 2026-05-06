export async function load({ parent }) {
	const { siteConfig } = await parent();
	return { siteConfig };
}
