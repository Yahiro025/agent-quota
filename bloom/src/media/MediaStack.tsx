import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import {
	AnimatePresence,
	LayoutGroup,
	motion,
	useReducedMotion,
	type Transition
} from "framer-motion";
import { invoke } from "@tauri-apps/api/core";
import {
	AppWindow,
	AudioLines,
	Clapperboard,
	Globe,
	ListMusic,
	MonitorPlay,
	Music2,
	Volume2,
	VolumeX
} from "lucide-react";
import { PauseIcon, PlayIcon, SkipBackIcon, SkipForwardIcon } from "../icons";
import { TitleMarquee } from "../CompactMediaPlayer";
import { QueueDrawer } from "./QueueDrawer";
import { pickFocus, progressPercent, type MediaCard } from "./mediaList";
import { formatRemaining, formatTime } from "./time";
import "./MediaStack.css";

type Op = "playPause" | "next" | "previous" | "show";

interface MediaStackProps {
	cards: MediaCard[];
	otherAudio: boolean;
	/** True while the focus block's drawer is open; the island grows for it. */
	queueExpanded: boolean;
	onQueueExpandedChange: (expanded: boolean) => void;
	/** Height the island must have to fit the whole stack, in island px. */
	onHeightChange: (height: number) => void;
	maxHeight: number;
	volume: number;
	onVolumeChange: (vol: number) => void;
	/** The focus card's artwork feeds the island glow; null when it has none. */
	onFocusArtworkChange: (art: string | null) => void;
	/** The focus block's height tells the island where the glow may start dimming. */
	onFocusHeightChange: (height: number) => void;
}

const EASE = [0.16, 1, 0.3, 1] as const;
const LAYOUT: Transition = { duration: 0.32, ease: EASE };
const FADE: Transition = { duration: 0.15 };
const TICK_MS = 250;

function act(generation: string, op: Op) {
	invoke("media_action", { generation, op }).catch(() => {});
}

/** One glyph per player family; the app name stays the source of truth. */
function AppGlyph({ app, size }: { app: string; size: number }) {
	const name = app.toLowerCase();
	const Icon = name.includes("spotify")
		? Music2
		: name.includes("youtube")
			? MonitorPlay
			: ["brave", "edge", "chrome", "firefox"].some((browser) => name.includes(browser))
				? Globe
				: Clapperboard;
	return <Icon size={size} aria-hidden="true" />;
}

function Artwork({
	card,
	size,
	radius,
	layoutId
}: {
	card: MediaCard;
	size: number;
	radius: number;
	layoutId?: string;
}) {
	const art = card.kind === "tab" ? null : card.artwork?.[0];
	return (
		<motion.div
			layoutId={layoutId}
			className={`msf-art${art ? "" : " is-glyph"}`}
			style={{ width: size, height: size, borderRadius: radius }}
		>
			{art ? (
				<img src={art} alt="" draggable={false} />
			) : card.kind === "tab" ? (
				<Globe size={Math.round(size * 0.46)} aria-hidden="true" />
			) : (
				<AppGlyph app={card.app} size={Math.round(size * 0.46)} />
			)}
		</motion.div>
	);
}

// The island is sized from the stack's whole natural height, so the same
// report the single card's drawer gives now has to include everything above
// the open drawer. The stack's top is measured against the island itself
// because the content starts below the island's rounded top edge.
function stackHeight(root: HTMLElement, cap: number) {
	const island = root.closest<HTMLElement>(".bloom") ?? root.parentElement;
	const box = root.getBoundingClientRect();
	// scrollHeight reports the root's stretched box, not its content, so a tall
	// island would never shrink back. The last child's bottom is the content end;
	// the root's own bottom padding is the breathing room above the island's
	// rounded edge, so it has to be part of the reported height.
	const padBottom = parseFloat(getComputedStyle(root).paddingBottom) || 0;
	const last = root.lastElementChild;
	const content = last
		? last.getBoundingClientRect().bottom - box.top + padBottom
		: root.scrollHeight;
	const scale = island && box.height > 0 && island.offsetHeight > 0 ? box.height / island.offsetHeight : 1;
	const top = island ? (box.top - island.getBoundingClientRect().top) / scale : 0;
	return Math.ceil(top + Math.min(content, cap));
}

