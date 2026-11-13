import { test, expect, type Page } from "@playwright/test";
import path from "node:path";
const MOCK = path.join(import.meta.dirname, "tauri-mock.js");
const media = (generation = "spotify:a") => ({
	target_generation: generation,
	title: "Current item",
	artist: "Artist",
	is_playing: true,
	has_media: true,
	position_ms: 10000,
	duration_ms: 180000,
	seek_enabled: true,
	position_updated_at: Date.now()
});
async function emit(page: Page, generation = "spotify:a") {
	await page.evaluate((info) => (window as any).__emit("media-update", info), media(generation));
}
test.beforeEach(async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	await page.addInitScript(() => {
		localStorage.setItem("bloom-media-layout", "compact");
		const w = window as any,
			original = w.__TAURI_INTERNALS__.invoke;
		w.__queueState = "ready";
		w.__queueResult = "confirmed";
		w.__TAURI_INTERNALS__.invoke = (cmd: string, args: any) => {
			const fallback = original(cmd, args);
			if (cmd === "media_panel_bounds") return Promise.resolve(500);
			if (cmd === "media_queue_get")
				return Promise.resolve({
					generation: args.generation,
					available: w.__queueState !== "unavailable",
					source: "Spotify",
					revision: "rev1",
					entries:
						w.__queueState === "empty" || w.__queueState === "unavailable"
							? []
							: Array.from({ length: w.__queueState === "few" ? 3 : 12 }, (_, i) => ({
									id: `occurrence-${i}`,
									title: i === 2 ? "Long title ".repeat(30) : "Same title",
									artist: i === 1 ? "" : "Artist",
									duration: i % 2 ? null : "3:20"
								}))
				});
			if (cmd === "media_queue_select")
				return new Promise((resolve) => {
					w.__resolveSelection = () => resolve(w.__queueResult);
				});
			return fallback;
		};
	});
	await page.goto("/");
	await expect(page.locator(".bloom")).toBeVisible();
	await page.waitForTimeout(1000);
	await emit(page);
	// The resting pill shows the player's art (or its placeholder) as the
	// button that opens the music view.
	await page.getByRole("button", { name: "Open music" }).click();
	await expect(page.locator(".cmp")).toBeVisible();
});
test("drawer preserves transport, selects one occurrence, and isolates scrolling", async ({
	page
}, testInfo) => {
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	const drawer = page.getByRole("region", { name: "Up next" });
	await expect(drawer).toBeVisible();
	await expect(drawer.getByRole("button", { name: /Same title/ })).toHaveCount(11);
	const play = page.getByRole("button", { name: "Pause", exact: true });
	await expect(play).toBeVisible();
	const center = await play.boundingBox(),
		bounds = await page.locator(".cmp").boundingBox();
	expect(Math.abs(center!.x + center!.width / 2 - (bounds!.x + bounds!.width / 2))).toBeLessThan(2);
	await page.locator(".bloom").screenshot({ path: testInfo.outputPath("queue-drawer.png") });
	await drawer.locator(".mq-row").nth(1).click();
	await drawer.locator(".mq-row").nth(3).click({ force: true });
	const calls = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "media_queue_select")
	);
	expect(calls).toHaveLength(1);
	expect(calls[0].args).toEqual({
		generation: "spotify:a",
		revision: "rev1",
		entryId: "occurrence-1"
	});
	await page.evaluate(() => (window as any).__resolveSelection());
	await expect(drawer).toContainText("Playing selected item");
	await drawer.hover();
	await page.mouse.wheel(0, 400);
	await expect(drawer).toBeVisible();
	await expect(page.locator(".cmp")).toBeVisible();
});
test("volume and queue are exclusive, layout switches close the drawer", async ({ page }) => {
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await page.getByRole("button", { name: "Volume", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toHaveCount(0);
	await expect(page.locator(".cmp-volume-slider")).toBeVisible();
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.locator(".cmp-volume-slider")).toHaveCount(0);
	await page.locator(".cmp-art").click();
	await expect(page.locator(".compact-premium-layout")).toBeVisible();
	await page.locator(".premium-album-art").click();
	await expect(page.locator(".cmp")).toBeVisible();
	await expect(page.getByRole("region", { name: "Up next" })).toHaveCount(0);
});
test("uncertain selection requires a check; source changes discard late results", async ({
	page
}) => {
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await page.evaluate(() => {
		(window as any).__queueResult = "uncertain";
	});
	await page.locator(".mq-row").nth(1).click();
	await page.evaluate(() => (window as any).__resolveSelection());
	await expect(page.getByRole("region", { name: "Up next" })).toContainText(
		"Playback is uncertain"
	);
	await expect(page.locator(".mq-row").first()).toBeDisabled();
	await page.getByRole("button", { name: "Check queue" }).click();
	await expect(page.locator(".mq-row").first()).toBeEnabled();
	await page.locator(".mq-row").nth(1).click();
	await emit(page, "youtube:b");
	await page.evaluate(() => {
		(window as any).__queueResult = "confirmed";
		(window as any).__resolveSelection();
	});
	await expect(page.getByRole("region", { name: "Up next" })).not.toContainText(
		"Playing selected item"
	);
	await expect(page.locator(".mq-row").first()).toBeEnabled();
});
test("empty and unsupported queues differ, reduced work area scrolls inside native region", async ({
	page
}) => {
	await page.evaluate(() => {
		(window as any).__queueState = "empty";
	});
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toContainText("Nothing up next");
	await page.evaluate(() => {
		(window as any).__queueState = "unavailable";
		(window as any).__emit("media-queue-update");
	});
	await expect(page.getByRole("region", { name: "Up next" })).toContainText("Queue unavailable");
	await page.evaluate(() => {
		const w = window as any,
			original = w.__TAURI_INTERNALS__.invoke;
		w.__TAURI_INTERNALS__.invoke = (cmd: string, args: unknown) =>
			cmd === "media_panel_bounds" ? Promise.resolve(300) : original(cmd, args);
		w.__queueState = "ready";
		window.dispatchEvent(new Event("resize"));
		w.__emit("media-queue-update");
	});
	await expect(page.locator(".mq-row")).toHaveCount(12);
	await expect
		.poll(async () => (await page.locator(".bloom").boundingBox())!.height)
		.toBeLessThanOrEqual(300);
	const scroll = await page
		.locator(".mq-list")
		.evaluate((e) => ({ height: e.clientHeight, content: e.scrollHeight }));
	expect(scroll.height).toBeLessThan(172);
	expect(scroll.content).toBeGreaterThan(scroll.height);
	const regions = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "set_window_region")
	);
	expect(regions.length).toBeGreaterThan(0);
});
test("a shrinking queue never slides the island out from under the pointer", async ({ page }) => {
	await page.evaluate(() => {
		(window as any).__queueState = "few";
	});
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.locator(".mq-row")).toHaveCount(3);
	// Let the island finish growing so the last row is inside it.
	await page.waitForTimeout(600);
	const last = (await page.locator(".mq-row").nth(2).boundingBox())!;
	await page.mouse.move(last.x + last.width / 2, last.y + last.height / 2);
	await page.waitForTimeout(400);
	// The last item was chosen: nothing is left upcoming.
	await page.evaluate(() => {
		(window as any).__queueState = "empty";
		(window as any).__emit("media-queue-update");
	});
	await expect(page.getByRole("region", { name: "Up next" })).toContainText("Nothing up next");
	await page.waitForTimeout(600);
	const island = () => page.locator(".bloom").boundingBox();
	expect((await island())!.y + (await island())!.height).toBeGreaterThan(last.y + last.height / 2);
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
	// Once the pointer is back over the content, the island fits it.
	const header = (await page.locator(".mq-header").boundingBox())!;
	await page.mouse.move(header.x + 40, header.y + header.height / 2, { steps: 4 });
	await page.waitForTimeout(600);
	expect((await island())!.y + (await island())!.height).toBeLessThan(last.y);
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
});
test("a seek drag cannot target the source that replaced the displayed player", async ({
	page
}) => {
	const seek = page.locator(".cmp-seek"),
		bounds = await seek.boundingBox();
	await page.mouse.move(bounds!.x + bounds!.width / 2, bounds!.y + bounds!.height / 2);
	await page.mouse.down();
	await emit(page, "youtube:b");
	await page.mouse.up();
	const seeks = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "media_action" && x.args.op === "seek")
	);
	expect(seeks).toHaveLength(0);
});
test.describe("reduced motion", () => {
	test.use({ contextOptions: { reducedMotion: "reduce" } });
	test("queue height is final on the next rendered frame", async ({ page }) => {
		expect(
			await page.evaluate(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches)
		).toBe(true);
		const island = () => document.querySelector(".bloom")!.getBoundingClientRect().height;
		const closed = await page.evaluate(island);
		await page.getByRole("button", { name: "Up next", exact: true }).click();
		const height = await page.evaluate(
			() =>
				new Promise<number>((resolve) =>
					requestAnimationFrame(() =>
						requestAnimationFrame(() =>
							resolve(document.querySelector(".bloom")!.getBoundingClientRect().height)
						)
					)
				)
		);
		await page.waitForTimeout(800);
		expect(height).toBeGreaterThan(closed + 150);
		expect(height).toBeCloseTo(await page.evaluate(island), 0);
	});
});
test("the island fits the drawer's content instead of a fixed slot", async ({ page }) => {
	const height = () => page.evaluate(() => document.querySelector(".bloom")!.getBoundingClientRect().height);
	const closed = await height();
	await page.evaluate(() => {
		(window as any).__queueState = "empty";
	});
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toContainText("Nothing up next");
	await page.waitForTimeout(800);
	const empty = await height();
	expect(empty).toBeGreaterThan(closed + 40);
	expect(empty).toBeLessThan(closed + 100);
	// A short list fits exactly: no scrollbar, nothing cut off.
	await page.evaluate(() => {
		(window as any).__queueState = "few";
		(window as any).__emit("media-queue-update");
	});
	await expect(page.locator(".mq-row")).toHaveCount(3);
	await page.waitForTimeout(800);
	const few = await page
		.locator(".mq-list")
		.evaluate((e) => ({ height: e.clientHeight, content: e.scrollHeight }));
	expect(few.content).toBeLessThanOrEqual(few.height);
	await page.evaluate(() => {
		(window as any).__queueState = "ready";
		(window as any).__emit("media-queue-update");
	});
	await expect(page.locator(".mq-row")).toHaveCount(12);
	await page.waitForTimeout(800);
	expect(await height()).toBeGreaterThan(empty + 120);
	// Every row the island shows is inside it: the list scrolls, the drawer never overflows.
	const overflow = await page.evaluate(() => {
		const mq = document.querySelector(".mq")!;
		return mq.scrollHeight - mq.clientHeight;
	});
	expect(overflow).toBeLessThanOrEqual(1);
});

