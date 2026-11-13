import { test, expect } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve, sep } from "node:path";
import { buildConnectors } from "./build.js";
test("bundles contain only each source's key and page-world code contains neither key", async () => {
	const directory = await mkdtemp(join(tmpdir(), "bloom-connector-build-test-"));
	const settings = {
		port: 12345,
		spotify: "public-test-spotify".padEnd(32, "s"),
		youtube: "public-test-youtube".padEnd(32, "y")
	};
	try {
		await buildConnectors(settings, directory);
		const spotify = await Bun.file(join(directory, "bloom-media.js")).text(),
			worker = await Bun.file(join(directory, "youtube/worker.js")).text(),
			page = await Bun.file(join(directory, "youtube/youtube-page.bundle.js")).text();
		expect(spotify).toContain(settings.spotify);
		expect(spotify).not.toContain(settings.youtube);
		expect(worker).toContain(settings.youtube);
		expect(worker).not.toContain(settings.spotify);
		expect(page).not.toContain(settings.spotify);
		expect(page).not.toContain(settings.youtube);
		const manifest = await Bun.file(join(directory, "youtube/manifest.json")).json();
		// "alarms" retries a connection made while Bloom was closed; "tabs" reads the
		// audible tabs whose titles become rows.
		expect(manifest.permissions).toEqual(["scripting", "alarms", "tabs"]);
		expect(manifest.version).toBe("0.3.0");
		expect(worker).toContain("alarms.onAlarm.addListener");
		expect(worker).toContain("alarms.create");
		expect(manifest.host_permissions).toEqual(["https://www.youtube.com/*"]);
	} finally {
		if (!resolve(directory).startsWith(resolve(tmpdir()) + sep + "bloom-connector-build-test-"))
			throw Error("Unsafe cleanup");
		await rm(directory, { recursive: true });
	}
});