function islandScale(el: HTMLElement) {
	const island = el.closest<HTMLElement>(".bloom");
	if (!island) return 1;
	const box = island.getBoundingClientRect();
	return box.height > 0 && island.offsetHeight > 0 ? box.height / island.offsetHeight : 1;
}

export function MediaStack({
	cards,
	otherAudio,
	queueExpanded,
	onQueueExpandedChange,
	onHeightChange,
	maxHeight,
	volume,
	onVolumeChange,
	onFocusArtworkChange,
	onFocusHeightChange
}: MediaStackProps) {
	const reduce = useReducedMotion();
	const [focusId, setFocusId] = useState<string | null>(null);
	const [now, setNow] = useState(() => Date.now());
	const [seeking, setSeeking] = useState<number | null>(null);
	const seekGeneration = useRef<string | null>(null);
	const seeked = useRef<{ generation: string; position: number; at: number } | null>(null);
	const rootRef = useRef<HTMLDivElement>(null);
	const focusRef = useRef<HTMLDivElement>(null);
	const queueRef = useRef<HTMLDivElement>(null);
	const titleViewRef = useRef<HTMLDivElement>(null);
	// Only an overflowing title gets the right-edge fade; a short one must end clean.
	const [titleOverflows, setTitleOverflows] = useState(false);

	// Derived, so a card that disappears hands focus over in the same frame.
	const focus = pickFocus(cards, focusId);
	const focusGeneration = focus?.generation ?? null;
	const playing = focus?.is_playing ?? false;
	const duration = focus?.duration_ms ?? 0;

	// Mute is one click away from what the last list said, so the button flips
	// before the round trip. An override lives until a list agrees with it or
	// names the card gone; a rejected action drops it and the truth returns.
	const [muteOverrides, setMuteOverrides] = useState<Record<string, boolean>>({});
	const mutedOf = (card: MediaCard) => muteOverrides[card.generation] ?? card.muted;
	const setMute = (card: MediaCard, muted: boolean) => {
		setMuteOverrides((prev) => ({ ...prev, [card.generation]: muted }));
		invoke("media_action", { generation: card.generation, op: muted ? "mute" : "unmute" }).catch(
			() => {
				setMuteOverrides((prev) => {
					const next = { ...prev };
					delete next[card.generation];
					return next;
				});
			}
		);
	};
	useEffect(() => {
		setMuteOverrides((prev) => {
			let stale = false;
			const next = { ...prev };
			for (const [generation, muted] of Object.entries(prev)) {
				const card = cards.find((c) => c.generation === generation);
				if (!card || card.muted === muted) {
					delete next[generation];
					stale = true;
				}
			}
			return stale ? next : prev;
		});
	}, [cards]);

	useEffect(() => {
		if (!playing) return;
		const id = setInterval(() => setNow(Date.now()), TICK_MS);
		return () => clearInterval(id);
	}, [playing]);

	// A promoted card starts its own clock; a stale tick would flash the old position.
	useEffect(() => {
		setSeeking(null);
		setNow(Date.now());
	}, [focusGeneration]);

	// The marquee wrapper is the viewport: measure its text against it so the
	// fade only exists while the title is actually cut off.
	useLayoutEffect(() => {
		const view = titleViewRef.current;
		if (!view) return;
		const check = () => {
			const text = view.querySelector<HTMLElement>(".cmp-title");
			setTitleOverflows(!!text && text.scrollWidth > view.clientWidth + 1);
		};
		check();
		const observer = new ResizeObserver(check);
		observer.observe(view);
		const text = view.querySelector<HTMLElement>(".cmp-title");
		if (text) observer.observe(text);
		return () => observer.disconnect();
	}, [focusGeneration, focus?.title]);

	// A player with no connector cannot list a queue, so its focus has none.
	useEffect(() => {
		if (queueExpanded && !focus?.queue) onQueueExpandedChange(false);
	}, [queueExpanded, focus?.queue, onQueueExpandedChange]);

	// Promotion closes the drawer: it belongs to the player that was focused.
	const lastFocus = useRef(focusGeneration);
	useEffect(() => {
		if (lastFocus.current === focusGeneration) return;
		lastFocus.current = focusGeneration;
		if (queueExpanded) onQueueExpandedChange(false);
	}, [focusGeneration, queueExpanded, onQueueExpandedChange]);

	const art = focus?.artwork?.[0] ?? null;
	useEffect(() => {
		onFocusArtworkChange(art);
	}, [art, onFocusArtworkChange]);
	useEffect(() => () => onFocusArtworkChange(null), [onFocusArtworkChange]);

	const measure = useCallback(() => {
		const root = rootRef.current;
		if (!root) return;
		onHeightChange(stackHeight(root, maxHeight));
		const block = focusRef.current;
		if (!block) {
			onFocusHeightChange(0);
			return;
		}
		// The drawer must not push the glow's dim boundary down with it: the
		// queue's small text needs the calm black the shelf already sits on.
		const drawer = queueRef.current?.offsetHeight ?? 0;
		onFocusHeightChange(
			Math.max(0, Math.ceil(block.getBoundingClientRect().height / islandScale(block)) - drawer)
		);
	}, [onHeightChange, onFocusHeightChange]);
	// Layout effect: the island's new height must be known before the frame paints.
	useLayoutEffect(measure);
	useEffect(() => {
		const root = rootRef.current;
		if (!root) return;
		const observer = new ResizeObserver(measure);
		observer.observe(root);
		// A card that grows under the stack's cap leaves the root's box unchanged.
		for (const card of Array.from(root.children)) observer.observe(card);
		return () => observer.disconnect();
	}, [measure, cards.length, queueExpanded, focusGeneration]);

	let elapsed = 0;
	if (focus && duration > 0) elapsed = (progressPercent(focus, now) / 100) * duration;
	const pending = seeked.current;
	if (focus && pending && pending.generation === focusGeneration && focus.position_updated_at <= pending.at) {
		elapsed = Math.min(
			duration,
			pending.position + (focus.is_playing ? Math.max(0, now - pending.at) : 0)
		);
	}
	const shown = seeking ?? elapsed;
	const percent = duration > 0 ? Math.min(100, Math.max(0, (shown / duration) * 100)) : 0;

	const startSeek = () => {
		if (!focus?.seek_enabled) return;
		seekGeneration.current = focus.generation;
	};
	const changeSeek = (event: React.ChangeEvent<HTMLInputElement>) => {
		setSeeking(Number(event.target.value));
	};
	const endSeek = () => {
		if (seeking === null) return;
		const generation = seekGeneration.current;
		const positionMs = seeking;
		seekGeneration.current = null;
		setSeeking(null);
		if (!generation || generation !== focusGeneration) return;
		seeked.current = { generation, position: positionMs, at: Date.now() };
		setNow(Date.now());
		invoke("media_action", { generation, op: "seek", positionMs }).catch(() => {});
	};

	const others = cards.filter((card) => card.generation !== focusGeneration);
	const queueOpen = queueExpanded && !!focus?.queue;

	return (
		<LayoutGroup>
			<div className="msf" ref={rootRef} style={{ maxHeight }}>
				{focus && (
					<motion.div
						ref={focusRef}
						className="msf-focus"
						layout={!reduce}
						transition={reduce ? FADE : LAYOUT}
						initial={reduce ? { opacity: 0 } : false}
						animate={{ opacity: 1 }}
						role="group"
						aria-label={`${focus.app}: ${focus.title || "Unknown"}`}
					>
						<div className="msf-focus-info">
							<Artwork
								card={focus}
								size={46}
								radius={10}
								layoutId={reduce ? undefined : `art-${focus.generation}`}
							/>
							<div className="msf-focus-text">
								<div
								ref={titleViewRef}
								className={`msf-focus-title${titleOverflows ? " is-overflowing" : ""}`}
								title={focus.title || "Unknown"}
							>
									<TitleMarquee title={focus.title || "Unknown"} />
								</div>
								<span className="msf-focus-artist" title={focus.artist}>
									{focus.artist || focus.app}
								</span>
							</div>
							<span className="msf-app-chip" title={focus.app}>
								<AppGlyph app={focus.app} size={12} />
								<span className="msf-app-name">{focus.app}</span>
							</span>
						</div>

						{focus.kind === "tab" ? (
							<div className="msf-tab-actions">
								<button
									className="msf-tab-btn"
									onClick={() => setMute(focus, !mutedOf(focus))}
								>
									{mutedOf(focus) ? <VolumeX size={14} /> : <Volume2 size={14} />}
									<span>{mutedOf(focus) ? "Unmute" : "Mute"}</span>
								</button>
								<button
									className="msf-tab-btn"
									onClick={() => act(focus.generation, "show")}
								>
									<AppWindow size={14} />
									<span>Show tab</span>
								</button>
							</div>
						) : (
							<>
								{duration > 0 && (
							<div className={`msf-scrub${focus.seek_enabled ? " is-seekable" : ""}`}>
								<span className="msf-time">{formatTime(shown)}</span>
								<div className="msf-track">
									<div className="msf-track-fill" style={{ width: `${percent}%` }} />
									{focus.seek_enabled && (
										<input
											type="range"
											className="msf-seek"
											aria-label="Seek"
											min={0}
											max={duration}
											step={100}
											value={shown}
											onMouseDown={startSeek}
											onTouchStart={startSeek}
											onChange={changeSeek}
											onMouseUp={endSeek}
											onTouchEnd={endSeek}
										/>
									)}
								</div>
								<span className="msf-time">{formatRemaining(shown, duration)}</span>
							</div>
						)}

						<div className="msf-transport">
							<div className="msf-transport-main">
								<button
									className="cmp-btn"
									aria-label="Previous track"
									title="Previous"
									onClick={() => act(focus.generation, "previous")}
								>
									<SkipBackIcon size={22} />
								</button>
								<button
									className="cmp-btn msf-play"
									aria-label={focus.is_playing ? "Pause" : "Play"}
									title={focus.is_playing ? "Pause" : "Play"}
									onClick={() => act(focus.generation, "playPause")}
								>
									{focus.is_playing ? <PauseIcon size={20} /> : <PlayIcon size={20} />}
								</button>
								<button
									className="cmp-btn"
									aria-label="Next track"
									title="Next"
									onClick={() => act(focus.generation, "next")}
								>
									<SkipForwardIcon size={22} />
								</button>
							</div>
							{focus.queue && (
								<button
									className={`cmp-btn cmp-btn-side cmp-queue-toggle msf-queue-toggle${queueOpen ? " is-active" : ""}`}
									aria-label="Up next"
									title="Up next"
									aria-expanded={queueOpen}
									onClick={() => onQueueExpandedChange(!queueExpanded)}
								>
									<ListMusic size={17} aria-hidden="true" />
								</button>
							)}
						</div>
							</>
						)}

						<AnimatePresence initial={false}>
							{queueOpen && (
								<motion.div
									key="queue"
									ref={queueRef}
									className="msf-drawer"
									initial={{ height: 0, opacity: 0 }}
									animate={{ height: "auto", opacity: 1 }}
									exit={{ height: 0, opacity: 0 }}
									transition={reduce ? FADE : { duration: 0.18, ease: EASE }}
								>
									<QueueDrawer generation={focus.generation} />
								</motion.div>
							)}
						</AnimatePresence>
					</motion.div>
				)}

				{/* While the drawer is open the shelf and footer give way to one quiet
				    line, so the queue never pushes them under the island's rounded edge. */}
				<AnimatePresence initial={false}>
					{queueOpen && others.length > 0 && (
						<motion.div
							key="also"
							className="msf-also-wrap"
							initial={{ height: 0, opacity: 0 }}
							animate={{ height: 32, opacity: 1 }}
							exit={{ height: 0, opacity: 0 }}
							transition={reduce ? FADE : LAYOUT}
						>
							<button
								className="msf-also"
								aria-label={`Close Up next and show ${others.length} more playing`}
								onClick={(event) => {
									event.stopPropagation();
									onQueueExpandedChange(false);
								}}
							>
								<span className="msf-also-avatars" aria-hidden="true">
									{others.slice(0, 3).map((card) => (
										<span key={card.generation} className="msf-also-art">
											{card.artwork?.[0] ? (
												<img src={card.artwork[0]} alt="" draggable={false} />
											) : (
												<AppGlyph app={card.app} size={9} />
											)}
										</span>
									))}
								</span>
								<span className="msf-also-text">{others.length} more playing</span>
							</button>
						</motion.div>
					)}
				</AnimatePresence>

				<motion.div
					className="msf-below"
					initial={false}
					animate={{ height: queueOpen ? 0 : "auto", opacity: queueOpen ? 0 : 1 }}
					transition={reduce ? { duration: 0 } : LAYOUT}
				>
				<AnimatePresence initial={false}>
					{others.length > 0 && (
						<motion.div
							key="shelf"
							className="msf-shelf"
							layout={!reduce}
							transition={reduce ? FADE : LAYOUT}
							initial={{ opacity: 0 }}
							animate={{ opacity: 1 }}
							exit={{ opacity: 0 }}
						>
							<AnimatePresence initial={false}>
								{others.map((card) => (
									<motion.div
										key={card.generation}
										className={`msf-row${card.kind === "tab" && mutedOf(card) ? " is-muted" : ""}`}
										layout={!reduce}
										transition={reduce ? FADE : LAYOUT}
										initial={reduce ? { opacity: 0 } : { height: 0, opacity: 0 }}
										animate={reduce ? { opacity: 1 } : { height: 40, opacity: 1 }}
										exit={reduce ? { opacity: 0 } : { height: 0, opacity: 0 }}
									>
										<button
											className="msf-row-open"
											aria-label={`Show ${card.title || "Unknown"}`}
											title={card.title || "Unknown"}
											onClick={() => setFocusId(card.generation)}
										>
											<Artwork
												card={card}
												size={26}
												radius={6}
												layoutId={reduce ? undefined : `art-${card.generation}`}
											/>
											<span className="msf-row-text">
												<span className="msf-row-title">{card.title || "Unknown"}</span>
												<span className="msf-row-sep" aria-hidden="true">
													·
												</span>
												<span className="msf-row-app">{card.app}</span>
											</span>
										</button>
										{card.kind === "tab" ? (
											<button
												className="msf-row-play"
												aria-label={`${mutedOf(card) ? "Unmute" : "Mute"} ${card.title || "Unknown"}`}
												title={mutedOf(card) ? "Unmute" : "Mute"}
												onClick={(event) => {
													event.stopPropagation();
													setMute(card, !mutedOf(card));
												}}
											>
												{mutedOf(card) ? <VolumeX size={16} /> : <Volume2 size={16} />}
											</button>
										) : (
											<button
												className="msf-row-play"
												aria-label={`${card.is_playing ? "Pause" : "Play"} ${card.title || "Unknown"}`}
												title={card.is_playing ? "Pause" : "Play"}
												onClick={(event) => {
													event.stopPropagation();
													act(card.generation, "playPause");
												}}
											>
												{card.is_playing ? <PauseIcon size={16} /> : <PlayIcon size={16} />}
											</button>
										)}
										{card.kind !== "tab" && (
											<span className="msf-row-progress" aria-hidden="true">
												<span style={{ width: `${progressPercent(card, now)}%` }} />
											</span>
										)}
									</motion.div>
								))}
							</AnimatePresence>
						</motion.div>
					)}
				</AnimatePresence>

				<div className="msf-footer">
					<span className="msf-other">
						{otherAudio && (
							<>
								<AudioLines size={12} aria-hidden="true" />
								<span>Other audio playing</span>
							</>
						)}
					</span>
					<div className="msf-volume">
						<Volume2 size={13} aria-hidden="true" />
						<input
							type="range"
							className="msf-volume-slider"
							aria-label="Volume"
							min={0}
							max={1}
							step={0.01}
							value={volume}
							style={{ "--vol": `${Math.round(volume * 100)}%` } as React.CSSProperties}
							onChange={(event) => onVolumeChange(parseFloat(event.target.value))}
							onClick={(event) => event.stopPropagation()}
						/>
					</div>
				</div>
				</motion.div>
			</div>
		</LayoutGroup>
	);
}