const card = (generation: string, source = "Spotify", extra: Record<string, unknown> = {}) => ({
	generation,
	source,
	app: source,
	title: `Track ${generation}`,
	artist: "Artist",
	is_playing: true,
	artwork: null,
	position_ms: 10000,
	duration_ms: 180000,
	seek_enabled: true,
	position_updated_at: Date.now(),
	queue: true,
	primary: false,
	...extra
});
async function emitList(page: Page, cards: unknown[], other_audio = false) {
	await page.evaluate((payload) => (window as any).__emit("media-list", payload), {
		cards,
		other_audio
	});
}
const TAB_TITLE = "Lovely Complex Episode 24 – Watch Anime Online in HD | AnimeSuge";
const tabCard = (generation: string, extra: Record<string, unknown> = {}) => ({
	generation,
	source: "Tab",
	app: "Brave",
	title: TAB_TITLE,
	artist: "anisuge.tv",
	kind: "tab",
	muted: false,
	is_playing: true,
	artwork: null,
	position_ms: 0,
	duration_ms: 0,
	seek_enabled: false,
	position_updated_at: Date.now(),
	queue: false,
	primary: false,
	...extra
});
const focusTitle = (page: Page) => page.locator(".msf-focus .cmp-title");
const lastQueue = (page: Page) =>
	page.evaluate(
		() =>
			(window as any).__invokes
				.filter((x: any) => x.cmd === "media_queue_get")
				.at(-1)?.args.generation
	);

