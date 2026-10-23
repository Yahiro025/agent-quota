import { defineConfig } from "@playwright/test";

export default defineConfig({
	testDir: "e2e",
	timeout: 30000,
	// One Vite instance and one notch: serial keeps state assertions simple.
	workers: 1,
	use: {
		baseURL: "http://localhost:1420",
		viewport: { width: 1280, height: 800 },
		screenshot: "only-on-failure"
	},
	webServer: {
		command: "bun run dev",
		url: "http://localhost:1420",
		reuseExistingServer: true,
		timeout: 30000
	},
	reporter: [["list"]]
});
