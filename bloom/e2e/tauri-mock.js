// Tauri v2 mock for the e2e quota-space spec: enough of
// __TAURI_INTERNALS__ for @tauri-apps/api (invoke, transformCallback,
// metadata) so the Bloom frontend mounts in Chromium without a WebView2
// backend.
//
//   - every invoke(cmd, args) is logged to window.__invokes
//   - plugin:event|listen / unlisten are honored; tests emit with
//     window.__emit("quota-snapshot", readings) or "notch-pointer-left"
//   - load_settings returns {} so the app falls back to localStorage,
//     which is preset below (weather off, no first-run splash)
//   - getVersion() ("plugin:app|version") -> "3.10.0" matching
//     bloom-app-version so the splash wait is skipped
//   - any command ending in "is_visible" -> true so the startup
//     visibility poll completes and the notch becomes ready

(() => {
	const callbacks = new Map();
	let nextCallbackId = 1;
	const listeners = []; // { eventId, event, handler }

	window.__invokes = [];
	window.__listeners = listeners;

	// unlisten() in @tauri-apps/api/event calls this before plugin:event|unlisten
	window.__TAURI_EVENT_PLUGIN_INTERNALS__ = {
		unregisterListener(event, eventId) {
			const idx = listeners.findIndex((l) => l.eventId === eventId && l.event === event);
			if (idx >= 0) listeners.splice(idx, 1);
		}
	};

	window.__emit = (event, payload) => {
		for (const l of listeners.slice()) {
			if (l.event !== event) continue;
			const cb = callbacks.get(l.handler);
			if (cb) cb({ event, id: 0, payload });
		}
	};

	const invoke = (cmd, args) => {
		const safeArgs = args === undefined ? {} : args;
		try {
			window.__invokes.push({ cmd, args: JSON.parse(JSON.stringify(safeArgs)) });
		} catch {
			window.__invokes.push({ cmd, args: null });
		}
		if (cmd === "plugin:event|listen") {
			const eventId = nextCallbackId++;
			listeners.push({ eventId, event: safeArgs.event, handler: safeArgs.handler });
			return Promise.resolve(eventId);
		}
		if (cmd === "plugin:event|unlisten") {
			const idx = listeners.findIndex((l) => l.eventId === safeArgs.eventId);
			if (idx >= 0) listeners.splice(idx, 1);
			return Promise.resolve(undefined);
		}
		if (cmd === "plugin:app|version") return Promise.resolve("3.10.0");
		if (cmd === "load_settings") return Promise.resolve({});
		if (cmd === "get_update_state") return Promise.resolve({ available: false });
		if (cmd === "plugin:autostart|is_enabled") return Promise.resolve(true);
		if (/is_visible$/.test(cmd)) return Promise.resolve(true);
		if (cmd === "plugin:window|scale_factor") return Promise.resolve(1);
		return Promise.resolve(undefined);
	};

	const transformCallback = (callback, once = false) => {
		const id = nextCallbackId++;
		callbacks.set(id, (data) => {
			if (once) callbacks.delete(id);
			callback(data);
		});
		return id;
	};

	window.__TAURI_INTERNALS__ = {
		invoke,
		transformCallback,
		callbacks,
		metadata: {
			currentWindow: { label: "main" },
			currentWebview: { label: "main", windowLabel: "main" }
		},
		plugins: {}
	};

	// Skip the first-run splash + autostart path and keep weather quiet:
	// load_settings returns {}, so these localStorage fallbacks win.
	localStorage.setItem("bloom-first-run", "done");
	localStorage.setItem("bloom-app-version", "3.10.0");
	localStorage.setItem("bloom-weather-enabled", "false");
})();
