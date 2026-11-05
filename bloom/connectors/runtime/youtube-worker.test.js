import { expect, test } from "bun:test";
import { createWorker } from "./youtube-worker.js";

function workerFor(tabs, scripted = {}) {
	const chrome = {
		tabs: { query: async () => tabs },
		scripting: {
			executeScript: async ({ target }) =>
				scripted[target.tabId] ?? [{ documentId: "doc", result: null }]
		}
	};
	return createWorker(chrome);
}

function actionWorker(tab, calls) {
	return createWorker({
		tabs: {
			get: async () => {
				if (!tab) throw new Error("gone");
				return tab;
			},
			update: async (id, props) => calls.push(["tabs.update", id, props])
		},
		windows: { update: async (id, props) => calls.push(["windows.update", id, props]) },
		scripting: { executeScript: async () => [] }
	});
}

test("inventory lists only audible tabs that no snapshot represents", async () => {
	const tabs = [
		{
			id: 7,
			windowId: 1,
			index: 0,
			audible: true,
			title: "Video",
			url: "https://www.youtube.com/watch?v=v"
		},
		{
			id: 3,
			windowId: 1,
			index: 2,
			audible: true,
			title: "  Anime  ",
			url: "https://animesuge.example/watch/1",
			mutedInfo: { muted: true }
		},
		{ id: 4, windowId: 1, index: 3, audible: false, title: "Hidden", url: "https://secret.example/" },
		{ id: 5, windowId: 2, index: 0, audible: true }
	];
	const inventory = await workerFor(tabs, {
		7: [{ documentId: "doc", result: { session: "inner", title: "Video" } }]
	}).inventory();
	expect(inventory.snapshots.length).toBe(1);
	expect(inventory.tabs).toEqual([
		{ id: 3, title: "Anime", site: "animesuge.example", muted: true, window: 1 }
	]);
	expect(inventory.unknown_playing).toBe(true);
	const wire = JSON.stringify(inventory);
	expect(wire).not.toContain("secret.example");
	expect(wire).not.toContain("Hidden");
});

test("tab rows are ordered by window then index and capped at eight", async () => {
	const tabs = [
		{ id: 20, windowId: 2, index: 0, audible: true, title: "Later", url: "https://b.example/" },
		...Array.from({ length: 10 }, (_, i) => ({
			id: i,
			windowId: 1,
			index: i + 1,
			audible: true,
			title: `Tab ${i}`,
			url: "https://a.example/"
		}))
	];
	const inventory = await workerFor(tabs).inventory();
	expect(inventory.tabs.map((row) => row.id)).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
	expect(inventory.unknown_playing).toBe(false);
});

test("a title longer than the wire cap is clipped", async () => {
	const inventory = await workerFor([
		{ id: 1, windowId: 1, index: 0, audible: true, title: "x".repeat(600), url: "https://a.example/" }
	]).inventory();
	expect(inventory.tabs[0].title.length).toBe(512);
});

test("mute, unmute and show act on the captured tab only", async () => {
	const calls = [],
		worker = actionWorker({ id: 3, windowId: 4, url: "https://animesuge.example/" }, calls);
	expect(await worker.perform({ op: "mute", session: "tab:3" })).toEqual({ status: "confirmed" });
	expect(calls.at(-1)).toEqual(["tabs.update", 3, { muted: true }]);
	expect(await worker.perform({ op: "unmute", session: "tab:3" })).toEqual({ status: "confirmed" });
	expect(calls.at(-1)).toEqual(["tabs.update", 3, { muted: false }]);
	expect(await worker.perform({ op: "show", session: "tab:3" })).toEqual({ status: "confirmed" });
	expect(calls.slice(-2)).toEqual([
		["tabs.update", 3, { active: true }],
		["windows.update", 4, { focused: true }]
	]);
});

test("a vanished or YouTube tab is stale and receives no command", async () => {
	const calls = [];
	expect(await actionWorker(null, calls).perform({ op: "mute", session: "tab:9" })).toEqual({
		status: "stale"
	});
	expect(
		await actionWorker(
			{ id: 9, windowId: 1, url: "https://www.youtube.com/watch?v=v" },
			calls
		).perform({ op: "show", session: "tab:9" })
	).toEqual({ status: "stale" });
	expect(calls).toEqual([]);
});
