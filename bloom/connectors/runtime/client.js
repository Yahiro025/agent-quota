// A single connection belongs to one source. Commands are never replayed after
// reconnection; the backend assigns a new target generation for that connection.
export function startConnector(
	config,
	{ inventory, perform, Socket = WebSocket, schedule = setTimeout, cancel = clearTimeout }
) {
	let stopped = false,
		socket,
		timer = null,
		retry = 1000,
		epoch = 0;
	// A reconnect changes transport identity, not the source operation in flight.
	const busy = new Set();
	function clear() {
		if (timer !== null) {
			cancel(timer);
			timer = null;
		}
	}
	function connect() {
		if (stopped) return;
		const ownEpoch = ++epoch,
			ws = new Socket(`ws://127.0.0.1:${config.port}/${config.source}`, ["bloom.v1", config.key]);
		socket = ws;
		const seen = new Set();
		let serverPolls = false,
			publishing = false;
		const current = () => !stopped && socket === ws && epoch === ownEpoch;
		const send = (value) => {
			if (!current() || ws.readyState !== 1) return;
			const data = JSON.stringify(value);
			if (data.length > 1048576 || new TextEncoder().encode(data).length > 1048576) {
				ws.close(1009);
				return;
			}
			ws.send(data);
		};
		async function publish() {
			try {
				const data = await inventory();
				if (!Array.isArray(data.snapshots) || data.snapshots.length > 16) {
					ws.close(1009);
					return;
				}
				send({ op: "inventory", ...data, pull: true });
			} catch {
				send({ op: "inventory", snapshots: [], unknown_playing: true, pull: true });
			}
		}
		// A server poll can land while the fallback chain is still awaiting the
		// source; dropping the overlap is safe because the next poll arrives in 2 s.
		async function publishOnce() {
			if (publishing) return;
			publishing = true;
			try {
				await publish();
			} finally {
				publishing = false;
			}
		}
		async function poll() {
			await publishOnce();
			if (current() && ws.readyState === 1 && !serverPolls) timer = schedule(poll, 2000);
		}
		ws.onopen = async () => {
			retry = 1000;
			await poll();
		};
		ws.onmessage = async (event) => {
			if (!current() || ws.readyState !== 1) return;
			let request;
			try {
				if (typeof event.data !== "string" || event.data.length > 1048576) throw new Error();
				request = JSON.parse(event.data);
				// The server pushes this once it has seen `pull:true`. Message events
				// escape the timer throttling that stalls the fallback chain.
				if (request && typeof request === "object" && request.op === "poll") {
					serverPolls = true;
					clear();
					await publishOnce();
					return;
				}
				if (
					typeof request.id !== "string" ||
					request.id.length > 128 ||
					typeof request.session !== "string" ||
					request.session.length > 256 ||
					![
						"select",
						"playPause",
						"next",
						"previous",
						"seek",
						"mute",
						"unmute",
						"show"
					].includes(request.op)
				)
					throw new Error();
			} catch {
				ws.close(1008);
				return;
			}
			if (seen.has(request.id)) return;
			// Close at the history bound instead of forgetting an ID and risking replay.
			if (seen.size >= 64) {
				ws.close(1000);
				return;
			}
			seen.add(request.id);
			if (busy.has(request.session) || busy.size >= 16) {
				send({ op: "result", id: request.id, status: "busy" });
				return;
			}
			busy.add(request.session);
			try {
				const result = await perform(request);
				send({ op: "result", id: request.id, ...result });
			} catch {
				send({ op: "result", id: request.id, status: "uncertain" });
			} finally {
				busy.delete(request.session);
			}
		};
		ws.onerror = () => ws.close();
		ws.onclose = () => {
			if (current()) {
				clear();
				timer = schedule(connect, retry);
				retry = Math.min(retry * 2, 10000);
			}
		};
	}
	connect();
	return {
		stop() {
			stopped = true;
			++epoch;
			clear();
			socket?.close();
		}
	};
}