test("two playing players focus the primary and shelf the other", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube")
	]);
	await expect(page.locator(".cmp")).toHaveCount(0);
	await expect(page.locator(".msf-focus")).toHaveAttribute("aria-label", "Spotify: Track spotify:a");
	await expect(focusTitle(page)).toHaveText("Track spotify:a");
	await expect(page.getByRole("button", { name: "Show Track youtube:b" })).toBeVisible();

	// Promotion: the shelf row becomes the focus block, and its queue follows it.
	await page.getByRole("button", { name: "Show Track youtube:b" }).click();
	await expect(page.locator(".msf-focus")).toHaveAttribute("aria-label", "YouTube: Track youtube:b");
	await expect(focusTitle(page)).toHaveText("Track youtube:b");
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
	expect(await lastQueue(page)).toBe("youtube:b");
});

test("a shelf play/pause acts on its own player and leaves focus alone", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube")
	]);
	await page.getByRole("button", { name: "Pause Track youtube:b" }).click();
	const actions = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "media_action")
	);
	expect(actions.at(-1).args).toEqual({ generation: "youtube:b", op: "playPause" });
	await expect(focusTitle(page)).toHaveText("Track spotify:a");
});

test("three players keep one focus and two shelf rows with their own queues", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube"),
		card("brave:c", "Windows", { app: "Brave" })
	]);
	await expect(page.locator(".msf-row")).toHaveCount(2);
	await expect(focusTitle(page)).toHaveText("Track spotify:a");
	await page.getByRole("button", { name: "Show Track brave:c" }).click();
	await expect(page.locator(".msf-focus")).toHaveAttribute("aria-label", "Brave: Track brave:c");
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
	expect(await lastQueue(page)).toBe("brave:c");
	// A Windows player is named by its app, never by the platform.
	await expect(page.locator(".msf-app-chip")).toHaveText("Brave");
	await expect(page.locator(".msf-app-chip")).not.toHaveText("Windows");
});

