import { defineConfig, devices } from '@playwright/test';

const baseURL = 'http://127.0.0.1:4173/ui/';

const desktop = {
	...devices['Desktop Chrome'],
	viewport: { width: 1280, height: 800 },
	locale: 'en-US',
	timezoneId: 'UTC',
	reducedMotion: 'reduce' as const
};

export default defineConfig({
	testDir: 'tests',
	fullyParallel: true,
	forbidOnly: Boolean(process.env.CI),
	reporter: 'list',
	use: {
		baseURL,
		trace: 'retain-on-failure'
	},
	webServer: {
		command: 'npm run build && npm run preview',
		url: baseURL,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000
	},
	projects: [
		{ name: 'screens', testDir: 'tests/screens', use: desktop },
		{ name: 'a11y', testDir: 'tests/a11y', use: desktop },
		{ name: 'e2e', testDir: 'tests/e2e', use: desktop },
		{ name: 'visual', testDir: 'tests/visual', use: desktop }
	]
});
