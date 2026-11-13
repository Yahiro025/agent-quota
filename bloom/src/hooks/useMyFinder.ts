import { useEffect, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { NO_MYFINDER, type MyFinderState } from "../myfinder";
import { onPixelRatioChange } from "../windowRegion";

export function useMyFinder() {
	const [state, setState] = useState(NO_MYFINDER);
	const [dpi, setDpi] = useState(window.devicePixelRatio);
	useEffect(() => onPixelRatioChange(() => setDpi(window.devicePixelRatio)), []);
	useEffect(() => {
		let disposed = false;
		let stop: (() => void) | undefined;
		const accept = (next: MyFinderState) => {
			if (!disposed) setState((current) => (next.revision >= current.revision ? next : current));
		};
		// Subscribe before reading, and reject an older initial response.
		void listen<MyFinderState>("myfinder-changed", (event) => accept(event.payload))
			.then(async (unlisten) => {
				if (disposed) {
					unlisten();
					return;
				}
				stop = unlisten;
				accept(await invoke<MyFinderState>("get_myfinder_state"));
			})
			.catch(() => {});
		return () => {
			disposed = true;
			stop?.();
		};
	}, []);
	return { state, dpi };
}