test("a long title never runs into the app chip", async ({ page }) => {
	const long =
		"Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge Extended Director's Cut";
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true, title: long }),
		card("brave:c", "Windows", { app: "Brave", title: "Other" })
	]);
	const title = (await page.locator(".msf-focus-title").boundingBox())!;
	const chip = (await page.locator(".msf-app-chip").boundingBox())!;
	expect(title.x + title.width).toBeLessThanOrEqual(chip.x + 0.5);
	await expect(page.locator(".msf-focus-title")).toHaveAttribute("title", long);
});

test("the shelf separates title and app, and only the title ellipsizes", async ({ page }) => {
	const long =
		"Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge Extended Director's Cut";
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("brave:c", "Windows", { app: "Brave", title: long })
	]);
	const sep = page.locator(".msf-row-sep").first();
	await expect(sep).toHaveText("·");
	expect(
		await sep.evaluate((e) => {
			const s = getComputedStyle(e);
			return [s.marginLeft, s.marginRight];
		})
	).toEqual(["6px", "6px"]);
	const app = page.locator(".msf-row-app").first();
	await expect(app).toHaveText("Brave");
	const appBox = await app.evaluate((e) => ({ scroll: e.scrollWidth, client: e.clientWidth }));
	expect(appBox.scroll).toBeLessThanOrEqual(appBox.client);
	const title = page.locator(".msf-row-title").first();
	const titleBox = await title.evaluate((e) => ({
		scroll: e.scrollWidth,
		client: e.clientWidth
	}));
	expect(titleBox.scroll).toBeGreaterThan(titleBox.client);
	const titleRect = (await title.boundingBox())!,
		sepRect = (await sep.boundingBox())!;
	expect(titleRect.x + titleRect.width).toBeLessThanOrEqual(sepRect.x + 0.5);
});

