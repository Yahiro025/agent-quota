import { useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { Play, RefreshCw } from "lucide-react";
import { QueueController } from "./queueState";
import "./QueueDrawer.css";

// The island grows to fit the drawer rather than a fixed slot, so a short or empty
// queue never leaves a blank band. This returns the whole island height the drawer
// needs: where the player above ends, the drawer's natural height, and the host's
// bottom padding. The list's content height is read (clamped to its max-height)
// because its box shrinks when the island is capped.
function islandHeight(section: HTMLElement) {
	const host = section.parentElement!;
	const hostBox = host.getBoundingClientRect();
	// Rects keep sub-pixel sizes; dividing by the host's rendered scale undoes the
	// island's entrance animation, which scales the content while it plays.
	const scale = hostBox.height > 0 && host.offsetHeight > 0 ? hostBox.height / host.offsetHeight : 1;
	const px = (v: string) => parseFloat(v) || 0;
	const s = getComputedStyle(section);
	let height = px(s.borderTopWidth) + px(s.paddingTop) + px(s.paddingBottom) + px(s.borderBottomWidth);
	for (const child of Array.from(section.children) as HTMLElement[]) {
		const c = getComputedStyle(child);
		const box = child.classList.contains("mq-list")
			? Math.min(child.scrollHeight, px(c.maxHeight) || Infinity)
			: child.getBoundingClientRect().height / scale;
		height += box + px(c.marginTop) + px(c.marginBottom);
	}
	const top = (section.getBoundingClientRect().top - hostBox.top) / scale;
	return Math.ceil(top + height + px(getComputedStyle(host).paddingBottom));
}

export function QueueDrawer({
	generation,
	onHeightChange
}: {
	generation: string;
	onHeightChange?: (height: number) => void;
}) {
	const controller = useMemo(() => {
		const c = new QueueController(invoke);
		c.target(generation);
		return c;
	}, [generation]);
	const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot);
	useEffect(() => {
		void controller.refresh();
		const unlisten = listen("media-queue-update", () => {
			void controller.refresh();
		});
		return () => {
			void unlisten.then((fn) => fn());
		};
	}, [controller]);
	const sectionRef = useRef<HTMLElement>(null);
	const listRef = useRef<HTMLOListElement>(null);
	const [fade, setFade] = useState(false);
	const updateFade = () => {
		const list = listRef.current;
		setFade(!!list && list.scrollTop + list.clientHeight < list.scrollHeight - 1);
	};
	// Layout effect: the island's new height must be known before the frame paints.
	useLayoutEffect(() => {
		if (sectionRef.current) onHeightChange?.(islandHeight(sectionRef.current));
		updateFade();
	});
	useEffect(() => {
		const section = sectionRef.current;
		if (!section) return;
		const observer = new ResizeObserver(() => {
			onHeightChange?.(islandHeight(section));
			updateFade();
		});
		observer.observe(section);
		return () => observer.disconnect();
	}, [onHeightChange]);
	const { view, pending, loading, message, needsCheck } = state;
	const blocked = !!pending || needsCheck || !!view?.pending || !!view?.needs_check;
	const check = () => {
		void controller.refresh(true);
	};
	return (
		<section
			ref={sectionRef}
			className="mq"
			aria-label="Up next"
			onClick={(e) => e.stopPropagation()}
			onWheel={(e) => e.stopPropagation()}
		>
			<header className="mq-header">
				<h2>Up next</h2>
				{view?.source && <span className="mq-source">{view.source}</span>}
				<button
					className="mq-check"
					aria-label="Check queue"
					title="Check queue"
					disabled={!!pending || !!view?.pending}
					onClick={check}
				>
					<RefreshCw size={13} aria-hidden="true" />
				</button>
			</header>
			{loading ? (
				<p className="mq-empty" role="status">
					Loading queue…
				</p>
			) : !view?.available ? (
				<p className="mq-empty">{view?.reason || "Queue unavailable for this player"}</p>
			) : view.entries.length === 0 ? (
				<p className="mq-empty">Nothing up next</p>
			) : (
				<ol
					ref={listRef}
					className={`mq-list${fade ? " has-more" : ""}${blocked ? " is-blocked" : ""}`}
					aria-label="Upcoming items"
					onScroll={updateFade}
				>
					{view.entries.map((entry, index) => (
						<li key={entry.id}>
							<button
								className="mq-row"
								disabled={blocked}
								aria-label={`Play ${entry.title || "Untitled"}${entry.artist ? ` by ${entry.artist}` : ""}, queue item ${index + 1}`}
								onClick={() => {
									void controller.select(entry.id);
								}}
								aria-busy={pending === entry.id}
								title={entry.title || "Untitled"}
							>
								<span className="mq-order" aria-hidden="true">
									{pending === entry.id ? (
										<span className="mq-pending" />
									) : (
										<>
											<span className="mq-index">{index + 1}</span>
											<Play size={11} fill="currentColor" />
										</>
									)}
								</span>
								<span className="mq-track">
									<span className="mq-title">{entry.title || "Untitled"}</span>
									{entry.artist && <span className="mq-artist">{entry.artist}</span>}
								</span>
								{entry.duration && <span className="mq-duration">{entry.duration}</span>}
							</button>
						</li>
					))}
				</ol>
			)}
			{view?.available && view.total_upcoming != null && view.total_upcoming > view.entries.length && (
				<p className="mq-note">
					Showing the next {view.entries.length} of {view.total_upcoming}
				</p>
			)}
			{(message || (!pending && view?.pending)) && (
				<p className="mq-note mq-status" role="status">
					<span>{message || "Waiting for playback confirmation…"}</span>
					{needsCheck && !pending && (
						<button className="mq-recheck" onClick={check}>
							Check again
						</button>
					)}
				</p>
			)}
		</section>
	);
}
