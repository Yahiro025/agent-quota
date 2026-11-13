import { startConnector } from "./client.js";
import { bindPage, performInDocument } from "./youtube-targets.js";
const config = globalThis.__BLOOM_MEDIA_CONFIG__;
const TAB_OPS = new Set(["mute", "unmute", "show"]),
	MAX_TABS = 8,
	TITLE_MAX = 512,
	SITE_MAX = 253;
function clip(value, limit) {
	const result = value.slice(0, limit),
		last = result.charCodeAt(result.length - 1);
	// Never leave half a surrogate pair on the wire.
	return last >= 0xd800 && last <= 0xdbff ? result.slice(0, -1) : result;
}
function siteOf(url) {
	try {
		return clip(new URL(url ?? "").hostname, SITE_MAX);
	} catch {
		return "";
	}
}
// A tab is only describable when it has a title or a URL; anything else cannot
// become a row and leaves the player unattributed.
function tabRow(tab) {
	const site = siteOf(tab.url),
		title = clip(typeof tab.title === "string" ? tab.title.trim() : "", TITLE_MAX) || site;
	return title
		? { id: tab.id, title, site, muted: !!tab.mutedInfo?.muted, window: tab.windowId }
		: null;
}
function isYouTube(url) {
	try {
		return new URL(url ?? "").origin === "https://www.youtube.com";
	} catch {
		return false;
	}
}
function tabRows(tabs, represented) {
	return tabs
		.filter((tab) => tab.audible && !represented.has(tab.id))
		.sort((a, b) => (a.windowId ?? 0) - (b.windowId ?? 0) || (a.index ?? 0) - (b.index ?? 0))
		.map(tabRow)
		.filter(Boolean)
		.slice(0, MAX_TABS);
}
async function performTab(chromeApi, request) {
	const match = /^tab:(\d+)$/.exec(request.session),
		id = match ? Number(match[1]) : NaN;
	if (!Number.isSafeInteger(id)) return { status: "stale" };
	let tab;
	try {
		tab = await chromeApi.tabs.get(id);
	} catch {
		return { status: "stale" };
	}
	if (!tab) return { status: "stale" };
	// A tab that now hosts a YouTube player belongs to the snapshot path.
	if (isYouTube(tab.url)) return { status: "stale" };
	if (request.op === "show") {
		await chromeApi.tabs.update(id, { active: true });
		await chromeApi.windows.update(tab.windowId, { focused: true });
	} else await chromeApi.tabs.update(id, { muted: request.op === "mute" });
	return { status: "confirmed" };
}
export function createWorker(chromeApi) {
	let targets = new Map();
	async function inventory() {
		const tabs = await chromeApi.tabs.query({}),
			audible = tabs.filter((t) => t.audible);
		const youtube = tabs.filter((t) => isYouTube(t.url));
		if (youtube.length > 16) {
			targets.clear();
			return { snapshots: [], tabs: tabRows(audible, new Set()), unknown_playing: true };
		}
		const next = new Map(),
			snapshots = [],
			represented = new Set();
		let unknown = audible.some((tab) => !tabRow(tab));
		await Promise.all(
			youtube.map(async (tab) => {
				try {
					const args = { target: { tabId: tab.id }, world: "MAIN" };
					let [result] = await chromeApi.scripting.executeScript({
						...args,
						func: () => globalThis.__bloomMediaPage?.read()
					});
					if (!result?.result) {
						await chromeApi.scripting.executeScript({
							...args,
							files: ["youtube-page.bundle.js"]
						});
						[result] = await chromeApi.scripting.executeScript({
							...args,
							func: () => globalThis.__bloomMediaPage?.read()
						});
					}
					const bound = bindPage(tab.id, result);
					if (!bound) {
						if (tab.audible) unknown = true;
						return;
					}
					represented.add(tab.id);
					next.set(bound.snapshot.session, bound.target);
					snapshots.push(bound.snapshot);
				} catch {
					if (tab.audible) unknown = true;
				}
			})
		);
		targets = next;
		return { snapshots, tabs: tabRows(audible, represented), unknown_playing: unknown };
	}
	async function perform(request) {
		if (TAB_OPS.has(request.op)) return performTab(chromeApi, request);
		// Chrome, not page content, supplies tab and document IDs. Full navigation
		// cannot redirect a queued action to the new page that inherited the tab ID.
		return performInDocument(
			chromeApi.scripting.executeScript.bind(chromeApi.scripting),
			targets.get(request.session),
			request
		);
	}
	return { inventory, perform };
}
// Tests import createWorker without a browser; the built worker always has both.
if (config && globalThis.chrome?.alarms) {
	// A suspended worker keeps no timers, so a connection attempt that failed while Bloom was
	// closed would never be retried. The alarm wakes the worker, and loading it reconnects.
	chrome.alarms.create("bloom-media-wake", { periodInMinutes: 0.5 });
	chrome.alarms.onAlarm.addListener(() => {});
	startConnector(config, createWorker(chrome));
}