test("Up next collapses the shelf into one also-playing strip", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube"),
		card("brave:c", "Windows", { app: "Brave" })
	]);
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
	await page.waitForTimeout(700);
	expect(
		await page.locator(".msf-below").evaluate((e) => e.getBoundingClientRect().height)
	).toBeLessThanOrEqual(0.5);
	const strip = page.getByRole("button", { name: "Close Up next and show 2 more playing" });
	await expect(strip).toBeVisible();
	await expect(strip.locator(".msf-also-art")).toHaveCount(2);
	await expect(strip).toContainText("2 more playing");
	await strip.click();
	await expect(page.getByRole("region", { name: "Up next" })).toHaveCount(0);
	await expect(page.locator(".msf-row")).toHaveCount(2);
	await expect(page.getByRole("button", { name: /Close Up next/ })).toHaveCount(0);
	await page.waitForTimeout(600);
	expect(
		await page.locator(".msf-below").evaluate((e) => e.getBoundingClientRect().height)
	).toBeGreaterThan(40);
});

test("queue content keeps its distance from the island's rounded bottom", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube"),
		card("brave:c", "Windows", { app: "Brave" })
	]);
	await page.getByRole("button", { name: "Up next", exact: true }).click();
	await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
	await page.waitForTimeout(800);
	const gap = await page.evaluate(() => {
		const island = document.querySelector(".bloom")!.getBoundingClientRect();
		let min = Infinity;
		for (const el of document.querySelectorAll(".msf *")) {
			const r = el.getBoundingClientRect();
			if (r.width <= 0 || r.height <= 0) continue;
			// Rows scrolled out of the queue list are clipped by the list, not
			// by the island, so they do not count as visible content.
			let hidden = false;
			for (let p = el.parentElement; p; p = p.parentElement) {
				const s = getComputedStyle(p);
				if (/(auto|scroll|hidden)/.test(s.overflowY) && r.bottom > p.getBoundingClientRect().bottom + 0.5) {
					hidden = true;
					break;
				}
			}
			if (!hidden) min = Math.min(min, island.bottom - r.bottom);
		}
		return min;
	});
	expect(gap).toBeGreaterThanOrEqual(9.5);
});

test("a paused player leaves the stack and the panel falls back to the single card", async ({
	page
}) => {
	await emitList(page, [card("spotify:a"), card("youtube:b", "YouTube")]);
	await expect(page.locator(".msf-focus")).toHaveCount(1);
	await expect(page.locator(".msf-row")).toHaveCount(1);
	await emitList(page, [card("spotify:a")]);
	await expect(page.locator(".msf")).toHaveCount(0);
	await expect(page.locator(".cmp")).toBeVisible();
});
test("a short shelf title stays one phrase with its app", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube")
	]);
	const row = page.locator(".msf-row").first();
	const title = row.locator(".msf-row-title"),
		sep = row.locator(".msf-row-sep");
	await expect(title).toHaveText("Track youtube:b");
	await expect(sep).toHaveText("·");
	const titleRect = (await title.boundingBox())!,
		sepRect = (await sep.boundingBox())!;
	// The title must not grow: the separator follows the text, not the row edge.
	const gap = sepRect.x - (titleRect.x + titleRect.width);
	expect(gap).toBeGreaterThanOrEqual(0);
	expect(gap).toBeLessThanOrEqual(12);
});

