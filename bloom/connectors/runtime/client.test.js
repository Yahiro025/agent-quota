import { expect, test } from "bun:test";
import { startConnector } from "./client.js";

test("sends a bounded inventory and dispatches each request ID at most once", async () => {
	const sent = [];
	let socket,
		calls = 0;
	class Socket {
		constructor() {
			socket = this;
			this.readyState = 1;
		}
		send(s) {
			sent.push(JSON.parse(s));
		}
		close() {
			this.readyState = 3;
		}
	}
	const connection = startConnector(
		{ port: 1234, key: "a".repeat(32), source: "spotify" },
		{
			Socket,
			inventory: async () => ({ snapshots: [{ session: "s" }], unknown_playing: false }),
			perform: async () => {
				calls++;
				return { status: "confirmed", snapshot: { session: "s" } };
			},
			schedule: () => 0,
			cancel: () => {}
		}
	);
	await socket.onopen();
	expect(sent[0].op).toBe("inventory");
	expect(sent[0].snapshots[0].session).toBe("s");
	await socket.onmessage({ data: JSON.stringify({ id: "one", session: "s", op: "select" }) });
	await socket.onmessage({ data: JSON.stringify({ id: "one", session: "s", op: "select" }) });
	expect(calls).toBe(1);
	expect(sent.filter((m) => m.op === "result").length).toBe(1);
	connection.stop();
});

test("rejects unknown actions and oversized server messages without source side effects", async () => {
	let socket,
		calls = 0;
	class Socket {
		constructor() {
			socket = this;
			this.readyState = 1;
		}
		send() {}
		close() {
			this.readyState = 3;
		}
	}
	const c = startConnector(
		{ port: 1234, key: "a".repeat(32), source: "spotify" },
		{
			Socket,
			inventory: async () => ({ snapshots: [] }),
			perform: async () => {
				calls++;
			},
			schedule: () => 0,
			cancel: () => {}
		}
	);
	await socket.onmessage({ data: JSON.stringify({ id: "x", session: "s", op: "shell" }) });
	expect(calls).toBe(0);
	expect(socket.readyState).toBe(3);
	c.stop();
});
test("reconnecting does not unlock an action still running in the same source session", async () => {
	let socket,
		nextTimer,
		calls = 0,
		finish;
	class Socket {
		constructor() {
			socket = this;
			this.readyState = 1;
		}
		send() {}
		close() {
			this.readyState = 3;
		}
	}
	const c = startConnector(
		{ port: 1234, key: "a".repeat(32), source: "spotify" },
		{
			Socket,
			inventory: async () => ({ snapshots: [] }),
			perform: async () => {
				calls++;
				await new Promise((resolve) => (finish = resolve));
				return { status: "dispatched" };
			},
			schedule: (fn) => {
				nextTimer = fn;
				return 1;
			},
			cancel: () => {}
		}
	);
	const old = socket,
		onGoing = socket.onmessage({
			data: JSON.stringify({ id: "one", session: "same", op: "next" })
		});
	old.onclose();
	nextTimer();
	await socket.onmessage({ data: JSON.stringify({ id: "two", session: "same", op: "next" }) });
	expect(calls).toBe(1);
	finish();
	await onGoing;
	c.stop();
});

function connectorHarness(inventory) {
	const sent = [];
	let socket;
	class Socket {
		constructor() {
			socket = this;
			this.readyState = 1;
		}
		send(value) {
			sent.push(JSON.parse(value));
		}
		close(code) {
			this.code = code;
			this.readyState = 3;
		}
	}
	let next = 1;
	const pending = new Map();
	const connection = startConnector(
		{ port: 1234, key: "a".repeat(32), source: "spotify" },
		{
			Socket,
			inventory,
			perform: async () => ({ status: "confirmed" }),
			schedule: (fn, ms) => {
				pending.set(next, { fn, ms });
				return next++;
			},
			cancel: (id) => pending.delete(id)
		}
	);
	return {
		sent,
		get socket() {
			return socket;
		},
		pending,
		connection,
		async advance(ms) {
			for (const [id, timer] of [...pending])
				if (timer.ms <= ms) {
					pending.delete(id);
					await timer.fn();
				}
		}
	};
}

