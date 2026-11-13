import { test, expect } from "bun:test";
import { bindPage, performInDocument } from "./youtube-targets.js";
test("equal titles in different tabs and new documents always have distinct outer identities", () => {
	const page = {
		documentId: "doc1",
		result: { session: "inner", title: "Same", tabId: 99, documentId: "forged" }
	};
	const a = bindPage(1, page),
		b = bindPage(2, page),
		c = bindPage(1, { ...page, documentId: "doc2" });
	expect(new Set([a.snapshot.session, b.snapshot.session, c.snapshot.session]).size).toBe(3);
	expect(a.target).toEqual({ tabId: 1, documentId: "doc1", inner: "inner" });
	expect(bindPage(1, { result: page.result })).toBeNull();
});
test("selection is pinned to captured document and rejects a navigation or replaced page player", async () => {
	const target = { tabId: 1, documentId: "doc1", inner: "old" },
		request = { session: "1/doc1/old", op: "select" };
	let call;
	const execute = async (args) => {
		call = args;
		return [
			{
				documentId: "doc1",
				result: { status: "confirmed", snapshot: { session: "old", title: "Track" } }
			}
		];
	};
	const value = await performInDocument(execute, target, request);
	expect(call.target).toEqual({ tabId: 1, documentIds: ["doc1"] });
	expect(value.snapshot.session).toBe(request.session);
	expect(
		await performInDocument(
			async () => [{ documentId: "doc2", result: { status: "confirmed" } }],
			target,
			request
		)
	).toEqual({ status: "stale" });
	expect(
		await performInDocument(
			async () => [
				{ documentId: "doc1", result: { status: "confirmed", snapshot: { session: "new" } } }
			],
			target,
			request
		)
	).toEqual({ status: "stale" });
	expect(await performInDocument(execute, null, request)).toEqual({ status: "stale" });
});