test("the shelf play glyph is a 16px mark in a 30px target", async ({ page }) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube")
	]);
	const play = page.getByRole("button", { name: "Pause Track youtube:b" });
	const glyph = (await play.locator("svg").boundingBox())!;
	expect(Math.abs(glyph.width - 16)).toBeLessThanOrEqual(0.5);
	expect(Math.abs(glyph.height - 16)).toBeLessThanOrEqual(0.5);
	const target = await play.evaluate((e) => {
		const s = getComputedStyle(e);
		return [parseFloat(s.width), parseFloat(s.height)];
	});
	expect(target[0]).toBeGreaterThanOrEqual(30);
	expect(target[1]).toBeGreaterThanOrEqual(30);
	await expect(play).toHaveCSS("opacity", "0.85");
});

test("a tab row mutes without a clock, and its focus offers Mute and Show tab", async ({
	page
}) => {
	await emitList(page, [
		card("spotify:a", "Spotify", { primary: true }),
		card("youtube:b", "YouTube"),
		tabCard("youtube:tab:7")
	]);
	const row = page.locator(".msf-row").filter({ hasText: TAB_TITLE });
	await expect(row.locator(".msf-row-app")).toHaveText("Brave");
	await expect(row.locator(".msf-row-progress")).toHaveCount(0);
	await page.getByRole("button", { name: `Mute ${TAB_TITLE}` }).click();
	// Optimistic: the button flips before any new list arrives.
	await expect(page.getByRole("button", { name: `Unmute ${TAB_TITLE}` })).toBeVisible();
	let actions = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "media_action")
	);
	expect(actions.at(-1).args).toEqual({ generation: "youtube:tab:7", op: "mute" });
	// Muting a row must not promote it.
	await expect(focusTitle(page)).toHaveText("Track spotify:a");

	await page.getByRole("button", { name: `Show ${TAB_TITLE}` }).click();
	await expect(page.locator(".msf-focus")).toHaveAttribute("aria-label", `Brave: ${TAB_TITLE}`);
	await expect(page.locator(".msf-focus-artist")).toHaveText("anisuge.tv");
	await expect(page.locator(".msf-app-chip")).toHaveText("Brave");
	await expect(page.locator(".msf-scrub")).toHaveCount(0);
	await expect(page.locator(".msf-transport")).toHaveCount(0);
	await expect(page.getByRole("button", { name: "Up next", exact: true })).toHaveCount(0);
	await page.getByRole("button", { name: "Show tab" }).click();
	actions = await page.evaluate(() =>
		(window as any).__invokes.filter((x: any) => x.cmd === "media_action")
	);
	expect(actions.at(-1).args).toEqual({ generation: "youtube:tab:7", op: "show" });

	// A rejected action returns the row to the last list's truth.
	await page.evaluate(() => {
		const w = window as any,
			original = w.__TAURI_INTERNALS__.invoke;
		w.__TAURI_INTERNALS__.invoke = (cmd: string, args: any) =>
			cmd === "media_action" && args.op === "unmute"
				? Promise.reject(new Error("rejected"))
				: original(cmd, args);
	});
	await page.getByRole("button", { name: "Unmute", exact: true }).click();
	await expect(page.getByRole("button", { name: "Mute", exact: true })).toBeVisible();
});

test("other audio is a row under the stack only", async ({ page }) => {
	await emitList(page, [card("spotify:a"), card("youtube:b", "YouTube")], true);
	await expect(page.getByText("Other audio playing")).toBeVisible();
	await emitList(page, [card("spotify:a")], true);
	await expect(page.locator(".cmp")).toBeVisible();
	await expect(page.getByText("Other audio playing")).toHaveCount(0);
});