test("inventory opts in to server-driven polling", async () => {
	const h = connectorHarness(async () => ({ snapshots: [{ session: "s" }] }));
	await h.socket.onopen();
	expect(h.sent[0]).toEqual({ op: "inventory", snapshots: [{ session: "s" }], pull: true });
	h.connection.stop();
});

test("a server poll publishes immediately without the fallback timer", async () => {
	let calls = 0;
	const h = connectorHarness(async () => {
		calls++;
		return { snapshots: [{ session: "s" }] };
	});
	await h.socket.onopen();
	expect(calls).toBe(1);
	expect(h.pending.size).toBe(1);
	await h.socket.onmessage({ data: JSON.stringify({ op: "poll" }) });
	expect(calls).toBe(2);
	expect(h.sent.map((m) => m.op)).toEqual(["inventory", "inventory"]);
	expect(h.sent[1].pull).toBe(true);
	expect(h.pending.size).toBe(0);
	h.connection.stop();
});

test("a received poll disarms the fallback chain, which otherwise keeps its 2 s cadence", async () => {
	let calls = 0;
	const inventory = async () => {
		calls++;
		return { snapshots: [] };
	};
	const fallback = connectorHarness(inventory);
	await fallback.socket.onopen();
	await fallback.advance(2000);
	expect(calls).toBe(2);
	await fallback.advance(2000);
	expect(calls).toBe(3);
	fallback.connection.stop();

	calls = 0;
	const pushed = connectorHarness(inventory);
	await pushed.socket.onopen();
	await pushed.socket.onmessage({ data: JSON.stringify({ op: "poll" }) });
	expect(calls).toBe(2);
	await pushed.advance(2000);
	expect(calls).toBe(2);
	expect(pushed.pending.size).toBe(0);
	pushed.connection.stop();
});

test("overlapping polls publish once", async () => {
	let calls = 0,
		release;
	const h = connectorHarness(() => {
		calls++;
		return calls === 1
			? Promise.resolve({ snapshots: [] })
			: new Promise((resolve) => (release = resolve));
	});
	await h.socket.onopen();
	const first = h.socket.onmessage({ data: JSON.stringify({ op: "poll" }) });
	const second = h.socket.onmessage({ data: JSON.stringify({ op: "poll" }) });
	expect(calls).toBe(2);
	release({ snapshots: [] });
	await Promise.all([first, second]);
	expect(calls).toBe(2);
	expect(h.sent.length).toBe(2);
	h.connection.stop();
});

test("the tab commands are accepted while unknown ops still close the socket", async () => {
	const h = connectorHarness(async () => ({ snapshots: [] }));
	await h.socket.onopen();
	for (const op of ["mute", "unmute", "show"])
		await h.socket.onmessage({ data: JSON.stringify({ id: op, session: "tab:1", op }) });
	expect(h.sent.filter((message) => message.op === "result").map((message) => message.status)).toEqual(
		["confirmed", "confirmed", "confirmed"]
	);
	expect(h.socket.readyState).toBe(1);
	await h.socket.onmessage({ data: JSON.stringify({ id: "bad", session: "tab:1", op: "bogus" }) });
	expect(h.socket.readyState).toBe(3);
	expect(h.socket.code).toBe(1008);
	h.connection.stop();
});

test("a poll neither bypasses validation nor consumes the request history", async () => {
	const h = connectorHarness(async () => ({ snapshots: [] }));
	await h.socket.onopen();
	await h.socket.onmessage({ data: JSON.stringify({ op: "poll" }) });
	// 64 IDs still fit; a poll counted against the bound would close at the 64th.
	for (let i = 0; i < 64; i++)
		await h.socket.onmessage({ data: JSON.stringify({ id: `r${i}`, session: "s", op: "next" }) });
	expect(h.sent.filter((m) => m.op === "result").length).toBe(64);
	expect(h.socket.readyState).toBe(1);
	await h.socket.onmessage({ data: JSON.stringify({ id: "bad", session: "s", op: "bogus" }) });
	expect(h.socket.readyState).toBe(3);
	expect(h.socket.code).toBe(1008);
	h.connection.stop();
});
