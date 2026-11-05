// The worker supplies these IDs from Chrome's InjectionResult, never page data.
export function bindPage(tabId, result) {
	if (
		!Number.isInteger(tabId) ||
		typeof result?.documentId !== "string" ||
		!result.documentId ||
		typeof result?.result?.session !== "string" ||
		!result.result.session
	)
		return null;
	const inner = result.result.session,
		session = `${tabId}/${result.documentId}/${inner}`;
	if (session.length > 256) return null;
	return {
		snapshot: { ...result.result, session },
		target: { tabId, documentId: result.documentId, inner }
	};
}
export async function performInDocument(execute, target, request) {
	if (!target) return { status: "stale" };
	const [result] = await execute({
		target: { tabId: target.tabId, documentIds: [target.documentId] },
		world: "MAIN",
		func: async (inner, request) => await globalThis.__bloomMediaPage?.perform(inner, request),
		args: [target.inner, request]
	});
	if (result?.documentId !== target.documentId || !result.result) return { status: "stale" };
	const value = result.result;
	if (value.snapshot) {
		if (value.snapshot.session !== target.inner) return { status: "stale" };
		value.snapshot = { ...value.snapshot, session: request.session };
	}
	return value;
}
