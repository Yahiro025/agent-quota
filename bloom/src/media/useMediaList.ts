import { useEffect, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { EMPTY_MEDIA_LIST, parseMediaList, type MediaList } from "./mediaList";

export type { MediaCard, MediaList, MediaSource } from "./mediaList";

/**
 * The full set of playing players. The single-slot `media-update` card stays
 * the pill's source; this only feeds the expanded panel's stack.
 */
export function useMediaList(): MediaList {
	const [list, setList] = useState<MediaList>(EMPTY_MEDIA_LIST);
	useEffect(() => {
		let alive = true;
		// The event can arrive before the mount reply; a late reply must not
		// overwrite a newer list with the state from before it.
		let seenEvent = false;
		const unlisten = listen<unknown>("media-list", (event) => {
			const next = parseMediaList(event.payload);
			if (!next) return;
			seenEvent = true;
			if (alive) setList(next);
		});
		invoke<unknown>("media_list_get")
			.then((raw) => {
				const next = parseMediaList(raw);
				if (alive && !seenEvent && next) setList(next);
			})
			.catch(() => {});
		return () => {
			alive = false;
			void unlisten.then((fn) => fn());
		};
	}, []);
	return list;
}
