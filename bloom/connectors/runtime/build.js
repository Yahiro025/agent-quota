import { mkdir } from "node:fs/promises";
import { resolve, join } from "node:path";
export async function buildConnectors(settings, out = resolve(import.meta.dir, "generated")) {
	if (
		!Number.isInteger(settings.port) ||
		settings.port < 1 ||
		settings.port > 65535 ||
		typeof settings.spotify !== "string" ||
		settings.spotify.length !== 32 ||
		typeof settings.youtube !== "string" ||
		settings.youtube.length !== 32 ||
		settings.spotify === settings.youtube
	)
		throw new Error("Invalid local connector configuration");
	const browser = join(out, "youtube");
	await mkdir(browser, { recursive: true });
	for (const [entry, destination, config] of [
		[
			"spotify.js",
			join(out, "bloom-media.js"),
			{ port: settings.port, key: settings.spotify, source: "spotify" }
		],
		[
			"youtube-worker.js",
			join(browser, "worker.js"),
			{ port: settings.port, key: settings.youtube, source: "youtube" }
		],
		["youtube-page.js", join(browser, "youtube-page.bundle.js"), null]
	]) {
		const build = await Bun.build({
			entrypoints: [join(import.meta.dir, entry)],
			target: "browser",
			format: "iife",
			define: config ? { "globalThis.__BLOOM_MEDIA_CONFIG__": JSON.stringify(config) } : {}
		});
		if (!build.success) throw new Error("Connector build failed");
		await Bun.write(destination, await build.outputs[0].text());
	}
	await Bun.write(
		join(browser, "manifest.json"),
		JSON.stringify(
			{
				manifest_version: 3,
				name: "Bloom Media Connector",
				version: "0.3.0",
				description:
					"Connect YouTube playlists and queues in Brave to Bloom's media panel, and list audible tabs' titles to the local Bloom app only. No cloud account or external server.",
				permissions: ["scripting", "alarms", "tabs"],
				host_permissions: ["https://www.youtube.com/*"],
				background: { service_worker: "worker.js" },
				action: { default_title: "Bloom Media Connector" }
			},
			null,
			2
		)
	);
}
if (import.meta.main) {
	const file = process.argv[2];
	if (!file)
		throw new Error(
			"Supply Bloom's local media-connectors.json path; its secrets are never logged."
		);
	await buildConnectors(await Bun.file(file).json());
	console.log(
		"Built local source-scoped connectors. Generated credentials must not be committed or shared."
	);
}
