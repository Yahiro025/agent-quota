import { expect, jest, test } from "bun:test";
import { QueueController, type QueueView } from "./queueState";
const view = (generation = "a"): QueueView => ({
	generation,
	available: true,
	source: "Spotify",
	revision: "r",
	entries: [
		{ id: "first", title: "Same title", artist: "Artist" },
		{ id: "second", title: "Same title", artist: "Artist" }
	]
});
const deferred = <T>() => {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((r) => {
		resolve = r;
	});
	return { promise, resolve };
};
test("one exact occurrence is sent, acknowledgment is not confirmation, explicit recheck is required", async () => {
	const result = deferred<string>(),
		sent: any[] = [];
	const c = new QueueController((async (cmd: string, args: unknown) => {
		sent.push([cmd, args]);
		return cmd === "media_queue_get" ? view() : result.promise;
	}) as any);
	c.target("a");
	await c.refresh();
	const pending = c.select("second");
	await c.select("first");
	expect(sent.filter((c) => c[0] === "media_queue_select")).toEqual([
		["media_queue_select", { generation: "a", revision: "r", entryId: "second" }]
	]);
	result.resolve("dispatched");
	await pending;
	expect(c.state.needsCheck).toBe(true);
	await c.refresh();
	await c.select("first");
	expect(sent.filter((c) => c[0] === "media_queue_select")).toHaveLength(1);
	await c.refresh(true);
	expect(c.state.needsCheck).toBe(false);
});
test("late request and inventory replies cannot replace a newer source with identical text", async () => {
	const result = deferred<string>(),
		oldRead = deferred<QueueView>();
	let reads = 0;
	const c = new QueueController((async (cmd: string, args: any) =>
		cmd === "media_queue_get"
			? ++reads === 2
				? oldRead.promise
				: view(args.generation)
			: result.promise) as any);
	c.target("a");
	await c.refresh();
	const pending = c.select("second"),
		loading = c.refresh();
	c.target("b");
	await c.refresh();
	oldRead.resolve(view("a"));
	result.resolve("confirmed");
	await Promise.all([pending, loading]);
	expect(c.state.generation).toBe("b");
	expect(c.state.view?.generation).toBe("b");
	expect(c.state.message).toBe("");
	expect(c.state.pending).toBeNull();
});
test("stale entries refresh without replay and missing entries cannot dispatch", async () => {
	let selects = 0;
	const c = new QueueController((async (cmd: string) =>
		cmd === "media_queue_get" ? view() : (++selects, "stale")) as any);
	c.target("a");
	await c.refresh();
	await c.select("absent");
	await c.select("second");
	expect(selects).toBe(1);
	expect(c.state.message).toContain("Queue changed");
	expect(c.state.needsCheck).toBe(false);
});
test("a success note lasts its full time even when an earlier one was still showing", async () => {
	jest.useFakeTimers();
	try {
		const c = new QueueController((async (cmd: string) =>
			cmd === "media_queue_get" ? view() : "confirmed") as any);
		c.target("a");
		await c.refresh();
		await c.select("first");
		expect(c.state.message).toBe("Playing selected item.");
		jest.advanceTimersByTime(3000);
		await c.select("second");
		// The first note's timer is due now; it must not clear the second note.
		jest.advanceTimersByTime(1500);
		expect(c.state.message).toBe("Playing selected item.");
		jest.advanceTimersByTime(2500);
		expect(c.state.message).toBe("");
	} finally {
		jest.useRealTimers();
	}
});