// Design review captures. Device scale 2 so the 1px hairlines and the 3px
// scrubber are legible when the PNGs are viewed at their natural size.
const SHOTS = path.resolve(import.meta.dirname, "../../docs/verification/media-stack");
const art = (from: string, to: string) =>
	`data:image/svg+xml;utf8,${encodeURIComponent(
		`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${from}"/><stop offset="1" stop-color="${to}"/></linearGradient></defs><rect width="96" height="96" fill="url(#g)"/></svg>`
	)}`;
const BLUE = art("#1246ff", "#79d4ff");
const YELLOW = art("#ffb300", "#ffe680");

test.describe("design screenshots", () => {
	test.use({ deviceScaleFactor: 2 });
	test("focus, shelf, queue and long-title states", async ({ page }) => {
		test.setTimeout(60_000);
		const shot = async (name: string) => {
			await page.waitForTimeout(500);
			await page.locator(".bloom").screenshot({ path: path.join(SHOTS, name) });
		};
		const spotify = card("spotify:a", "Spotify", { primary: true, artwork: [BLUE] });
		const brave = card("brave:c", "Windows", {
			app: "Brave",
			title: "Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge",
			artwork: [YELLOW]
		});
		const youtube = card("youtube:b", "YouTube");

		await emitList(page, [spotify, brave]);
		await expect(page.getByRole("button", { name: "Show Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge" })).toBeVisible();
		await shot("two-players.png");

		// Two players with the drawer open: one row collapses into "1 more playing".
		await page.getByRole("button", { name: "Up next", exact: true }).click();
		await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
		await shot("queue-open.png");
		await page.getByRole("button", { name: "Up next", exact: true }).click();
		await expect(page.getByRole("region", { name: "Up next" })).toHaveCount(0);

		await emitList(page, [spotify, brave, youtube]);
		await expect(page.locator(".msf-row")).toHaveCount(2);
		await shot("three-players.png");

		// Three players: two avatars and "2 more playing" replace the shelf.
		await page.getByRole("button", { name: "Up next", exact: true }).click();
		await expect(page.getByRole("region", { name: "Up next" })).toBeVisible();
		await shot("queue-open-strip.png");

		await page.getByRole("button", { name: "Up next", exact: true }).click();
		await expect(page.getByRole("region", { name: "Up next" })).toHaveCount(0);
		await page
			.getByRole("button", { name: "Show Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge" })
			.hover();
		await shot("shelf-hover.png");

		const long = "Lovely Complex Episode 24 – Watch Online in HD | AnimeSuge Extended Director's Cut";
		await emitList(page, [
			card("spotify:a", "Spotify", { primary: true, artwork: [BLUE], title: long }),
			brave
		]);
		await expect(focusTitle(page)).toHaveText(long);
		await shot("long-title.png");
		// At rest, close up: the fade, not a mid-letter clip, ends the title.
		await page.waitForTimeout(500);
		await page
			.locator(".msf-focus")
			.screenshot({ path: path.join(SHOTS, "long-title-rest.png") });
	});

	test("tab row, tab focus and a muted tab row", async ({ page }) => {
		test.setTimeout(60_000);
		const shot = async (name: string) => {
			await page.waitForTimeout(500);
			await page.locator(".bloom").screenshot({ path: path.join(SHOTS, name) });
		};
		const spotify = card("spotify:a", "Spotify", { primary: true, artwork: [BLUE] });
		const tab = tabCard("youtube:tab:12");
		await emitList(page, [spotify, card("youtube:b", "YouTube"), tab]);
		await expect(page.getByRole("button", { name: `Show ${TAB_TITLE}` })).toBeVisible();
		await shot("tab-row.png");

		await page.getByRole("button", { name: `Show ${TAB_TITLE}` }).click();
		await expect(page.locator(".msf-tab-actions")).toBeVisible();
		await shot("tab-focus.png");

		// Mute from the focus, then hand focus back so the shelf shows the state.
		await page.getByRole("button", { name: "Mute", exact: true }).click();
		await page.getByRole("button", { name: "Show Track spotify:a" }).click();
		await expect(page.getByRole("button", { name: `Unmute ${TAB_TITLE}` })).toBeVisible();
		await shot("tab-muted.png");
	});
});
