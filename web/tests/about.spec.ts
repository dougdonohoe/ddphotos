import { test, expect } from '@playwright/test';
import { waitForHydration } from './helpers';

// The About dialog shows the build time in the viewer's timezone, not the build
// machine's. Two zones, neither of them UTC (what a Docker build runs in), prove
// the time follows the browser. Tokyo has no DST, so its label is stable.
const zones = [
	{ timezoneId: 'America/Denver', label: /M[SD]T$/ },
	{ timezoneId: 'Asia/Tokyo', label: /GMT\+9$/ }
];

for (const { timezoneId, label } of zones) {
	test.describe(`About dialog in ${timezoneId}`, () => {
		test.use({ timezoneId });

		test('shows the build time in the viewer timezone', async ({ page }) => {
			const about = await page.request.get('/about.json').then((r) => r.json());
			// Hour and minute only: browsers differ on the space before AM/PM.
			const expected = new Date(about.builtOn).toLocaleTimeString('en-US', {
				timeZone: timezoneId,
				hour: 'numeric',
				minute: '2-digit',
				hour12: false
			});
			const hour12 = ((Number(expected.split(':')[0]) + 11) % 12) + 1;
			const minute = expected.split(':')[1];

			// The privacy page is never encrypted, so the footer is always ready.
			await page.goto('/privacy');
			await waitForHydration(page);
			await page.getByRole('button', { name: 'About this site' }).click();

			const built = page.locator('.modal-body dd').first();
			await expect(built).toContainText(`at ${hour12}:${minute}`);
			await expect(built).toHaveText(label);
		});
	});
}
