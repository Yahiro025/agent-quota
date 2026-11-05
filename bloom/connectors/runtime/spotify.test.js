import { expect, test } from "bun:test";

test("Spotify answers tab commands as unavailable without reading the player", async () => {
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
		close() {
			this.readyState = 3;
		}
	}
	const previous = globalThis.WebSocket;
	globalThis.WebSocket = Socket;
	globalThis.__BLOOM_MEDIA_CONFIG__ = { port: 1234, key: "a".repeat(32), source: "spotify" };
	try {
		await import("./spotify.js");
		for (const op of ["mute", "unmute", "show"])
			await socket.onmessage({ data: JSON.stringify({ id: op, session: "tab:1", op }) });
		expect(sent.filter((message) => message.op === "result")).toEqual([
			{ op: "result", id: "mute", status: "unavailable" },
			{ op: "result", id: "unmute", status: "unavailable" },
			{ op: "result", id: "show", status: "unavailable" }
		]);
	} finally {
		globalThis.__bloomMediaConnection?.stop();
		delete globalThis.__BLOOM_MEDIA_CONFIG__;
		globalThis.WebSocket = previous;
	}
});
