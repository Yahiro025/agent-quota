import { motion, AnimatePresence, useAnimation, useReducedMotion } from "framer-motion";
import {
	useEffect,
	useLayoutEffect,
	useReducer,
	useState,
	useCallback,
	useMemo,
	useRef,
	memo
} from "react";
import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { getCurrentWebviewWindow } from "@tauri-apps/api/webviewWindow";
import { getVersion } from "@tauri-apps/api/app";
import type { UpdateCheckResult } from "./updater";
import { createRegionSender, measureElements, notchPillBox, type RegionRect } from "./windowRegion";
import "./App.css";
import "./myfinder.css";
import { band } from "./quota/protocol";
import {
	fitQuotaStrip,
	stripHeadline,
	stripSecond,
	stripShowsValue,
	worstBand
} from "./quota/stripFit";

// Clear bar between MyFinder's menu and Bloom's strip, in CSS pixels.
const STRIP_MENU_GAP = 8;
// Expand and Pin appear on hover; their width is always held back so the
// strip does not grow over the menu when the pointer arrives.
const STRIP_CONTROLS_WIDTH = 48;
import { initTheme } from "./theme";
import {
	PlayIcon,
	PauseIcon,
	SkipBackIcon,
	SkipForwardIcon,
	VolumeLowIcon,
	VolumeHighIcon,
	MusicNoteIcon,
	HeadphonesIcon
} from "./icons";
import { CompactMediaPlayer } from "./CompactMediaPlayer";
import { MediaStack } from "./media/MediaStack";
import { useMediaList } from "./media/useMediaList";
import { useSettingsSync } from "./hooks/useSettingsSync";
import { useTrailingThrottle } from "./hooks/useTrailingThrottle";
import { reloadIfMirrorWasStale } from "./hooks/settingsMirror";
import { writeText } from "@tauri-apps/plugin-clipboard-manager";
import { startDrag } from "@crabnebula/tauri-plugin-drag";
import type { WidgetConfig } from "./components/StatusWidgetConfig";
import { EdgeSurface } from "./components/EdgeSurface";
import { useMyFinder } from "./hooks/useMyFinder";
import { constrainBarRegions, fittedBarHeight, myfinderIdleHidden, myfinderMaterial } from "./myfinder";
import { ShelfTypeIcon } from "./components/ShelfTypeIcon";
import { QuotaChip, QuotaHoverCard, QuotaPanel, QuotaRingMark } from "./quota/QuotaPanel";
import { useSlidingPad } from "./quota/useSlidingPad";
import { QuotaAlert } from "./quota/QuotaAlert";
import { parseSnapshot, urgent, type QuotaAgent } from "./quota/protocol";
import { AgentMark, agentName } from "./quota/marks";
import { alertRows, hoverQuotaAgent, nextAgent, quotaView } from "./quota/quotaView";
import { reduceAttention } from "./quota/attention";
import {
	parseHiddenAgents,
	visibleReadings,
	QUOTA_HIDDEN_AGENTS_KEY
} from "./quota/agentVisibility";
import { DEFAULT_PANEL, OverviewPanel, overviewRows } from "./quota/OverviewPanel";
import {
	Cpu,
	HardDrive,
	ArrowUpDown,
	ArrowUp,
	ArrowDown,
	BellRing,
	Play,
	Pause,
	RotateCcw,
	Type,
	X,
	Inbox,
	Music,
	Maximize2,
	Pin
} from "lucide-react";

/**
 * The notch has two kinds of content:
 * - Panels are tools the user chooses. A click opens the last-used panel by
 *   default, or a selected fixed panel when panel memory is turned off.
 * - Live activities (music, a set timer, a filled shelf) show a compact form in
 *   the pill or a bubble beside it, and opening one shows its own view, which
 *   stands alone (no panel arrows) and is never remembered as the last panel.
 *   Dragging files onto the notch also opens the shelf. Wheel/swipe switches
 *   between tool panels, including when a live activity occupies the pill.
 */
type NotchPanel = "timer" | "quotas" | "overview";
const NOTCH_PANELS: readonly NotchPanel[] = ["timer", "quotas", "overview"];
const asNotchPanel = (value: unknown): NotchPanel | null =>
	NOTCH_PANELS.includes(value as NotchPanel) ? (value as NotchPanel) : null;

// Shared panel entrance/exit handoff animation across notch tool views.
// NOTE: Only apply to panels whose content does not rely on .bloom as containing block.
// A transform or filter on a wrapper makes it the containing block for absolutely
// positioned descendants, collapsing quick panels (.qp) whose size resolves against .bloom.
const PANEL_HANDOFF = {
	initial: { opacity: 0, scale: 0.96, y: -4, filter: "blur(6px)" },
	animate: { opacity: 1, scale: 1, y: 0, filter: "blur(0px)", transitionEnd: { filter: "none" } },
	exit: { opacity: 0, scale: 0.97, filter: "blur(4px)", transition: { duration: 0.1 } },
	transition: { type: "spring", stiffness: 420, damping: 32, mass: 0.85 }
} as const;

// Unscaled variant for panels (like quotas) whose children measure client rects on mount.
const PANEL_HANDOFF_UNSCALED = {
	initial: { opacity: 0, filter: "blur(6px)" },
	animate: { opacity: 1, filter: "blur(0px)", transitionEnd: { filter: "none" } },
	exit: { opacity: 0, filter: "blur(4px)", transition: { duration: 0.1 } },
	transition: { type: "spring", stiffness: 420, damping: 32, mass: 0.85 }
} as const;

const PANEL_HANDOFF_REDUCED = {
	initial: { opacity: 0 },
	animate: { opacity: 1 },
	exit: { opacity: 0, transition: { duration: 0 } },
	transition: { duration: 0 }
} as const;

// Pomodoro timer limit.
const MAX_TIMER_SECONDS = 180 * 60;

// Inline timer editing: digits fill from the right and the colon is inserted
// automatically ("130" -> 1:30, "2500" -> 25:00, "45" -> 0:45).
const timerDigitsToSeconds = (digits: string): number | null => {
	if (!digits) return null;
	const secs = parseInt(digits.slice(-2) || "0", 10);
	const mins = parseInt(digits.slice(0, -2) || "0", 10);
	const total = mins * 60 + secs;
	return total > 0 && total <= MAX_TIMER_SECONDS ? total : null;
};

const timerSecondsToDigits = (seconds: number): string => {
	const mins = Math.floor(seconds / 60);
	const secs = seconds % 60;
	return `${mins}${secs.toString().padStart(2, "0")}`.replace(/^0+(?=\d)/, "");
};

const formatTimerDigits = (digits: string): string => {
	if (!digits) return "0:00";
	const secs = digits.slice(-2).padStart(2, "0");
	const mins = parseInt(digits.slice(0, -2) || "0", 10);
	return `${mins}:${secs}`;
};

// Completion chime for the Pomodoro timer. Synthesized with the Web Audio API
// so no audio asset is needed; created on the Start click so the webview's
// autoplay policy lets it play when the timer ends.
let timerChimeCtx: AudioContext | null = null;

const getTimerChimeCtx = (): AudioContext | null => {
	try {
		if (!timerChimeCtx) timerChimeCtx = new AudioContext();
		if (timerChimeCtx.state === "suspended") timerChimeCtx.resume().catch(() => {});
		return timerChimeCtx;
	} catch {
		return null;
	}
};

const playTimerChime = () => {
	const ctx = getTimerChimeCtx();
	if (!ctx) return;
	const start = ctx.currentTime + 0.02;
	const master = ctx.createGain();
	master.gain.value = 0.45;
	master.connect(ctx.destination);

	// Soft rising bell arpeggio (A5–C#6–E6) with a quiet octave harmonic.
	const notes = [
		{ freq: 880.0, at: 0 },
		{ freq: 1108.73, at: 0.18 },
		{ freq: 1318.51, at: 0.36 }
	];
	notes.forEach(({ freq, at }) => {
		const osc = ctx.createOscillator();
		const harmonic = ctx.createOscillator();
		const gain = ctx.createGain();
		const harmonicGain = ctx.createGain();
		osc.type = "sine";
		osc.frequency.value = freq;
		harmonic.type = "sine";
		harmonic.frequency.value = freq * 2.01;
		harmonicGain.gain.value = 0.12;
		gain.gain.setValueAtTime(0.0001, start + at);
		gain.gain.exponentialRampToValueAtTime(0.32, start + at + 0.02);
		gain.gain.exponentialRampToValueAtTime(0.0001, start + at + 1.4);
		osc.connect(gain);
		harmonic.connect(harmonicGain);
		harmonicGain.connect(gain);
		gain.connect(master);
		osc.start(start + at);
		harmonic.start(start + at);
		osc.stop(start + at + 1.5);
		harmonic.stop(start + at + 1.5);
	});
};

// Simple SVG icons
function BatteryIcon({
	charging,
	level,
	threshold = 20
}: {
	charging: boolean;
	level: number;
	threshold?: number;
}) {
	const percentage = Math.min(Math.max(level, 0), 100);

	return (
		<div
			style={{
				display: "flex",
				alignItems: "center",
				position: "relative",
				height: "14px",
				justifyContent: "center"
			}}
		>
			<svg width="20" height="10" viewBox="0 0 20 10" fill="none">
				{/* Battery Shell - Centered at 9px within 20px width, ignoring the tip's offset */}
				<rect
					x="2"
					y="0.75"
					width="14"
					height="8.5"
					rx="2.4"
					stroke="currentColor"
					strokeOpacity={0.35}
					strokeWidth="1.1"
				/>
				{/* Battery Tip */}
				<path
					d="M17.5 3.5V6.5"
					stroke="currentColor"
					strokeOpacity={0.35}
					strokeWidth="1.2"
					strokeLinecap="round"
				/>
				{/* Fill */}
				<rect
					x="3.8"
					y="2.5"
					width={Math.max(0.5, (percentage / 100) * 10.4)}
					height="5"
					rx="1"
					fill={charging ? "#32D74B" : percentage <= threshold ? "#FF453A" : "white"}
				/>
			</svg>
			{/* Charging Bolt - Centered on the battery body */}
			{charging && (
				<div
					style={{
						position: "absolute",
						top: "50%",
						left: "9px",
						transform: "translate(-50%, -50%)",
						color: "white",
						filter: "drop-shadow(0px 0px 1.5px rgba(0,0,0,0.8))"
					}}
				>
					<svg width="7" height="10" viewBox="0 0 8 12" fill="currentColor">
						<path d="M4.5 0L0 7H3.5L2.5 12L8 5H4.5L5.5 0H4.5Z" />
					</svg>
				</div>
			)}
		</div>
	);
}

function GreenDownArrowIcon() {
	return (
		<div
			style={{
				display: "inline-flex",
				alignItems: "center",
				justifyContent: "center",
				width: "18px",
				height: "18px",
				borderRadius: "50%",
				background: "rgba(50, 215, 75, 0.15)",
				border: "1px solid rgba(50, 215, 75, 0.35)",
				boxShadow: "0 0 8px rgba(50, 215, 75, 0.25)",
				flexShrink: 0,
				verticalAlign: "middle"
			}}
		>
			<svg
				width="10"
				height="10"
				viewBox="0 0 24 24"
				fill="none"
				stroke="#32D74B"
				strokeWidth="3"
				strokeLinecap="round"
				strokeLinejoin="round"
			>
				<line x1="12" y1="4" x2="12" y2="16"></line>
				<polyline points="18 10 12 16 6 10"></polyline>
				<line x1="6" y1="20" x2="18" y2="20"></line>
			</svg>
		</div>
	);
}

export const Visualizer = memo(function Visualizer({
	isPlaying,
	bars = 5,
	height = 20
}: {
	isPlaying: boolean;
	bars?: number;
	height?: number;
}) {
	const [audioData, setAudioData] = useState<number[]>(new Array(bars).fill(0.18));

	useEffect(() => {
		if (!isPlaying) {
			setAudioData(new Array(bars).fill(0.18));
			return;
		}

		const unlisten = listen<{ frequencies: number[] }>("audio-visualization", (event) => {
			// If we receive fewer frequencies than bars, repeat or interpolate
			// If more, slice
			let data = event.payload.frequencies;
			if (data.length > bars) data = data.slice(0, bars);
			while (data.length < bars) data.push(0.18);
			setAudioData(data);
		});

		return () => {
			unlisten.then((fn) => fn());
		};
	}, [isPlaying, bars]);

	return (
		<div
			className="visualizer-horizontal"
			style={{ height: `${height}px`, width: `${bars * 6}px` }}
		>
			{audioData.map((value, i) => (
				<motion.div
					key={i}
					className="bar-horizontal"
					animate={{
						scaleY: isPlaying ? Math.max(0.2, value) : 0.1,
						opacity: isPlaying ? 0.95 : 0.5
					}}
					transition={{
						type: "spring",
						stiffness: 600,
						damping: 30,
						mass: 0.5
					}}
				/>
			))}
		</div>
	);
});

// Odometer-style digit for the timer clock. When the value changes the old
// digit rolls up and out while the new one rolls in from below with a slight
// 3D tilt — no card chrome, just the numerals.
const RollDigit = memo(function RollDigit({
	value,
	compact = false
}: {
	value: string;
	compact?: boolean;
}) {
	return (
		<span className={`roll-digit ${compact ? "compact" : ""}`}>
			<AnimatePresence initial={false}>
				<motion.span
					key={value}
					className="roll-num"
					initial={{ y: "72%", rotateX: -50, opacity: 0 }}
					animate={{ y: "0%", rotateX: 0, opacity: 1 }}
					exit={{ y: "-72%", rotateX: 50, opacity: 0 }}
					transition={{ duration: 0.4, ease: [0.22, 1, 0.36, 1] }}
				>
					{value}
				</motion.span>
			</AnimatePresence>
		</span>
	);
});

interface MediaInfo {
	target_generation?: string;
	/** Display name of the player that owns this media. */
	app?: string;
	title: string;
	artist: string;
	is_playing: boolean;
	has_media: boolean;
	artwork?: string[];
	position_ms?: number;
	duration_ms?: number;
	seek_enabled?: boolean;
	position_updated_at?: number;
	lyrics?: import("./media/lyrics").Lyrics;
}

interface AudioSessionInfo {
	pid: number;
	name: string;
	process_path: string | null;
	volume: number;
	is_muted: boolean;
}

/** A mixer session matches a media app by its display name or exe stem. */
const appMatchesSession = (session: AudioSessionInfo, app: string) => {
	const target = app.toLowerCase();
	const name = session.name.toLowerCase();
	const stem = (session.process_path?.split(/[\\/]/).pop() ?? "")
		.replace(/\.exe$/i, "")
		.toLowerCase();
	// Vendor prefixes: "Microsoft Edge", "Google Chrome".
	return name === target || name.endsWith(` ${target}`) || stem === target;
};


// A shelf file's preview from the backend: a picture when one can be made,
// else the opening text of a document or text file, and whether it's a folder
// (for the placeholder icon).
interface ShelfPreview {
	thumb: string | null;
	text: string | null;
	folder: boolean;
}

interface ShelfItem {
	id: string;
	type: "file" | "text";
	name: string;
	path?: string;
	text?: string;
	timestamp: number;
}

type BloomMode =
	| "music"
	| "timer"
	| "status"
	| "shelf"
	| "quotas"
	| "overview";

// The shelf is a short-lived stash: one row, newest first, oldest evicted.
const SHELF_MAX_ITEMS = 10;

// The queue drawer's tallest slot under the compact player; past it the list scrolls.
const QUEUE_MAX_HEIGHT = 236;

const MARQUEE_SPEED = 30; // px/s — constant for all titles
const MARQUEE_MIN_DURATION = 5; // floor so short titles don't flicker

const TitleMarquee = ({ title }: { title: string }) => {
	const textRef = useRef<HTMLSpanElement>(null);
	const containerRef = useRef<HTMLDivElement>(null);
	const [isOverflowing, setIsOverflowing] = useState(false);
	const [scrollDistance, setScrollDistance] = useState(0);
	const [scrollDuration, setScrollDuration] = useState(8);

	useEffect(() => {
		const check = () => {
			if (textRef.current && containerRef.current) {
				const textWidth = textRef.current.scrollWidth;
				const containerWidth = containerRef.current.clientWidth;
				const overflow = textWidth > containerWidth;
				setIsOverflowing(overflow);
				if (overflow) {
					const distance = textWidth - containerWidth;
					setScrollDistance(distance);
					setScrollDuration(Math.max(distance / MARQUEE_SPEED, MARQUEE_MIN_DURATION));
				}
			}
		};
		check();
		const observer = new ResizeObserver(check);
		if (containerRef.current) observer.observe(containerRef.current);
		return () => observer.disconnect();
	}, [title]);

	return (
		<div
			ref={containerRef}
			className="premium-title-wrap"
			style={{ width: "100%", overflow: "hidden" }}
		>
			<span
				ref={textRef}
				className={`premium-title ${isOverflowing ? "marquee" : ""}`}
				style={
					{
						"--scroll-distance": isOverflowing ? `-${scrollDistance}px` : undefined,
						"--scroll-duration": `${scrollDuration}s`
					} as React.CSSProperties
				}
			>
				{title}
			</span>
		</div>
	);
};

function App() {
	const reduceMotion = useReducedMotion();
	const { state: myfinder, dpi: displayDpi } = useMyFinder();
	const integrated = myfinder.available && myfinder.height > 0;
	const finderMaterial = myfinderMaterial(myfinder.color);
	const panelHandoff = reduceMotion ? PANEL_HANDOFF_REDUCED : PANEL_HANDOFF;
	const panelHandoffUnscaled = reduceMotion ? PANEL_HANDOFF_REDUCED : PANEL_HANDOFF_UNSCALED;
	useEffect(() => {
		return initTheme();
	}, []);

	const [isHovered, setIsHovered] = useState(false);
	// Expanded into a panel by a click. Hover alone only peeks.
	const [isOpen, setIsOpen] = useState(() => localStorage.getItem("bloom-quota-pinned") === "true");
	const isOpenRef = useRef(false);
	isOpenRef.current = isOpen;
	const [isReady, setIsReady] = useState(false);
	const [scale, setScale] = useState(() =>
		parseFloat(localStorage.getItem("bloom-scale") || "1.0")
	);
	const [timeFormat24h, setTimeFormat24h] = useState(
		() => localStorage.getItem("bloom-time-format-24h") === "true"
	);

	const [batteryLevel, setBatteryLevel] = useState(100);
	const [isCharging, setIsCharging] = useState(false);
	// The status widget whose tooltip is open, the side of the pill it hangs
	// from, and the distance from the widget to just past that edge.
	const [statusTooltip, setStatusTooltip] = useState<{
		id: string;
		side: "below" | "left" | "right";
		offset: number;
	} | null>(null);
	const statusTooltipTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const [clusterWidth, setClusterWidth] = useState(0);
	// Width of the bar row apart from the quota housing: the clock, gaps and padding.
	const [stripOther, setStripOther] = useState(120);
	const [showPowerPulse, setShowPowerPulse] = useState(false);
	const [showLowBatteryPulse, setShowLowBatteryPulse] = useState(false);
	const [lowBatteryThreshold, setLowBatteryThreshold] = useState(() =>
		parseInt(localStorage.getItem("bloom-low-battery-threshold") || "20")
	);
	const prevChargingRef = useRef<boolean | null>(null);
	const powerPulseTimeoutRef = useRef<any>(null);
	const lowBatteryPulseShownRef = useRef<boolean>(false);

	const [eventPeek, setEventPeek] = useState(false);
	const eventPeekTimeoutRef = useRef<any>(null);
	// Holds a peek-mode notch open while an urgent quota alert lives; the
	// attention controller (declared below) feeds it.
	const [quotaAlertHold, setQuotaAlertHold] = useState(false);
	const updatePulseTimerRef = useRef<any>(null);
	const triggerEventPeek = useCallback((duration = 3000) => {
		setEventPeek(true);
		if (eventPeekTimeoutRef.current) clearTimeout(eventPeekTimeoutRef.current);
		eventPeekTimeoutRef.current = setTimeout(() => setEventPeek(false), duration);
	}, []);

	const [notchMode, setNotchMode] = useState(() => {
		const raw = localStorage.getItem("bloom-notch-mode") || "fixed";
		if (raw === "auto-hide") return "smart";
		return raw;
	});

	useEffect(() => {
		if (isReady && prevChargingRef.current !== null && prevChargingRef.current !== isCharging) {
			setShowPowerPulse(true);
			if (notchMode === "peek") triggerEventPeek(4000);
			if (powerPulseTimeoutRef.current) clearTimeout(powerPulseTimeoutRef.current);
			powerPulseTimeoutRef.current = setTimeout(() => {
				setShowPowerPulse(false);
			}, 4000);
		}
		prevChargingRef.current = isCharging;
	}, [isCharging, isReady, notchMode, triggerEventPeek]);

	useEffect(() => {
		// Trigger pulse when dropping below threshold while discharging
		if (
			isReady &&
			batteryLevel <= lowBatteryThreshold &&
			!isCharging &&
			!lowBatteryPulseShownRef.current
		) {
			setShowLowBatteryPulse(true);
			if (notchMode === "peek") triggerEventPeek(5000);
			lowBatteryPulseShownRef.current = true;
			setTimeout(() => setShowLowBatteryPulse(false), 5000);
		}

		// Reset the "shown" state if battery is charged or threshold is lowered
		if (isCharging || batteryLevel > lowBatteryThreshold) {
			lowBatteryPulseShownRef.current = false;
		}
	}, [batteryLevel, isCharging, lowBatteryThreshold, isReady, notchMode, triggerEventPeek]);

	// Media state
	const [isPlaying, setIsPlaying] = useState(false);
	const [mediaInfo, setMediaInfo] = useState<MediaInfo>({
		title: "",
		artist: "",
		is_playing: false,
		has_media: false
	});
	// The expanded panel shows a stack only when two or more players are
	// playing; the pill keeps using the single `media-update` card.
	const { cards: mediaCards, other_audio: otherAudio } = useMediaList();
	const [mediaStackHeight, setMediaStackHeight] = useState(0);
	// Stack mode owns its own glow source: the focus card's art and where its
	// block ends, so the shelf sits on calm black instead of the primary's art.
	const [stackFocusArt, setStackFocusArt] = useState<string | null>(null);
	const [stackFocusHeight, setStackFocusHeight] = useState(0);
	const [albumArtUrl, setAlbumArtUrl] = useState<string | null>(null);
	const [albumArtKey, setAlbumArtKey] = useState(0);
	const [volume, setVolume] = useState(0.5);
	const [isMuted, setIsMuted] = useState(false);
	// Sliders show 0 while muted, matching the overlay HUD.
	const displayVolume = isMuted ? 0 : volume;
	// The media card's own app volume; a null pid means no live session and the
	// media sliders keep the system-volume fallback.
	const [mediaVolume, setMediaVolume] = useState(0.5);
	const [mediaMuted, setMediaMuted] = useState(false);
	const [mediaSessionPid, setMediaSessionPid] = useState<number | null>(null);

	// System metrics for status widgets
	const [cpuUsage, setCpuUsage] = useState(0);
	const [diskSpace, setDiskSpace] = useState(0);
	const [netUpSpeed, setNetUpSpeed] = useState(0);
	const [netDownSpeed, setNetDownSpeed] = useState(0);
	const [statusWidgets, setStatusWidgets] = useState<WidgetConfig>({
		left: ["cpu"],
		right: ["net"]
	});

	const [windowLabel, setWindowLabel] = useState<string>("");
	useEffect(() => {
		setWindowLabel(getCurrentWebviewWindow().label);
	}, []);

	// Update state
	const [updateAvailable, setUpdateAvailable] = useState(false);
	const [showUpdateIndicator, setShowUpdateIndicator] = useState(
		() => localStorage.getItem("bloom-show-update-indicator") !== "false"
	);
	const [showUpdatePulse, setShowUpdatePulse] = useState(false);

	useEffect(() => {
		if (windowLabel !== "main") return;

		let unlisten: (() => void) | undefined;
		let disposed = false;

		listen<UpdateCheckResult>("update-available", (event) => {
			setUpdateAvailable(event.payload.available);
			if (!event.payload.available) {
				setShowUpdatePulse(false);
				if (updatePulseTimerRef.current) clearTimeout(updatePulseTimerRef.current);
				return;
			}
			setShowUpdatePulse(true);
			if (notchMode === "peek") triggerEventPeek(6000);
			if (updatePulseTimerRef.current) clearTimeout(updatePulseTimerRef.current);
			updatePulseTimerRef.current = setTimeout(() => {
				setShowUpdatePulse(false);
			}, 6000);
		}).then((fn) => {
			if (disposed) fn();
			else unlisten = fn;
		});

		invoke<UpdateCheckResult>("get_update_state")
			.then((state) => {
				// Only restore a badge; clearing is left to check results so a
				// late mount read cannot race an "update-available" event.
				if (state.available) setUpdateAvailable(true);
			})
			.catch((e) => console.error("Failed to read update state:", e));

		return () => {
			disposed = true;
			unlisten?.();
			if (updatePulseTimerRef.current) clearTimeout(updatePulseTimerRef.current);
		};
	}, [windowLabel, notchMode, triggerEventPeek]);

	const [screenFree, setScreenFree] = useState(true);
	const [isImpacted, setIsImpacted] = useState(false);
	const [isExpanded, setIsExpanded] = useState(false);
	const [startupAnimating, setStartupAnimating] = useState(false);

	const [isNotchHovered, setIsNotchHovered] = useState(false);

	const [isEdgeHovered, setIsEdgeHovered] = useState(false);
	const [isOverlapped, setIsOverlapped] = useState(false);
	const [interactionState, setInteractionState] = useState<"active" | "grace" | "none">("none");
	// Fullscreen retracts the pill in every notch mode; the top-centre dwell
	// (notch-edge-hover) brings it back until the hover grace ends.
	const isVisible = screenFree || interactionState !== "none";
	const bloomRef = useRef<HTMLDivElement>(null);
	const finderStripRef = useRef<HTMLDivElement>(null);
	const pillClusterRef = useRef<HTMLDivElement>(null);
	const timeCenterRef = useRef<HTMLDivElement>(null);
	const musicBubbleRef = useRef<HTMLButtonElement>(null);
	const shelfBubbleRef = useRef<HTMLButtonElement>(null);
	const reportNotchRect = useRef<() => void>(() => {});
	// The pill's target box in window CSS px, and the sender that applies the
	// notch window's region (holding earlier regions while it shrinks).
	const pillTargetRef = useRef<RegionRect | null>(null);
	const barConstraint = useRef<{ strip: RegionRect; boundary: number } | null>(null);
	const sendNotchRegion = useMemo(
		() =>
			createRegionSender((rects) => {
				const fit = barConstraint.current;
				invoke("set_window_region", {
					rects: fit ? constrainBarRegions(rects, fit.strip, fit.boundary) : rects
				}).catch(() => {});
			}),
		[]
	);
	const sendFittedRegion = useMemo(() => createRegionSender((rects) => {
		const fit = barConstraint.current;
		invoke("set_window_region", { rects: fit ? constrainBarRegions(rects, fit.strip, fit.boundary) : rects }).catch(() => {});
	}, 0), []);

	// Recomputes the notch window's region from the pill's target box and the
	// floating parts drawn around it, and sends it if it changed. Reads only
	// refs and the DOM, so it is safe to call from any callback.
	const updateNotchRegion = () => {
		const wanted: RegionRect[] = [];
		if (barConstraint.current) {
			// Follow the painted animation frame directly: holding the old
			// panel would leave an invisible input blocker after closing.
			const panel = bloomRef.current;
			if (panel && Number(getComputedStyle(panel).opacity) > 0.01) {
				const r = panel.getBoundingClientRect();
				if (r.height > 1) wanted.push({ x: r.left, y: r.top, width: r.width, height: r.height }); // unpadded: an unpainted region pixel shows as a pale haze
			}
		} else if (pillTargetRef.current) wanted.push(pillTargetRef.current);
		if (finderStripRef.current) {
			const row = finderStripRef.current.querySelector(".main-row") ?? finderStripRef.current;
			const r = row.getBoundingClientRect();
			wanted.push({ x: r.left, y: r.top, width: r.width, height: r.height });
		}
		// Each floating part, padded to cover what it paints outside its box: a
		// tooltip card's shadow reaches 20px out plus a 6px drop, a bubble's
		// glow less, and the screen corners nothing.
		wanted.push(
			...measureElements([
				[".activity-bubble", 16],
				[".status-tooltip", 28],
				[".screen-corner", 0]
			])
		);
		if (barConstraint.current) sendFittedRegion(wanted);
		else sendNotchRegion(wanted);
	};
	const [systemSettingsReady, setSystemSettingsReady] = useState(false);
	const [waitForStartupSplash, setWaitForStartupSplash] = useState(false);
	const initialSystemSyncDone = useRef(false);
	const splashDone = useRef(false);
	const systemSyncQueue = useRef<Promise<void>>(Promise.resolve());
	const appliedNotchMode = useRef<string | null>(null);

	const [quotaPinned, setQuotaPinned] = useState(
		() => localStorage.getItem("bloom-quota-pinned") === "true"
	);
	const [quotaCompact, setQuotaCompact] = useState(
		() => localStorage.getItem("bloom-quota-compact") === "true"
	);

	const isAnyInteraction = isHovered || isNotchHovered || isEdgeHovered;
	const isHidden =
		myfinderIdleHidden(myfinder, isOpen || quotaPinned, isNotchHovered) || (!startupAnimating && !integrated &&
		((notchMode === "smart" && isOverlapped && interactionState === "none") ||
			(notchMode === "peek" && interactionState === "none" && !eventPeek && !quotaAlertHold && !quotaPinned)));

	useEffect(() => {
		if (isAnyInteraction) {
			setInteractionState("active");
		} else if (interactionState !== "none") {
			setInteractionState("grace");
			const timer = setTimeout(() => setInteractionState("none"), 800);
			return () => clearTimeout(timer);
		}
	}, [isAnyInteraction]);

	useEffect(() => {
		if (windowLabel === "main") {
			invoke("set_notch_hovered", { hovered: isNotchHovered }).catch(() => {});
		}
	}, [isNotchHovered, windowLabel]);

	// Tell the backend whether the notch is actually on screen so a hidden
	// notch (smart/peek mode) stays click-through instead of swallowing clicks
	// in its footprint.
	useEffect(() => {
		if (windowLabel === "main") {
			invoke("set_notch_visible", { visible: isVisible && !isHidden }).catch(() => {});
		}
	}, [isVisible, isHidden, windowLabel]);

	useEffect(() => {
		const updateRect = () => {
			if (bloomRef.current && windowLabel === "main") {
				// Activity bubbles sit beside the pill and must stay clickable too.
				const rects = [bloomRef.current, finderStripRef.current, musicBubbleRef.current, shelfBubbleRef.current]
					.filter((el): el is NonNullable<typeof el> => el !== null)
					.map((el) => el.getBoundingClientRect());
				const left = Math.min(...rects.map((r) => r.left));
				const top = Math.min(...rects.map((r) => r.top));
				const right = Math.max(...rects.map((r) => r.right));
				const bottom = Math.max(...rects.map((r) => r.bottom));
				invoke("update_notch_rect", {
					rect: {
						x: Math.round(left),
						y: Math.round(top),
						width: Math.round(right - left),
						height: Math.round(bottom - top)
					}
				}).catch(() => {});
				// Bubbles finishing their entrance land here too.
				updateNotchRegion();
			}
		};
		reportNotchRect.current = updateRect;

		updateRect();
		window.addEventListener("resize", updateRect);
		const observer = new ResizeObserver(updateRect);
		if (bloomRef.current) observer.observe(bloomRef.current);
		if (finderStripRef.current) observer.observe(finderStripRef.current);

		return () => {
			window.removeEventListener("resize", updateRect);
			observer.disconnect();
		};
	}, [isExpanded, isHidden, windowLabel, scale, integrated]);

	useEffect(() => {
		if (!windowLabel) return;

		// Only animate the main top-bar
		if (windowLabel !== "main") {
			setIsReady(true);
			setIsImpacted(true);
			setIsExpanded(true);
			return;
		}

		const proceedWithStartup = () => {
			const checkVisibility = async () => {
				try {
					const win = getCurrentWebviewWindow();
					const visible = await win.isVisible();
					if (visible) {
						setStartupAnimating(true);
						setIsReady(true);
						// The window is shown here; WebView2 may only create its
						// render child now, so refresh the drop targets.
						invoke("setup_shelf_drop").catch(() => {});
						setTimeout(() => {
							setIsImpacted(true);
							setIsExpanded(true);
						}, 240);
						setTimeout(() => setStartupAnimating(false), 1500);
						return true;
					}
				} catch (e) {}
				return false;
			};

			const interval = setInterval(async () => {
				if (await checkVisibility()) clearInterval(interval);
			}, 100);

			checkVisibility();
			return interval;
		};

		// Mirror Overlay.tsx's splash decision so we only wait when a splash will actually fire.
		// Overlay always emits splash-done, but on a normal relaunch it emits it near-instantly
		// (after one async getVersion() call) — before this listener would be registered.
		// By making the same decision here we avoid a race and avoid any unnecessary delay.
		const firstRun = localStorage.getItem("bloom-first-run") === null;
		const storedVersion = localStorage.getItem("bloom-app-version");

		const waitForSplash = () => {
			// Splash is definitely coming — register listener now (2800ms animation gives us plenty of time)
			let started = false;
			let interval: any;
			const unlistenSplash = listen("splash-done", () => {
				if (started) return;
				started = true;
				interval = proceedWithStartup();
				unlistenSplash.then((fn) => fn());
			});
			const safetyTimer = setTimeout(() => {
				if (started) return;
				started = true;
				interval = proceedWithStartup();
				unlistenSplash.then((fn) => fn());
			}, 6000);
			return () => {
				clearTimeout(safetyTimer);
				if (interval) clearInterval(interval);
				unlistenSplash.then((fn) => fn());
			};
		};

		if (firstRun || storedVersion === null) {
			// Splash is definitely showing — wait for it
			return waitForSplash();
		}

		// Has a version key — need async check to know if version changed
		let interval: any;
		getVersion()
			.then((currentVersion) => {
				if (storedVersion !== currentVersion) {
					// Version changed — splash is coming, wait for it
					// (splash takes 2800ms so there's plenty of time to register the listener)
					waitForSplash();
				} else {
					// Same version — no splash, start immediately
					interval = proceedWithStartup();
				}
			})
			.catch(() => {
				interval = proceedWithStartup();
			});
		return () => {
			if (interval) clearInterval(interval);
		};
	}, [windowLabel]);

	// Settings state
	const [settingsTimerSoundEnabled, setSettingsTimerSoundEnabled] = useState(
		() => localStorage.getItem("bloom-timer-sound-enabled") !== "false"
	);
	const [settingsMusicModeEnabled, setSettingsMusicModeEnabled] = useState(
		() => localStorage.getItem("bloom-music-mode-enabled") !== "false"
	);
	const [rememberPanelEnabled, setRememberPanelEnabled] = useState(
		() =>
			(localStorage.getItem("bloom-notch-remember-panel") ??
				localStorage.getItem("bloom-notch-click-to-open")) !== "false"
	);
	const rememberPanelEnabledRef = useRef(rememberPanelEnabled);
	rememberPanelEnabledRef.current = rememberPanelEnabled;
	const [defaultPanel, setDefaultPanel] = useState<NotchPanel>(
		() => asNotchPanel(localStorage.getItem("bloom-notch-default-panel")) ?? DEFAULT_PANEL
	);
	// The panel the open notch was last left on. Opening the notch returns to it,
	// across restarts too (bloom-notch-last-panel).
	const [lastPanel, setLastPanel] = useState<NotchPanel | null>(() =>
		asNotchPanel(localStorage.getItem("bloom-notch-last-panel"))
	);
	const [settingsMusicCompactNotch, setSettingsMusicCompactNotch] = useState(
		() => localStorage.getItem("bloom-music-compact-notch") !== "false"
	);
	const [shelfPersist, setShelfPersist] = useState(
		() => localStorage.getItem("bloom-shelf-persist") === "true"
	);
	const [settingsVisualizerEnabled, setSettingsVisualizerEnabled] = useState(
		() => localStorage.getItem("bloom-visualizer-enabled") !== "false"
	);
	const [settingsAlbumArtEnabled, setSettingsAlbumArtEnabled] = useState(
		() => localStorage.getItem("bloom-media-album-art-enabled") !== "false"
	);
	const [settingsAmbienceEnabled, setSettingsAmbienceEnabled] = useState(
		() => localStorage.getItem("bloom-media-ambience-enabled") !== "false"
	);
	const [settingsCompactGlowEnabled, setSettingsCompactGlowEnabled] = useState(
		() => localStorage.getItem("bloom-media-compact-glow-enabled") !== "false"
	);
	const [settingsCornersEnabled, setSettingsCornersEnabled] = useState(
		() => localStorage.getItem("bloom-corners-enabled") === "true"
	);
	const [mediaLayout, setMediaLayout] = useState<"classic" | "compact">(
		() => (localStorage.getItem("bloom-media-layout") as "classic" | "compact") || "classic"
	);
	const [compactVolumeExpanded, setCompactVolumeExpanded] = useState(false);
	const [compactQueueExpanded, setCompactQueueExpanded] = useState(false);
	// The drawer reports the island height it needs, measured from the rendered player
	// above it; the island never grows more than QUEUE_MAX_HEIGHT for it.
	const [queueIslandHeight, setQueueIslandHeight] = useState(Number.POSITIVE_INFINITY);
	// A shrinking island must not slide out from under the pointer: that ends the hover
	// and closes the notch mid-task (choosing the last item empties the list). The shrink
	// waits, holding the island just below the pointer, until the pointer moves above the
	// new bottom edge. Pointer y is in the island's layout px; -1 when outside.
	const [queueFloor, setQueueFloor] = useState(0);
	const queuePointerY = useRef(-1);
	const queueIslandHeightRef = useRef(Number.POSITIVE_INFINITY);
	const reportQueueIslandHeight = useCallback((next: number) => {
		const pointer = queuePointerY.current;
		const shown = queueIslandHeightRef.current;
		if (Number.isFinite(shown) && next < shown && pointer > next - 8) setQueueFloor(pointer + 24);
		queueIslandHeightRef.current = next;
		setQueueIslandHeight(next);
	}, []);
	const trackQueuePointer = (e: React.MouseEvent<HTMLElement>) => {
		const box = e.currentTarget.getBoundingClientRect();
		const y = ((e.clientY - box.top) * e.currentTarget.offsetHeight) / (box.height || 1);
		queuePointerY.current = y;
		if (queueFloor && y < queueIslandHeightRef.current - 8) setQueueFloor(0);
	};
	const [mediaAvailableHeight, setMediaAvailableHeight] = useState(420);

	useEffect(() => {
		if (!windowLabel) return;
		let cancelled = false;

		invoke("load_settings")
			.then(async (settings: any) => {
				if (await reloadIfMirrorWasStale(settings)) return;
				if (cancelled) return;
				const getVal = (key: string, fallback: string | null = null) => {
					const val = settings[key];
					if (val !== undefined && val !== null) return String(val);
					const local = localStorage.getItem(key);
					if (local !== null) return local;
					return fallback;
				};

				setSettingsTimerSoundEnabled(getVal("bloom-timer-sound-enabled", "true") !== "false");
				setSettingsMusicModeEnabled(getVal("bloom-music-mode-enabled", "true") !== "false");
				setRememberPanelEnabled(
					(getVal("bloom-notch-remember-panel") ?? getVal("bloom-notch-click-to-open", "true")) !==
						"false"
				);
				setDefaultPanel(asNotchPanel(getVal("bloom-notch-default-panel")) ?? DEFAULT_PANEL);
				setShelfPersist(getVal("bloom-shelf-persist", "false") === "true");
				setSettingsMusicCompactNotch(getVal("bloom-music-compact-notch", "true") !== "false");
				setLastPanel(asNotchPanel(getVal("bloom-notch-last-panel")));
				setQuotaHiddenAgents(parseHiddenAgents(getVal(QUOTA_HIDDEN_AGENTS_KEY)));
				const savedQuotaCompact = getVal("bloom-quota-compact", "false") === "true";
				setQuotaCompact(savedQuotaCompact);
				const savedQuotaPinned = getVal("bloom-quota-pinned", "false") === "true";
				localStorage.setItem("bloom-quota-pinned", String(savedQuotaPinned));
				setQuotaPinned(savedQuotaPinned);
				if (savedQuotaPinned) {
					setIsOpen(true);
					setBloomMode("quotas");
				}
				const viz =
					getVal("bloom-media-visualizer-enabled") ?? getVal("bloom-visualizer-enabled", "true");
				setSettingsVisualizerEnabled(viz !== "false");
				setSettingsAlbumArtEnabled(getVal("bloom-media-album-art-enabled", "true") !== "false");
				setSettingsAmbienceEnabled(getVal("bloom-media-ambience-enabled", "true") !== "false");
				setSettingsCompactGlowEnabled(
					getVal("bloom-media-compact-glow-enabled", "true") !== "false"
				);
				setSettingsCornersEnabled(getVal("bloom-corners-enabled", "false") === "true");
				setTimeFormat24h(getVal("bloom-time-format-24h") === "true");

				const thresholdStr = getVal("bloom-low-battery-threshold", "20");
				if (thresholdStr) setLowBatteryThreshold(parseInt(thresholdStr as string));

				const nMode = getVal("bloom-notch-mode", "fixed");
				if (nMode) {
					const mapped = nMode === "auto-hide" ? "smart" : nMode;
					setNotchMode(mapped);
				}

				if (windowLabel === "main") {
					const firstRun = localStorage.getItem("bloom-first-run") === null;
					if (firstRun) {
						// The dev origin starts with empty localStorage; enabling here
						// would point the login Run key at target/debug/bloom.exe.
						if (!import.meta.env.DEV) {
							import("@tauri-apps/plugin-autostart").then(({ enable, isEnabled }) => {
								isEnabled().then((enabled) => {
									if (!enabled) enable().catch(() => {});
								});
							});
						}
						localStorage.setItem("bloom-first-run", "done");
					}
					const storedVersion = localStorage.getItem("bloom-app-version");
					let waitForSplash = firstRun || storedVersion === null;
					if (!waitForSplash) {
						try {
							waitForSplash = storedVersion !== (await getVersion());
						} catch {
							// Version lookup is optional; continue startup without a splash.
						}
					}
					if (cancelled) return;
					setWaitForStartupSplash(waitForSplash);
					setSystemSettingsReady(true);
				}

				const scaleVal = getVal("bloom-scale");
				if (scaleVal !== null) setScale(parseFloat(scaleVal));

				const widgetsVal = getVal("bloom-status-widgets");
				if (widgetsVal) {
					try {
						const parsed = JSON.parse(widgetsVal);
						if (parsed && Array.isArray(parsed.left) && Array.isArray(parsed.right)) {
							setStatusWidgets(parsed);
						}
					} catch {}
				}
			})
			.catch((error) => {
				console.error(error);
				if (!cancelled && windowLabel === "main") setSystemSettingsReady(true);
			});
		return () => {
			cancelled = true;
		};
	}, [windowLabel]); // eslint-disable-line react-hooks/exhaustive-deps

	useEffect(() => {
		// Disable context menu globally
		const preventContext = (e: MouseEvent) => e.preventDefault();
		document.addEventListener("contextmenu", preventContext);

		const unlistenVisibility = listen<boolean>("visibility-change", (event) => {
			setScreenFree(event.payload);
		});

		const unlistenNotchOverlap = listen<boolean>("notch-overlap", (event) => {
			setIsOverlapped(event.payload);
		});

		const unlistenNotchEdgeHover = listen<boolean>("notch-edge-hover", (event) => {
			setIsEdgeHovered(event.payload);
		});

		// The backend emits this when the window flips to click-through while
		// the webview still thinks the pointer is inside the notch (it cannot
		// receive the mouseleave once the window is click-through).
		const unlistenNotchPointerLeft = listen("notch-pointer-left", () => {
			releaseNotchHover();
		});

		return () => {
			unlistenVisibility.then((f) => f());
			unlistenNotchOverlap.then((f) => f());
			unlistenNotchEdgeHover.then((f) => f());
			unlistenNotchPointerLeft.then((f) => f());
			document.removeEventListener("contextmenu", preventContext);
		};
	}, [windowLabel]);

	// Settings sync — consolidated dispatch for all settings events
	useSettingsSync(
		{
			"bloom-timer-sound-enabled": setSettingsTimerSoundEnabled,
			"bloom-music-mode-enabled": setSettingsMusicModeEnabled,
			"bloom-notch-remember-panel": setRememberPanelEnabled,
			"bloom-notch-default-panel": (v: unknown) => setDefaultPanel(asNotchPanel(v) ?? "timer"),
			"bloom-shelf-persist": setShelfPersist,
			"bloom-music-compact-notch": setSettingsMusicCompactNotch,
			"bloom-notch-last-panel": (v: unknown) => setLastPanel(asNotchPanel(v)),
			"bloom-media-visualizer-enabled": setSettingsVisualizerEnabled,
			"bloom-visualizer-enabled": setSettingsVisualizerEnabled,
			"bloom-media-album-art-enabled": setSettingsAlbumArtEnabled,
			"bloom-media-ambience-enabled": setSettingsAmbienceEnabled,
			"bloom-media-compact-glow-enabled": setSettingsCompactGlowEnabled,
			"bloom-media-layout": setMediaLayout,
			"bloom-corners-enabled": setSettingsCornersEnabled,
			"bloom-scale": setScale,
			"bloom-low-battery-threshold": setLowBatteryThreshold,
			"bloom-notch-mode": setNotchMode,
			[QUOTA_HIDDEN_AGENTS_KEY]: (value) => setQuotaHiddenAgents(parseHiddenAgents(value)),
			"bloom-quota-compact": (value) => setQuotaCompact(String(value) === "true"),
			"bloom-quota-pinned": (value) => {
				const pinned = String(value) === "true";
				localStorage.setItem("bloom-quota-pinned", String(pinned));
				setQuotaPinned(pinned);
				if (pinned) {
					setIsOpen(true);
					setBloomMode("quotas");
				}
			},
			"bloom-status-widgets": (value) => {
				try {
					const parsed = JSON.parse(value);
					if (parsed && Array.isArray(parsed.left) && Array.isArray(parsed.right)) {
						setStatusWidgets(parsed);
					}
				} catch {}
			},
			"bloom-show-update-indicator": (value) => setShowUpdateIndicator(String(value) === "true"),
			"bloom-time-format-24h": setTimeFormat24h
		},
        [windowLabel],
        (key) => { if (key === QUOTA_HIDDEN_AGENTS_KEY) setQuotaHiddenAgents([]); }
    );

	// One owner applies the latest window settings. Superseded splash timers and
	// retries are cancelled; an in-flight command finishes before the new state
	// is applied, so an old startup retry cannot overwrite a user change.
	useEffect(() => {
		if (windowLabel !== "main" || !systemSettingsReady) return;

		let cancelled = false;
		let started = false;
		let attempts = 0;
		let startTimer: number | undefined;
		let splashTimer: number | undefined;
		let retryTimer: number | undefined;
		let settleTimer: number | undefined;
		let unlistenSplash: (() => void) | undefined;

		const reconcile = () => {
			systemSyncQueue.current = systemSyncQueue.current
				.catch(() => {})
				.then(async () => {
					if (cancelled) return;
					try {
						if (appliedNotchMode.current !== notchMode) {
							await invoke("change_notch_mode", { mode: notchMode });
							appliedNotchMode.current = notchMode;
						}
						if (cancelled) return;
						await invoke("sync_appbar");
						if (cancelled) return;
						const wasInitialSync = !initialSystemSyncDone.current;
						initialSystemSyncDone.current = true;
						if (wasInitialSync) {
							settleTimer = window.setTimeout(() => {
								if (!cancelled) {
									systemSyncQueue.current = systemSyncQueue.current
										.catch(() => {})
										.then(() => invoke<void>("sync_appbar"))
										.catch((error) => console.error("AppBar settle failed", error));
								}
							}, 1000);
						}
					} catch (error) {
						console.error("Window reconciliation failed", error);
						if (!cancelled && attempts++ < 1) {
							retryTimer = window.setTimeout(reconcile, 1000);
						}
					}
				});
		};

		const start = () => {
			if (cancelled || started) return;
			started = true;
			splashDone.current = true;
			if (splashTimer !== undefined) window.clearTimeout(splashTimer);
			unlistenSplash?.();
			startTimer = window.setTimeout(reconcile, initialSystemSyncDone.current ? 0 : 400);
		};

		if (waitForStartupSplash && !splashDone.current && !initialSystemSyncDone.current) {
			void listen("splash-done", start)
				.then((unlisten) => {
					if (cancelled || started) unlisten();
					else unlistenSplash = unlisten;
				})
				.catch(() => start());
			splashTimer = window.setTimeout(start, 6000);
		} else {
			start();
		}

		return () => {
			cancelled = true;
			for (const timer of [startTimer, splashTimer, retryTimer, settleTimer]) {
				if (timer !== undefined) window.clearTimeout(timer);
			}
			unlistenSplash?.();
		};
	}, [windowLabel, systemSettingsReady, waitForStartupSplash, notchMode]);

	// Bloom mode state: 'music', 'calendar', 'command-center', 'shelf', or 'status'
	const [bloomMode, setBloomMode] = useState<BloomMode>(() =>
		localStorage.getItem("bloom-quota-pinned") === "true" ? "quotas" : "status"
	);

	// Shelf: files and selected text stashed in the notch via drag-and-drop.
	// Files are stored as plain paths (references, never copies). Session-scoped
	// by default; the bloom-shelf-persist setting keeps items across restarts.
	const [shelfItems, setShelfItems] = useState<ShelfItem[]>(() => {
		if (localStorage.getItem("bloom-shelf-persist") !== "true") return [];
		try {
			const saved = localStorage.getItem("bloom-shelf-items");
			const parsed = saved ? JSON.parse(saved) : [];
			if (!Array.isArray(parsed)) return [];
			return parsed
				.filter((i): i is ShelfItem => {
					if (typeof i?.id !== "string" || typeof i?.name !== "string") return false;
					// Items stored before text support have no `type`; treat as files.
					if (i.type === "text") return typeof i.text === "string";
					return typeof i.path === "string";
				})
				.map((i) => ({ ...i, type: i.type === "text" ? "text" : "file" }));
		} catch {
			return [];
		}
	});
	useEffect(() => {
		try {
			if (shelfPersist) {
				localStorage.setItem("bloom-shelf-items", JSON.stringify(shelfItems));
			} else {
				localStorage.removeItem("bloom-shelf-items");
			}
		} catch {
			/* storage full or unavailable — shelf just won't persist */
		}
	}, [shelfItems, shelfPersist]);

	// Drop file references whose files are gone (moved/deleted/drive unplugged)
	// once at startup, so a persisted shelf does not accumulate dead entries.
	// Text items are always kept.
	const shelfPrunedRef = useRef(false);
	useEffect(() => {
		if (shelfPrunedRef.current) return;
		shelfPrunedRef.current = true;
		const filePaths = shelfItems
			.filter((i) => i.type === "file" && i.path)
			.map((i) => i.path as string);
		if (filePaths.length === 0) return;
		invoke<string[]>("shelf_prune", { paths: filePaths })
			.then((existing) => {
				const alive = new Set(existing);
				setShelfItems((prev) => {
					const kept = prev.filter(
						(i) => i.type !== "file" || (i.path !== undefined && alive.has(i.path))
					);
					return kept.length === prev.length ? prev : kept;
				});
			})
			.catch(() => {});
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

	// Previews are generated lazily per path and cached. Images, PDFs, Office
	// files with a saved thumbnail, and anything Windows can thumbnail (videos,
	// folders...) show a picture; documents and text files show their opening
	// text; the rest fall back to an icon for their type. Text items render
	// their content.
	const [shelfThumbs, setShelfThumbs] = useState<Record<string, ShelfPreview>>({});
	const shelfThumbPending = useRef<Set<string>>(new Set());
	useEffect(() => {
		for (const item of shelfItems) {
			if (item.type !== "file" || !item.path) continue;
			const path = item.path;
			if (shelfThumbs[path] !== undefined || shelfThumbPending.current.has(path)) continue;
			shelfThumbPending.current.add(path);
			// The item may be removed while this is in flight; don't resurrect
			// a stale cache entry.
			const storeThumb = (preview: ShelfPreview) =>
				setShelfThumbs((prev) => {
					if (!shelfItemsRef.current.some((i) => i.path === path)) return prev;
					return { ...prev, [path]: preview };
				});
			invoke<ShelfPreview>("shelf_thumbnail", { path })
				.then(storeThumb)
				.catch(() => storeThumb({ thumb: null, text: null, folder: false }))
				.finally(() => shelfThumbPending.current.delete(path));
		}
	}, [shelfItems, shelfThumbs]);

	// Drop cached thumbnails for items that left the shelf so the cache cannot
	// grow without bound over a long session (each entry is a data URI).
	useEffect(() => {
		const current = new Set(shelfItems.filter((i) => i.path).map((i) => i.path as string));
		setShelfThumbs((prev) => {
			const keys = Object.keys(prev);
			if (keys.every((k) => current.has(k))) return prev;
			const next: Record<string, ShelfPreview> = {};
			for (const k of keys) {
				if (current.has(k)) next[k] = prev[k];
			}
			return next;
		});
	}, [shelfItems]);

	// Native drag preview for items without a thumbnail.
	const [shelfDragIcon, setShelfDragIcon] = useState<string | null>(null);
	useEffect(() => {
		invoke<string | null>("shelf_drag_icon")
			.then(setShelfDragIcon)
			.catch(() => {});
	}, []);

	// Picking a file back up is a drag-out gesture: press, move past a small
	// threshold, and the OS takes over the drag. A press without movement is a
	// plain click that opens the file (or copies text items).
	const shelfPointerRef = useRef<{ id: string; x: number; y: number; dragging: boolean } | null>(
		null
	);
	// Timestamp of the last drop onto the notch. A drop recorded during our
	// own drag-out means the item came back to the notch (pointer still over
	// it), which decides whether the drag cleanup clears the hover flags.
	const shelfSelfDropAtRef = useRef(0);
	// True while our own drag-out is running, so the drag-enter we receive when
	// it starts over the notch does not flash the hover outline or re-open it.
	const shelfSelfDragRef = useRef(false);
	// State mirror of shelfSelfDragRef for the attention controller's drag
	// gesture; the ref above serves drag listeners, this drives effects.
	const [shelfDragOut, setShelfDragOut] = useState(false);
	const handleShelfPointerDown = (e: React.PointerEvent<HTMLDivElement>, item: ShelfItem) => {
		if (e.button !== 0) return;
		// Text items have no native drag-out; a click copies them.
		if (item.type !== "file" || !item.path) return;
		// The remove button must keep receiving its own click; capturing the
		// pointer on the tile would retarget it.
		if ((e.target as HTMLElement).closest(".shelf-tile-remove")) return;
		e.stopPropagation();
		e.currentTarget.setPointerCapture(e.pointerId);
		shelfPointerRef.current = { id: item.id, x: e.clientX, y: e.clientY, dragging: false };
	};
	const handleShelfPointerMove = (e: React.PointerEvent<HTMLDivElement>, item: ShelfItem) => {
		const st = shelfPointerRef.current;
		if (!st || st.id !== item.id || st.dragging) return;
		if (Math.hypot(e.clientX - st.x, e.clientY - st.y) < 6) return;
		if (!item.path) return;
		const path = item.path;
		const icon = shelfThumbs[path]?.thumb ?? shelfDragIcon;
		if (!icon) return;
		st.dragging = true;
		// Picking an item up is a deliberate act; don't let the post-drop
		// auto-close fire mid-drag.
		clearShelfCollapse();
		shelfSelfDragRef.current = true;
		setShelfDragOut(true);
		const dragStartedAt = Date.now();
		startDrag({ item: [path], icon })
			.catch((err) => console.error("[shelf] drag out failed:", path, err))
			.finally(() => {
				shelfSelfDragRef.current = false;
				setShelfDragOut(false);
				// A drop recorded during this drag means it landed back on the
				// notch, so the pointer is still over it. Otherwise the native
				// drag owned the pointer and the webview never saw it leave —
				// release the hover state (flags AND the resting mode) or the
				// shelf panel stays expanded. The command also makes the
				// backend re-evaluate its edge-hover state immediately.
				const droppedOnSelf = shelfSelfDropAtRef.current >= dragStartedAt;
				if (!droppedOnSelf) {
					releaseNotchHover();
					invoke("set_notch_hovered", { hovered: false }).catch(() => {});
				}
			});
	};
	const handleShelfTileClick = (e: React.MouseEvent<HTMLDivElement>, item: ShelfItem) => {
		e.stopPropagation();
		const st = shelfPointerRef.current;
		shelfPointerRef.current = null;
		// Only this tile's own drag suppresses the click; a stale flag from a
		// previous tile's drag-out must not swallow it.
		if (st && st.id === item.id && st.dragging) return;
		openShelfItem(item);
	};

	// Wheel scrolling for the shelf strip: ease towards a target instead of
	// jumping, so wheel and trackpad input both feel fluid.
	const shelfScrollTargetRef = useRef<number | null>(null);
	const shelfGridRef = useRef<HTMLDivElement>(null);
	// Whether the shelf row overflows, so it gets room for its scrollbar.
	const [shelfOverflowing, setShelfOverflowing] = useState(false);
	// Measured against the open shelf's width (380px minus its 14px side
	// padding) rather than the live one, which is still growing while the
	// notch animates open. A removed tile stays in the row until its exit
	// animation ends, so the row is measured again then.
	const measureShelfOverflow = useCallback(() => {
		const el = shelfGridRef.current;
		setShelfOverflowing(!!el && el.scrollWidth > 380 - 28 + 1);
	}, []);
	useLayoutEffect(measureShelfOverflow, [shelfItems, bloomMode, measureShelfOverflow]);
	const shelfScrollRafRef = useRef<number | null>(null);
	// The wheel scrolls the row from anywhere over the shelf view, not just
	// over the tiles. Mice that report lines instead of pixels are scaled up.
	const handleShelfWheel = (e: React.WheelEvent<HTMLDivElement>) => {
		const el = shelfGridRef.current;
		if (!el) return;
		const max = el.scrollWidth - el.clientWidth;
		if (max <= 0) return;
		const unit = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? el.clientWidth : 1;
		const base = shelfScrollTargetRef.current ?? el.scrollLeft;
		shelfScrollTargetRef.current = Math.max(0, Math.min(max, base + (e.deltaY + e.deltaX) * unit));
		if (shelfScrollRafRef.current !== null) return;
		const step = () => {
			const target = shelfScrollTargetRef.current;
			if (target === null) {
				shelfScrollRafRef.current = null;
				return;
			}
			const diff = target - el.scrollLeft;
			if (Math.abs(diff) < 0.5) {
				el.scrollLeft = target;
				shelfScrollTargetRef.current = null;
				shelfScrollRafRef.current = null;
				return;
			}
			const before = el.scrollLeft;
			el.scrollLeft += diff * 0.22;
			// At fractional display scales a small step can round back to the
			// same position; finish the move instead of stalling short of it.
			if (el.scrollLeft === before) {
				el.scrollLeft = target;
				shelfScrollTargetRef.current = null;
				shelfScrollRafRef.current = null;
				return;
			}
			shelfScrollRafRef.current = requestAnimationFrame(step);
		};
		shelfScrollRafRef.current = requestAnimationFrame(step);
	};

	// The shelf closes itself shortly after a drop so the notch does not stay
	// expanded. Hovering it cancels the timer; leaving collapses it.
	const shelfCollapseTimer = useRef<number | null>(null);
	// What the collapsed notch shows (see collapsedMode). Refs let the drop
	// listener and the backend pointer-left event, both mounted once, act on
	// the current state.
	const restingModeRef = useRef<BloomMode>("status");
	const bloomModeRef = useRef<BloomMode>("status");
	const rememberPanelRef = useRef<(panel: NotchPanel) => void>(() => {});
	const notchPinnedRef = useRef(false);
	const releaseDeferredRef = useRef(false);
	const suppressHoverReleaseRef = useRef(false);
	const suppressHoverReleaseTimer = useRef<number | null>(null);
	// Inside MyFinder the 8px gap between the bar and its menu is outside the
	// native window region, so crossing it makes the webview report a leave and
	// the pointer re-enters a moment later. The release waits out that crossing.
	const finderLeaveTimer = useRef<number | null>(null);
	const cancelFinderLeave = () => {
		if (finderLeaveTimer.current === null) return;
		window.clearTimeout(finderLeaveTimer.current);
		finderLeaveTimer.current = null;
	};
	const releaseFinderHoverSoon = () => {
		cancelFinderLeave();
		finderLeaveTimer.current = window.setTimeout(() => {
			finderLeaveTimer.current = null;
			releaseNotchHover();
		}, 250);
	};

	const suppressNextHoverRelease = () => {
		suppressHoverReleaseRef.current = true;
		if (suppressHoverReleaseTimer.current !== null) {
			window.clearTimeout(suppressHoverReleaseTimer.current);
		}
		suppressHoverReleaseTimer.current = window.setTimeout(() => {
			suppressHoverReleaseTimer.current = null;
			suppressHoverReleaseRef.current = false;
		}, 600);
	};

	// Shared hover-release, called on DOM hover end and when the backend
	// reports the pointer left the (now click-through) notch window. Closes an
	// open notch, remembering its panel, and returns to the resting mode.
	const releaseNotchHover = () => {
		// A click inside the notch can shrink the island out from under a
		// stationary pointer (closing Up next, for one). That hover-end is not
		// the user leaving, so ignore the first one after a press.
		if (suppressHoverReleaseRef.current) {
			suppressHoverReleaseRef.current = false;
			return;
		}
		// A view being typed into (a Wi-Fi password) keeps the notch open; the
		// release happens once it lets go, if the pointer is still away.
		if (notchPinnedRef.current) {
			releaseDeferredRef.current = true;
			return;
		}
		setIsHovered(false);
		setIsNotchHovered(false);
		if (isOpenRef.current) {
			const panel = asNotchPanel(bloomModeRef.current);
			if (panel && rememberPanelEnabledRef.current) rememberPanelRef.current(panel);
			setIsOpen(false);
		}
		setBloomMode((prev) =>
			prev === "music" ||
			prev === "timer" ||
			prev === "status" ||
			prev === "shelf" ||
			prev === "quotas" ||
			prev === "overview"
				? restingModeRef.current
				: prev
		);
	};

	const clearShelfCollapse = () => {
		if (shelfCollapseTimer.current !== null) {
			window.clearTimeout(shelfCollapseTimer.current);
			shelfCollapseTimer.current = null;
		}
	};

	const collapseShelfSoon = (delay = 1800) => {
		clearShelfCollapse();
		shelfCollapseTimer.current = window.setTimeout(() => {
			shelfCollapseTimer.current = null;
			if (bloomModeRef.current !== "shelf") return;
			setIsOpen(false);
			setBloomMode(restingModeRef.current);
		}, delay);
	};

	const removeShelfItem = (id: string) => {
		const next = shelfItems.filter((i) => i.id !== id);
		setShelfItems(next);
		if (next.length === 0 && bloomMode === "shelf") {
			// Nothing left to show: an open notch moves to the remembered panel,
			// a closed one to its resting mode, instead of an empty shelf.
			clearShelfCollapse();
			setBloomMode(isOpen ? openingPanel() : restingModeRef.current);
		}
	};

	// A full shelf never evicts silently: whatever fits is added and the rest
	// is reported. Reads through a ref so the drop listener (mounted once) sees
	// the current list.
	const shelfItemsRef = useRef<ShelfItem[]>(shelfItems);
	shelfItemsRef.current = shelfItems;
	// Tiles that were just added get an entrance animation; ones restored when
	// the panel reopens do not (the panel itself already fades in).
	const shelfNewIdsRef = useRef<Set<string>>(new Set());
	const markShelfNew = (ids: string[]) => {
		for (const id of ids) shelfNewIdsRef.current.add(id);
		window.setTimeout(() => {
			for (const id of ids) shelfNewIdsRef.current.delete(id);
		}, 900);
	};
	const [shelfNotice, setShelfNotice] = useState<string | null>(null);
	const shelfNoticeTimer = useRef<number | null>(null);
	const showShelfNotice = (message: string) => {
		setShelfNotice(message);
		if (shelfNoticeTimer.current !== null) window.clearTimeout(shelfNoticeTimer.current);
		shelfNoticeTimer.current = window.setTimeout(() => {
			shelfNoticeTimer.current = null;
			setShelfNotice(null);
		}, 2600);
	};

	const addShelfItems = (paths: string[]) => {
		if (paths.length === 0) return;
		const prev = shelfItemsRef.current;
		const existing = new Set(prev.filter((i) => i.path).map((i) => i.path));
		const fresh: ShelfItem[] = [];
		for (const p of paths) {
			if (existing.has(p)) continue;
			existing.add(p);
			fresh.push({
				id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
				type: "file",
				name: p.split("\\").pop()?.split("/").pop() || p,
				path: p,
				timestamp: Date.now()
			});
		}
		if (fresh.length === 0) return;

		const room = Math.max(0, SHELF_MAX_ITEMS - prev.length);
		if (room === 0) {
			showShelfNotice(`Shelf is full (${SHELF_MAX_ITEMS}) — remove an item first`);
			return;
		}
		if (fresh.length > room) {
			showShelfNotice(`Shelf is full — added ${room} of ${fresh.length} items`);
		}
		const added = fresh.slice(0, room);
		markShelfNew(added.map((i) => i.id));
		setShelfItems([...added, ...prev]);
	};

	const addShelfText = (text: string) => {
		const prev = shelfItemsRef.current;
		if (prev.some((i) => i.type === "text" && i.text === text)) return;
		if (prev.length >= SHELF_MAX_ITEMS) {
			showShelfNotice(`Shelf is full (${SHELF_MAX_ITEMS}) — remove an item first`);
			return;
		}
		const item: ShelfItem = {
			id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
			type: "text",
			name: text.trim().replace(/\s+/g, " ").slice(0, 48),
			text,
			timestamp: Date.now()
		};
		markShelfNew([item.id]);
		setShelfItems([item, ...prev]);
	};

	// Files open in their default app; text items are copied to the clipboard
	// so they can be pasted anywhere. A file that no longer exists is reported
	// and dropped from the shelf.
	const openShelfItem = (item: ShelfItem) => {
		if (item.type === "text") {
			if (!item.text) return;
			writeText(item.text)
				.then(() => showShelfNotice("Copied to clipboard"))
				.catch((e) => console.error("[shelf] copy failed:", e));
			return;
		}
		if (item.path) {
			const path = item.path;
			invoke("shelf_open", { path }).catch(async (e) => {
				console.error("[shelf] open failed:", path, e);
				// Only a confirmed-missing file leaves the shelf; a transient
				// failure (e.g. no association) must not remove it.
				const alive = await invoke<string[]>("shelf_prune", { paths: [path] }).catch(() => null);
				if (alive && alive.length === 0) {
					showShelfNotice("File no longer exists — removed from shelf");
					removeShelfItem(item.id);
				} else {
					showShelfNotice("Couldn't open the file");
				}
			});
		}
	};

	// Shelf drag-and-drop plumbing. The backend registers its own OLE drop
	// target (src-tauri/src/shelf.rs) because wry's target can go stale when
	// WebView2 recreates its render child; it emits the shelf-drag-* events
	// below. Drag-enter switches to the shelf panel (restoring the previous
	// mode on cancel); drop stashes the real file paths.
	const [isDragHovering, setIsDragHovering] = useState(false);
	const shelfPrevModeRef = useRef<BloomMode>("status");
	const shelfPrevOpenRef = useRef(false);
	const shelfDragActiveRef = useRef(false);
	useEffect(() => {
		let unlisteners: (() => void)[] = [];
		let cancelled = false;

		const enter = (event: { payload: { paths?: string[]; text?: string } }) => {
			// Our own drag-out starts with the cursor over the notch; ignore
			// the enter it causes instead of flashing the hover state.
			if (shelfSelfDragRef.current) return;
			console.info("[shelf] drag-enter:", event.payload.paths ?? event.payload.text);
			setIsDragHovering(true);
			const prev = bloomModeRef.current;
			if (prev === "shelf") return;
			shelfPrevModeRef.current = prev;
			shelfPrevOpenRef.current = isOpenRef.current;
			shelfDragActiveRef.current = true;
			setIsOpen(true);
			setBloomMode("shelf");
		};
		const leave = () => {
			setIsDragHovering(false);
			if (shelfDragActiveRef.current) {
				shelfDragActiveRef.current = false;
				if (bloomModeRef.current === "shelf") {
					setIsOpen(shelfPrevOpenRef.current);
					setBloomMode(shelfPrevModeRef.current);
				}
			}
		};
		const drop = (event: { payload: { paths?: string[]; text?: string } }) => {
			setIsDragHovering(false);
			shelfDragActiveRef.current = false;
			shelfSelfDropAtRef.current = Date.now();
			const paths = event.payload.paths ?? [];
			const text = event.payload.text;
			console.info("[shelf] drop:", paths.length > 0 ? paths : text);
			if (paths.length === 0 && !text) return;
			if (paths.length > 0) addShelfItems(paths);
			if (text) addShelfText(text);
			setIsOpen(true);
			setBloomMode("shelf");
			collapseShelfSoon();
		};

		Promise.all([
			listen<{ paths?: string[]; text?: string }>("shelf-drag-enter", enter),
			listen("shelf-drag-leave", leave),
			listen<{ paths?: string[]; text?: string }>("shelf-drag-drop", drop)
		])
			.then((fns) => {
				if (cancelled) {
					fns.forEach((fn) => fn());
				} else {
					unlisteners = fns;
				}
			})
			.catch((e) => console.error("[shelf] drag-drop listener failed:", e));

		invoke("setup_shelf_drop").catch((e) => console.error("[shelf] drop target setup failed:", e));

		return () => {
			cancelled = true;
			unlisteners.forEach((fn) => fn());
		};
	}, []);

	// Window height is now kept constant to prevent rendering layout lag and sharp corners

	const lastScrollTime = useRef(0);
	const handleWheel = (e: React.WheelEvent) => {
		const target = e.target as HTMLElement;
		if (
			target.closest(".calendar-grid") ||
			target.closest(".timer-column") ||
			target.closest(".shelf-content") ||
			target.closest(".mq") ||
			target.closest(".qp-list")
		) {
			return;
		}

		const now = Date.now();
		if (now - lastScrollTime.current < 250) return;

		// Use absolute values to detect horizontal swipe gestures on trackpad
		const delta = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY;
		if (Math.abs(delta) < 5) return; // Ignore tiny movements

		if (cycleBloomMode(delta > 0 ? 1 : -1)) lastScrollTime.current = now;
	};


	// Wheel and swipe navigate panels, even from a collapsed live activity.
	// Music and shelf remain entry points through their activity affordances.
	const cycleBloomMode = (direction: 1 | -1) => {
		const activePanels = NOTCH_PANELS.filter((p) => !(quotaPinned && p === "quotas"));
		const currentIndex = activePanels.indexOf(bloomMode as NotchPanel);
		const nextMode =
			currentIndex === -1
				? nextOpeningPanel()
				: activePanels[(currentIndex + direction + activePanels.length) % activePanels.length];
		setIsOpen(true);
		setBloomMode(nextMode);
		return true;
	};

	// Timer state
	const [timerSeconds, setTimerSeconds] = useState(0);
	const [isTimerRunning, setIsTimerRunning] = useState(false);
	const [isTimerFinished, setIsTimerFinished] = useState(false);
	const [isEditingTimer, setIsEditingTimer] = useState(false);
	const [timerEditDigits, setTimerEditDigits] = useState("");
	const [lastDurationSeconds, setLastDurationSeconds] = useState(() => {
		const stored = parseInt(localStorage.getItem("bloom-timer-last-duration") || "", 10);
		return Number.isFinite(stored) && stored > 0 ? stored : 25 * 60;
	});
	const timerInputRef = useRef<HTMLInputElement>(null);
	const timerEditActiveRef = useRef(false);
	const timerIntervalRef = useRef<any>(null);
	const prevTimerFinishedRef = useRef(false);

	const formatTimerTime = (totalSeconds: number) => {
		const mins = Math.floor(Math.abs(totalSeconds) / 60);
		const secs = Math.abs(totalSeconds) % 60;
		return `${mins}:${secs.toString().padStart(2, "0")}`;
	};

	const startTimer = (seconds: number) => {
		// Unlock audio here (user gesture) so the completion chime can play later.
		getTimerChimeCtx();
		timerEditActiveRef.current = false;
		setIsEditingTimer(false);
		setTimerSeconds(seconds);
		setLastDurationSeconds(seconds);
		localStorage.setItem("bloom-timer-last-duration", String(seconds));
		setIsTimerRunning(true);
		setIsTimerFinished(false);
	};

	const resetTimer = () => {
		timerEditActiveRef.current = false;
		setIsEditingTimer(false);
		setIsTimerRunning(false);
		setTimerSeconds(0);
		setIsTimerFinished(false);
	};

	// Idle/finished play restarts the last duration; paused play resumes.
	const handlePrimaryTimerAction = () => {
		if (isEditingTimer) {
			commitTimerEdit(true);
		} else if (isTimerRunning) {
			setIsTimerRunning(false);
		} else if (timerSeconds > 0) {
			getTimerChimeCtx();
			setIsTimerRunning(true);
		} else {
			startTimer(lastDurationSeconds);
		}
	};

	// Clicking the big time turns it into a masked editor. Digits fill from the
	// right, so the colon never has to be typed.
	const beginTimerEdit = () => {
		if (isTimerRunning || isEditingTimer) return;
		if (isTimerFinished) resetTimer();
		const base = timerSeconds > 0 ? timerSeconds : lastDurationSeconds;
		setTimerEditDigits(timerSecondsToDigits(base));
		timerEditActiveRef.current = true;
		setIsEditingTimer(true);
	};

	const commitTimerEdit = (start: boolean) => {
		if (!timerEditActiveRef.current) return;
		timerEditActiveRef.current = false;
		setIsEditingTimer(false);
		const total = timerDigitsToSeconds(timerEditDigits);
		if (total === null) return;
		const base = timerSeconds > 0 ? timerSeconds : lastDurationSeconds;
		// A tap that changes nothing (e.g. paused timer) must not reset the state.
		if (!start && total === base) return;
		localStorage.setItem("bloom-timer-last-duration", String(total));
		setLastDurationSeconds(total);
		setTimerSeconds(start ? total : 0);
		setIsTimerRunning(start);
		setIsTimerFinished(false);
		if (start) getTimerChimeCtx();
	};

	useEffect(() => {
		if (isEditingTimer) {
			timerInputRef.current?.focus();
			timerInputRef.current?.select();
		}
	}, [isEditingTimer]);

	const timerEditValid = timerDigitsToSeconds(timerEditDigits) !== null;

	const timerState: "idle" | "running" | "paused" | "finished" = isEditingTimer
		? "idle"
		: isTimerFinished
			? "finished"
			: isTimerRunning
				? "running"
				: timerSeconds > 0
					? "paused"
					: "idle";
	const timerDisplaySeconds =
		timerSeconds > 0 || isTimerFinished ? timerSeconds : lastDurationSeconds;
	const primaryTimerLabel = isTimerRunning
		? "Pause"
		: timerSeconds > 0
			? "Resume"
			: isTimerFinished
				? "Restart"
				: "Start";
	const timerEndTime =
		timerSeconds > 0 && !isTimerFinished
			? new Date(Date.now() + timerSeconds * 1000).toLocaleTimeString([], {
					hour: "2-digit",
					minute: "2-digit",
					hour12: !timeFormat24h
				})
			: null;

	// Split the display time into individual flip-clock digits. Keys are counted
	// from the right so existing digits keep their identity when minutes grow
	// from two digits to three. While editing, the same clock renders the digits
	// being typed so the layout never changes.
	const clockText = isEditingTimer
		? formatTimerDigits(timerEditDigits)
		: formatTimerTime(timerDisplaySeconds);
	const [clockMinutesText, clockSecondsText] = clockText.split(":");
	const clockMinutesPadded = clockMinutesText.padStart(2, "0");
	const clockCompact = clockMinutesPadded.length > 2;
	const minuteDigitItems = clockMinutesPadded
		.split("")
		.map((digit, i, arr) => ({ digit, key: `m${arr.length - i}` }));
	const secondDigitItems = clockSecondsText
		.split("")
		.map((digit, i, arr) => ({ digit, key: `s${arr.length - i}` }));

	useEffect(() => {
		if (isTimerRunning && timerSeconds > 0) {
			timerIntervalRef.current = setInterval(() => {
				setTimerSeconds((s) => s - 1);
			}, 1000);
		} else if (isTimerRunning && timerSeconds === 0) {
			setIsTimerRunning(false);
			setIsTimerFinished(true);
		}
		return () => {
			if (timerIntervalRef.current) clearInterval(timerIntervalRef.current);
		};
	}, [isTimerRunning, timerSeconds === 0]);

	// Completion moment: chime once. The alert itself is dispatched through
	// the attention controller below, which owns the peek.
	useEffect(() => {
		if (isTimerFinished && !prevTimerFinishedRef.current) {
			if (settingsTimerSoundEnabled) playTimerChime();
		}
		prevTimerFinishedRef.current = isTimerFinished;
	}, [isTimerFinished, settingsTimerSoundEnabled]);

	// Which agents the notch draws. The helper keeps polling all of them; this
	// only filters the surfaces, and hides by exception so a new agent still shows.
	const [quotaHiddenAgents, setQuotaHiddenAgents] = useState<string[]>(() =>
		parseHiddenAgents(localStorage.getItem(QUOTA_HIDDEN_AGENTS_KEY))
	);

	// Quota readings live at App level so the controller sees urgent agents
	// while the panel is closed and the rows survive panel remounts. The
	// event carries the readings array, not a protocol line; wrap it so
	// parseSnapshot stays the single decoder.
	const [quotaReadings, setQuotaReadings] = useState<QuotaAgent[]>([]);
	const [hasQuotaSnapshot, setHasQuotaSnapshot] = useState(false);
	const shownQuotaReadings = useMemo(
		() => visibleReadings(quotaReadings, quotaHiddenAgents),
		[quotaReadings, quotaHiddenAgents]
	);
	useEffect(() => {
		const unlisten = listen<QuotaAgent[]>("quota-snapshot", (event) => {
			const line = JSON.stringify({ v: 1, op: "snapshot", readings: event.payload });
			const next = parseSnapshot(line);
			if (next !== null) {
				setQuotaReadings(next);
				setHasQuotaSnapshot(true);
			}
		});
		return () => {
			unlisten.then((f) => f());
		};
	}, []);

	// The quota space's selected agent and pin live here: the urgent alert
	// sets the selection before opening, and the pin rides the shared notch
	// pin so pointer-leave release defers exactly like a Wi-Fi password.
	const [quotaSelected, setQuotaSelected] = useState<string | undefined>(undefined);
	// Measured content height of the quota space, in the pill's own CSS px.
	// The observer watches `.qs`, not the scroll viewport, so the cap never
	// feeds back into the requested height.
	const [quotaSpaceHeight, setQuotaSpaceHeight] = useState(0);
	const [quotaSpaceWidth, setQuotaSpaceWidth] = useState(0);
	// Measured content height of the integrated Music/Controls body, in the
	// pill's own CSS px. Those panels used fixed estimates that ran a row
	// short, so the dropdown drew a scrollbar. The body is height:auto in
	// MyFinder, so its box is the content's own height and does not track the
	// pill being animated to it.
	const [finderBodyHeight, setFinderBodyHeight] = useState(0);
	const finderBodyMeasureRef = useCallback((el: HTMLDivElement | null) => {
		if (!el) return undefined;
		// scrollHeight is a layout extent: it ignores the handoff transform and
		// `zoom`, so this is already the pill's own CSS px.
		const measure = () => setFinderBodyHeight(el.scrollHeight);
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		measure();
		return () => ro.disconnect();
	}, []);
	const quotaSpaceMeasureRef = useCallback(
		(el: HTMLDivElement | null) => {
			const target = el?.firstElementChild;
			if (!el || !target) return undefined;
			// `.qs` sits below the status row inside `.bloom`; the island must
			// cover its bottom edge, not merely its own height. Measuring the
			// edge delta is invariant to the pill's enter/leave translate.
			const measure = () => {
				const pill = el.closest(".bloom");
				const h = pill
					? target.getBoundingClientRect().bottom - pill.getBoundingClientRect().top
					: target.getBoundingClientRect().height;
				setQuotaSpaceHeight(h / scale);
				// zoom scales client rects but not scroll extents, so
				// scrollWidth is already in the island's unzoomed CSS px.
				// The pill's 12px side padding is added back so the island
				// covers all of `.qs` inside its padded area.
				// innerWidth/innerHeight are the window's unzoomed CSS
				// size; /scale returns the display bound to the island's
				// coordinate space. `.qs` and the scrollport get the caps
				// (--qs-max-w/-h, on the viewport element so `.qs`
				// inherits) while the island itself is clamped here too.
				const capW = Math.floor(window.innerWidth / scale);
				el.style.setProperty("--qs-max-w", `${capW - 24}px`);
				el.style.setProperty("--qs-max-h", `${Math.floor(window.innerHeight / scale) - 36}px`);
				setQuotaSpaceWidth(Math.min(target.scrollWidth + 24, capW));
			};
			const ro = new ResizeObserver(measure);
			ro.observe(target);
			// The caps follow the real window size: set_quota_panel_height
			// resizes the window under the scrollport, which never resizes
			// `.qs` itself, so the observer alone would keep stale caps.
			window.addEventListener("resize", measure);
			return () => {
				ro.disconnect();
				window.removeEventListener("resize", measure);
			};
		},
		[scale]
	);
	// `.qs` is width:max-content, so a changed ring row resizes the element
	// itself and the observer re-measures with no reset bookkeeping.

	// One visibility controller: the open panel is only ever moved by the
	// user, alerts record themselves beside it, and a deferred gesture
	// swallows an alert whole.
	const [attention, dispatchAttention] = useReducer(reduceAttention, {
		panel: "status",
		alert: null,
		deferred: false
	});
	useEffect(() => {
		dispatchAttention({ type: "open-panel", panel: bloomMode });
	}, [bloomMode]);

	// Timer completion and an urgent quota reading are both alerts. The
	// wanted id is computed, not edged per source, so when the newer one
	// ends an older live condition comes back on its own.
	const attentionAlertId = isTimerFinished
		? "timer"
		: shownQuotaReadings.some(urgent)
			? "quota"
			: null;
	// The wanted id is compared against what the controller recorded, not a
	// separate ref: an alert dropped while a gesture deferred it re-dispatches
	// itself once the last gesture ends.
	const attentionAlert = attention.alert;
	const attentionDeferred = attention.deferred;
	useEffect(() => {
		if (attentionDeferred) return;
		if (attentionAlertId === attentionAlert) return;
		if (attentionAlert !== null) {
			dispatchAttention({ type: "alert-finished" });
		}
		if (attentionAlertId !== null) {
			dispatchAttention({ type: "alert", id: attentionAlertId });
		}
	}, [attentionAlertId, attentionAlert, attentionDeferred]);

	// A recorded alert peeks the notch in peek mode on its own edge only, so a
	// notch-mode switch does not re-peek a stale alert. attention.alert never
	// feeds setBloomMode, so an alert cannot move the open panel.
	const prevPeekAlertRef = useRef<string | null>(null);
	useEffect(() => {
		const edge = attentionAlert !== null && attentionAlert !== prevPeekAlertRef.current;
		prevPeekAlertRef.current = attentionAlert;
		if (edge && notchMode === "peek") triggerEventPeek(6000);
	}, [attentionAlert, notchMode, triggerEventPeek]);

	// An urgent reading keeps a peek-mode notch on screen, but the usage strip
	// itself is only a hover detail of the collapsed notch. It does not cover
	// an open panel.
	const quotaUrgent =
		attentionAlert === "quota" && quotaView(shownQuotaReadings, bloomMode).showInRest;
	// The hover row draws the badge itself now — red rings at 90%+ — so an
	// urgent reading no longer covers it. quotaAlertVisible stays exported
	// for the unit tests that pin its rule down.
	const quotaAlertActive = false;
	// The pinned agent's compact ring badge on the collapsed hover row, or
	// the busiest reading when nothing is pinned yet.
	const hoverAgent = hoverQuotaAgent(shownQuotaReadings, quotaSelected);
	useEffect(() => {
		setQuotaAlertHold(quotaUrgent);
	}, [quotaUrgent]);
	// A finished timer keeps its compact-timer display and additionally
	// widens the pill in fixed/smart modes, like the battery pulses.
	const alertExpanded = quotaAlertActive || attentionAlert === "timer";

	// Busy gestures defer alerts. Each source owns only its rising edge; one
	// derived flag owns the falling edge, so a drag ending while the timer is
	// still being edited does not free alerts early.
	const typingActive = isEditingTimer;
	const dragActive = isDragHovering || shelfDragOut;
	const anyGesture = typingActive || dragActive;
	const prevGesturesRef = useRef({
		typing: false,
		drag: false,
		any: false
	});
	useEffect(() => {
		const prev = prevGesturesRef.current;
		if (typingActive && !prev.typing) dispatchAttention({ type: "typing" });
		if (dragActive && !prev.drag) dispatchAttention({ type: "dragging" });
		if (prev.any && !anyGesture) dispatchAttention({ type: "clear-defer" });
		prevGesturesRef.current = {
			typing: typingActive,
			drag: dragActive,
			any: anyGesture
		};
	}, [typingActive, dragActive, anyGesture]);

	const lastTrackRef = useRef<string | null>(null);
	const lastPlayingRef = useRef<boolean>(false);
	const [musicActivityVisible, setMusicActivityVisible] = useState(false);

	// Keep a paused media session visible briefly so it can be resumed, then
	// clear its compact activity even if Windows retains the media session.
	useEffect(() => {
		if (!settingsMusicModeEnabled || !mediaInfo.has_media) {
			setMusicActivityVisible(false);
			return;
		}
		setMusicActivityVisible(true);
		if (isPlaying) return;
		const timeout = window.setTimeout(() => setMusicActivityVisible(false), 5000);
		return () => window.clearTimeout(timeout);
	}, [settingsMusicModeEnabled, mediaInfo.has_media, isPlaying]);

	// Like the Dynamic Island, the collapsed notch shows one live activity at a
	// time. Music fades a few seconds after playback stops. A set timer
	// (running, paused or just finished) takes the pill, and music then moves to
	// its own bubble beside it.
	const timerActive = timerSeconds > 0 || isTimerFinished;
	const musicActivity =
		settingsMusicModeEnabled &&
		settingsMusicCompactNotch &&
		mediaInfo.has_media &&
		musicActivityVisible;
	const collapsedMode = musicActivity && !timerActive ? "music" : "status";
	const showMusicBubble = musicActivity && timerActive && !isOpen && isVisible && !isHidden;
	const showShelfBubble =
		shelfItems.length > 0 && !isOpen && isVisible && !isHidden;
	restingModeRef.current = collapsedMode;
	bloomModeRef.current = bloomMode;

	// A click opens the last-used panel when memory is enabled, otherwise the
	// selected default.
	const openingPanel = (): NotchPanel =>
		rememberPanelEnabled ? (lastPanel ?? defaultPanel) : defaultPanel;

	const nextOpeningPanel = (): NotchPanel => {
		const preferred = openingPanel();
		if (quotaPinned && preferred === "quotas") {
			return defaultPanel !== "quotas" ? defaultPanel : "timer";
		}
		return preferred;
	};

	// Expands the notch into a panel, or into an activity's own view.
	const openNotch = (view: NotchPanel | "music" | "shelf") => {
		setIsOpen(true);
		setBloomMode(view);
	};

	const rememberPanel = (panel: NotchPanel) => {
		// Overview is an optional glance; opening the notch must keep using
		// the last real panel, so it is never remembered.
		if (panel === "overview" || panel === lastPanel) return;
		setLastPanel(panel);
		localStorage.setItem("bloom-notch-last-panel", panel);
		invoke("save_setting", { key: "bloom-notch-last-panel", value: panel }).catch(console.error);
	};
	rememberPanelRef.current = rememberPanel;

	// Called by views that take typed input; see releaseNotchHover.
	const setNotchPinned = useCallback((pinned: boolean) => {
		notchPinnedRef.current = pinned;
		if (!pinned && releaseDeferredRef.current) {
			releaseDeferredRef.current = false;
			releaseNotchHover();
		}
		// releaseNotchHover only touches refs and state setters.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

 
	// The quota pin persists across panel switches and restarts; only an
	// expanded quota dropdown holds the dropdown region open on hover leave,
	// whereas a pinned compact strip stays in the topbar independently.
	const handleQuotaPin = useCallback((pinned: boolean) => {
		setQuotaPinned(pinned);
		if (!pinned) {
			setIsOpen(true);
			setBloomMode("quotas");
		}
		localStorage.setItem("bloom-quota-pinned", String(pinned));
		invoke("save_setting", {
			key: "bloom-quota-pinned",
			value: String(pinned)
		}).catch(console.error);
	}, []);

	const dropdownPinned = quotaPinned && !quotaCompact && bloomMode === "quotas";
	useEffect(() => {
		setNotchPinned(dropdownPinned);
	}, [dropdownPinned, setNotchPinned]);

	const handleQuotaCompact = useCallback(
		(compact: boolean) => {
			// Already there: no write, no hover-release suppression.
			if (compact === quotaCompact) return;
			// Collapsing shrinks the island out from under the pointer; that
			// hover-end is not the person leaving, so it must not close the strip.
			if (compact) suppressNextHoverRelease();
			setQuotaCompact(compact);
			localStorage.setItem("bloom-quota-compact", String(compact));
			invoke("save_setting", {
				key: "bloom-quota-compact",
				value: String(compact)
			}).catch(console.error);
		},
		[quotaCompact]
	);

	// Close acts like the pointer leaving the open dropdown without wiping a
	// persistent topbar pin; if an expanded quota panel was pinned, collapse
	// it back to the pinned topbar strip.
	const closeQuotaSpace = () => {
		if (quotaPinned && !quotaCompact) {
			handleQuotaCompact(true);
		}
		setNotchPinned(false);
		releaseDeferredRef.current = false;
		releaseNotchHover();
	};

	// In peek mode, briefly show the notch when a new track starts or playback
	// resumes. The notch itself never switches views for media; the collapsed
	// pill follows the live-activity rule below.
	useEffect(() => {
		const isNewTrackWhilePlaying = mediaInfo.title !== lastTrackRef.current && isPlaying;
		const justStartedPlaying = isPlaying && !lastPlayingRef.current;

		// Peek notch on media events (play start or track change)
		if (notchMode === "peek" && (isNewTrackWhilePlaying || justStartedPlaying)) {
			triggerEventPeek(3000);
		}

		lastTrackRef.current = mediaInfo.title;
		lastPlayingRef.current = isPlaying;
	}, [isPlaying, mediaInfo.title, notchMode, triggerEventPeek]);

	// Leave the music view when music is turned off or the player ends its media
	// session: an open notch falls back to the remembered panel.
	useEffect(() => {
		if (bloomMode === "music" && !(settingsMusicModeEnabled && mediaInfo.has_media)) {
			setBloomMode(isOpen ? openingPanel() : collapsedMode);
		}
	}, [settingsMusicModeEnabled, mediaInfo.has_media, bloomMode, isOpen]);

	// The collapsed notch always shows its current live activity: music while a
	// session exists, the timer (with music in its bubble) once one is set, and
	// the clock otherwise. An open notch is left alone.
	useEffect(() => {
		if (isOpen) return;
		if ((bloomMode === "status" || bloomMode === "music") && bloomMode !== collapsedMode) {
			setBloomMode(collapsedMode);
		}
	}, [collapsedMode, bloomMode, isOpen]);

	// Battery API
	useEffect(() => {
		let battery: any = null;

		const initBattery = async () => {
			try {
				battery = await (navigator as any).getBattery();

				const updateBattery = () => {
					setBatteryLevel(Math.round(battery.level * 100));
					setIsCharging(battery.charging);
				};

				updateBattery();

				battery.addEventListener("levelchange", updateBattery);
				battery.addEventListener("chargingchange", updateBattery);

				return () => {
					battery.removeEventListener("levelchange", updateBattery);
					battery.removeEventListener("chargingchange", updateBattery);
				};
			} catch (e) {
				// Battery API not supported
			}
		};

		initBattery();
	}, []);

	// Listen for Volume Changes
	useEffect(() => {
		const unlisten = listen<{ volume: number; is_muted: boolean }>("volume-change", (event) => {
			setVolume(event.payload.volume);
			setIsMuted(event.payload.is_muted);
		});
		return () => {
			unlisten.then((fn) => fn());
		};
	}, []);

	// The system volume is the music sliders' fallback when the player has no
	// audio session of its own.
	useEffect(() => {
		invoke<{ volume: number; is_muted: boolean }>("get_volume_state")
			.then((state) => {
				setVolume(state.volume);
				setIsMuted(state.is_muted);
			})
			.catch(() => {});
	}, []);

	// Poll system metrics for status widgets
	useEffect(() => {
		const fetchMetrics = () => {
			invoke<number>("get_cpu_usage")
				.then(setCpuUsage)
				.catch((e) => console.warn("CPU:", e));
			invoke<number>("get_disk_space")
				.then(setDiskSpace)
				.catch((e) => console.warn("Disk:", e));
			invoke<[number, number]>("get_network_speed")
				.then(([up, down]) => {
					setNetUpSpeed(up);
					setNetDownSpeed(down);
				})
				.catch(() => {});
		};
		fetchMetrics();
		const interval = setInterval(fetchMetrics, 3000);
		return () => clearInterval(interval);
	}, []);

	// Native Windows Media Controls - Listen for updates from background worker
	useEffect(() => {
		const unlisten = listen<MediaInfo>("media-update", (event) => {
			const info = event.payload;
			if (!info) return;

			setMediaInfo((prev) => {
				// Find if artwork changed by checking the first element
				const prevArt = prev.artwork?.[0];
				const nextArt = info.artwork?.[0];
				const artChanged = prevArt !== nextArt;

				if (
					prev.title === info.title &&
					prev.artist === info.artist &&
					prev.is_playing === info.is_playing &&
					prev.has_media === info.has_media &&
					prev.target_generation === info.target_generation &&
					JSON.stringify(prev.lyrics) === JSON.stringify(info.lyrics) &&
					!artChanged &&
					prev.position_ms === info.position_ms &&
					prev.duration_ms === info.duration_ms
				) {
					return prev;
				}

				// Update playing state separately for the hook triggers
				setIsPlaying(info.is_playing);

				if (info.artwork && info.artwork.length > 0) {
					const newArt = info.artwork[0];
					setAlbumArtUrl((prev) => {
						if (prev !== newArt) {
							setAlbumArtKey((k) => k + 1);
							return newArt;
						}
						return prev;
					});
				} else {
					setAlbumArtUrl(null);
				}

				return info;
			});
		});

		return () => {
			unlisten.then((fn) => fn());
		};
	}, []);

	// Media controls via Tauri commands
	/* Unused saveAndBroadcast removed to fix TS build error */
	const togglePlayPause = useCallback(async () => {
		try {
			await invoke("media_action", {
				generation: mediaInfo.target_generation ?? "",
				op: "playPause"
			});
		} catch (e) {
			console.error("Failed to toggle play/pause:", e);
		}
	}, [mediaInfo.target_generation]);

	const skipNext = useCallback(async () => {
		try {
			await invoke("media_action", { generation: mediaInfo.target_generation ?? "", op: "next" });
		} catch (e) {
			console.error("Failed to skip next:", e);
		}
	}, [mediaInfo.target_generation]);

	const skipPrevious = useCallback(async () => {
		try {
			await invoke("media_action", {
				generation: mediaInfo.target_generation ?? "",
				op: "previous"
			});
		} catch (e) {
			console.error("Failed to skip previous:", e);
		}
	}, [mediaInfo.target_generation]);

	// Slide-push animation for prev/next buttons
	const prevFront = useAnimation();
	const prevBack = useAnimation();
	const nextFront = useAnimation();
	const nextBack = useAnimation();

	const animatePrev = useCallback(async () => {
		prevFront.set({ x: 0 });
		prevBack.set({ x: 30 });
		prevFront.start({ x: -30, transition: { duration: 0.18, ease: [0.4, 0, 0.2, 1] } });
		await prevBack.start({ x: 0, transition: { duration: 0.18, ease: [0.4, 0, 0.2, 1] } });
		skipPrevious();
	}, [skipPrevious, prevFront, prevBack]);

	const animateNext = useCallback(async () => {
		nextFront.set({ x: 0 });
		nextBack.set({ x: -30 });
		nextFront.start({ x: 30, transition: { duration: 0.18, ease: [0.4, 0, 0.2, 1] } });
		await nextBack.start({ x: 0, transition: { duration: 0.18, ease: [0.4, 0, 0.2, 1] } });
		skipNext();
	}, [skipNext, nextFront, nextBack]);

	const sendVolume = useTrailingThrottle((newVol: number) => {
		invoke("set_volume", { volume: newVol }).catch(() => {});
	}, 50);

	const handleVolumeChange = useCallback(
		(newVol: number) => {
			setVolume(newVol);
			// set_volume unmutes for any non-zero level.
			setIsMuted(newVol === 0);
			sendVolume(newVol);
		},
		[sendVolume]
	);

	const sendMediaVolume = useTrailingThrottle((pid: number, newVol: number) => {
		invoke("set_app_volume", { pid, volume: newVol }).catch(() => {});
	}, 50);

	const handleMediaVolumeChange = useCallback(
		(newVol: number) => {
			setMediaVolume(newVol);
			// set_app_volume unmutes for any non-zero level.
			setMediaMuted(newVol === 0);
			if (mediaSessionPid != null) sendMediaVolume(mediaSessionPid, newVol);
		},
		[mediaSessionPid, sendMediaVolume]
	);

	// The media volume row controls its player's mixer level, not the system
	// endpoint. Look the session up when the music view opens; no session means
	// the system fallback stays in charge.
	useEffect(() => {
		const app = mediaInfo.app;
		if (!isOpen || bloomMode !== "music" || !app) return;
		let alive = true;
		invoke<AudioSessionInfo[]>("get_audio_sessions")
			.then((sessions) => {
				const session = sessions.find((s) => appMatchesSession(s, app));
				if (!alive) return;
				setMediaSessionPid(session?.pid ?? null);
				if (session) {
					setMediaVolume(session.volume);
					setMediaMuted(session.is_muted);
				}
			})
			.catch(() => {});
		return () => {
			alive = false;
		};
	}, [isOpen, bloomMode, mediaInfo.app]);

	// The notch never takes focus, and native `title` tooltips only show for an
	// active window, so the status widgets draw their own. A tooltip opens
	// after a tooltip-like delay, by which time the battery details fetched on
	// enter are in. Under the status strip it hangs below the pill; once the
	// notch is open the pill is the tall panel, so it sits beside the pill on
	// the widget's side instead, level with the widget. A quota strip mark
	// passes `under` so its card stays beneath the mark it belongs to.
	const showStatusTooltip = useCallback(
		(id: string, widget: HTMLElement, under = false) => {
			if (statusTooltipTimerRef.current) clearTimeout(statusTooltipTimerRef.current);
			statusTooltipTimerRef.current = setTimeout(() => {
				const pill = bloomRef.current?.getBoundingClientRect();
				const own = widget.getBoundingClientRect();
				// Client rects are zoomed by the notch scale; offsets are not.
				if (!pill || under) {
					setStatusTooltip({ id, side: "below", offset: 8 });
				} else if (!isOpen) {
					const gap = (pill.bottom - own.bottom) / scale;
					setStatusTooltip({ id, side: "below", offset: Math.max(0, gap) + 8 });
				} else if (own.left + own.width / 2 >= pill.left + pill.width / 2) {
					const gap = (pill.right - own.right) / scale;
					setStatusTooltip({ id, side: "right", offset: Math.max(0, gap) + 10 });
				} else {
					const gap = (own.left - pill.left) / scale;
					setStatusTooltip({ id, side: "left", offset: Math.max(0, gap) + 10 });
				}
			}, 400);
		},
		[scale, isOpen]
	);

	const hideStatusTooltip = useCallback(() => {
		if (statusTooltipTimerRef.current) clearTimeout(statusTooltipTimerRef.current);
		statusTooltipTimerRef.current = null;
		setStatusTooltip(null);
	}, []);

	// Widgets unmount when the notch collapses, without a mouse leave, and an
	// open card is placed for the notch's shape when it opened. MyFinder's bar
	// never collapses, and the pointer entering it flips `isHovered` while it
	// is already over a mark, so clearing on every change there swallowed the
	// first hover and only a move between marks brought a card back.
	useEffect(() => {
		if (!integrated) hideStatusTooltip();
	}, [integrated, isHovered, isOpen, hideStatusTooltip]);

	// An open panel covers the bar, and a hidden bar takes its marks away, so
	// neither may keep a card alive without a pointer on the mark.
	useEffect(() => {
		if (integrated && (isOpen || isHidden)) hideStatusTooltip();
	}, [integrated, isOpen, isHidden, hideStatusTooltip]);

	useLayoutEffect(() => {
		const el = pillClusterRef.current;
		if (!el) return;
		const kids = Array.from(el.children) as HTMLElement[];
		if (kids.length === 0) return;
		// offsetWidth ignores absolutely positioned tooltip cards. The gap is read,
		// not assumed: MyFinder's strip spaces its items wider than the notch does.
		const gap = parseFloat(getComputedStyle(el).columnGap) || 0;
		const content = kids.reduce((w, k) => w + k.offsetWidth, 0) + Math.max(0, kids.length - 1) * gap;
		const needed = Math.ceil(content + 24);
		// The pill must always cover the row it draws, so the width follows the
		// measured content in both directions. Freezing or zeroing it here
		// clips the clock off the resting notch.
		setClusterWidth((prev) => (prev === needed ? prev : needed));
		const housing = el.querySelector<HTMLElement>(".quota-housing");
		if (housing) {
			const other = needed - housing.offsetWidth;
			setStripOther((prev) => (prev === other ? prev : other));
		}
	});

	const cycleTimePanel = (e: React.MouseEvent) => {
		e.stopPropagation();
		if (isTimerFinished) {
			resetTimer();
			return;
		}
		if (!isOpen || (bloomMode === "quotas" && quotaCompact)) {
			openNotch(nextOpeningPanel());
		} else {
			cycleBloomMode(1);
		}
	};

	// Where a status tooltip card sits and how it slides in, by placement.
	const statusTooltipMotion = (side: "below" | "left" | "right", offset: number) => {
		if (side === "below") {
			return {
				style: { top: `calc(100% + ${offset}px)`, left: "50%" },
				hidden: { opacity: 0, x: "-50%", y: -4 },
				shown: { opacity: 1, x: "-50%", y: 0 }
			};
		}
		// Top-aligned with the widget rather than centered on it: the widgets
		// sit near the top of the screen, which would cut a centered card off.
		const nudge = side === "right" ? -4 : 4;
		return {
			style: { top: -8, [side === "right" ? "left" : "right"]: `calc(100% + ${offset}px)` },
			hidden: { opacity: 0, x: nudge, y: 0 },
			shown: { opacity: 1, x: 0, y: 0 }
		};
	};

	// A status widget with its tooltip card, which hangs outside the pill.
	const statusWidget = (
		id: string,
		tooltip: string,
		content: React.ReactNode,
		onEnter?: () => void
	) => (
		<div
			className="passive-feature"
			key={id}
			onMouseEnter={(e) => {
				onEnter?.();
				showStatusTooltip(id, e.currentTarget);
			}}
			onMouseLeave={hideStatusTooltip}
		>
			{content}
			<AnimatePresence>
				{statusTooltip?.id === id &&
					(() => {
						const placed = statusTooltipMotion(statusTooltip.side, statusTooltip.offset);
						return (
							<motion.div
								className="status-tooltip"
								style={placed.style}
								initial={placed.hidden}
								animate={placed.shown}
								exit={placed.hidden}
								transition={{ duration: 0.15 }}
							>
								{tooltip}
							</motion.div>
						);
					})()}
			</AnimatePresence>
		</div>
	);

	// Render a status widget by ID
	const renderStatusWidget = (id: string) => {
		switch (id) {
			case "cpu":
				return statusWidget(
					"cpu",
					"CPU Usage",
					<>
						<Cpu size={12} strokeWidth={2} />
						<span className="label">
							{cpuUsage}
							<span className="unit">%</span>
						</span>
					</>
				);
			case "disk":
				return statusWidget(
					"disk",
					"Free Disk Space",
					<>
						<HardDrive size={12} strokeWidth={2} />
						<span className="label">{diskSpace}GB</span>
					</>
				);
			case "net":
				return statusWidget(
					"net",
					"Network Speed",
					integrated ? (
						// Stacked like MyFinder's own two-value readouts: on one line the
						// strip ran into the bar's tray icons.
						<span className="net-stack">
							<ArrowUp size={9} strokeWidth={2.6} />
							<span>{formatBytes(netUpSpeed)}</span>
							<ArrowDown size={9} strokeWidth={2.6} />
							<span>{formatBytes(netDownSpeed)}</span>
						</span>
					) : (
						<>
							<ArrowUpDown size={12} strokeWidth={2} />
							<span className="label">
								↑<span className="net-speed">{formatBytes(netUpSpeed)}</span>{" "}
								↓<span className="net-speed">{formatBytes(netDownSpeed)}</span>
							</span>
						</>
					)
				);
			default:
				return null;
		}
	};

	const formatBytes = (bytes: number) => {
		if (bytes < 1024) return `${bytes}B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}K`;
		return `${(bytes / (1024 * 1024)).toFixed(1)}M`;
	};

	// Music mode shows any time we have media info (playing or paused) and music mode setting is enabled
	const isMusicMode = mediaInfo.has_media && bloomMode === "music" && settingsMusicModeEnabled;
	// A media session stays reachable on hover after its activity fades, or
	// whenever the compact music display is disabled. The collapsed row then
	// shows the music view again: waveform left, album art right.
	const showMusicOnStatusHover =
		!isOpen &&
		isHovered &&
		bloomMode === "status" &&
		settingsMusicModeEnabled &&
		mediaInfo.has_media &&
		(!musicActivityVisible || !settingsMusicCompactNotch);
	const showCompactMusicIcons = musicActivity || showMusicOnStatusHover;
	const quotaIndex = hoverAgent ? shownQuotaReadings.findIndex((a) => a.id === hoverAgent.id) : -1;
	const quotaPlace =
		hoverAgent && shownQuotaReadings.length > 1 && quotaIndex >= 0
			? `${quotaIndex + 1} of ${shownQuotaReadings.length}`
			: undefined;
	const quotaBadge =
		hoverAgent !== undefined ? (
			<button
				key="quota-badge"
				type="button"
				className="notch-quota-badge"
				data-testid="notch-hover-quota"
				aria-label={`${agentName(hoverAgent.id)}. Show next agent`}
				tabIndex={-1}
				onMouseDown={(e) => e.preventDefault()}
				onClick={(e) => {
					e.stopPropagation();
					const next = nextAgent(shownQuotaReadings, hoverAgent.id);
					if (next !== undefined) setQuotaSelected(next);
				}}
				onMouseEnter={(e) => !integrated && showStatusTooltip("quota-hover", e.currentTarget)}
				onMouseLeave={hideStatusTooltip}
			>
				<AnimatePresence mode="popLayout" initial={false}>
					<motion.span
						key={hoverAgent.id}
						className="notch-quota-mark"
						initial={{ opacity: 0, scale: 0.92 }}
						animate={{ opacity: 1, scale: 1 }}
						exit={{ opacity: 0, scale: 0.92 }}
						transition={
							reduceMotion ? { duration: 0 } : { duration: 0.22, ease: [0.16, 1, 0.3, 1] }
						}
					>
						{integrated ? (
							<span className="quota-chip-mark">
								<AgentMark id={hoverAgent.id} />
							</span>
						) : (
							<QuotaRingMark a={hoverAgent} />
						)}
					</motion.span>
				</AnimatePresence>
				<AnimatePresence>
					{!integrated && statusTooltip?.id === "quota-hover" &&
						(() => {
							const placed = statusTooltipMotion(statusTooltip.side, statusTooltip.offset);
							return (
								<motion.div
									className="status-tooltip quota-hover"
									style={placed.style}
									initial={placed.hidden}
									animate={placed.shown}
									exit={placed.hidden}
									transition={{ duration: 0.15 }}
								>
									<QuotaHoverCard a={hoverAgent} place={quotaPlace} />
								</motion.div>
							);
						})()}
				</AnimatePresence>
			</button>
		) : null;

	// The compact quota space is the notch row itself: the agent marks take the
	// place of the CPU/network widgets, the clock stays beside them, and the
	// two controls past it expand the space or hold it on screen. Nothing
	// renders below, so pinning it costs no extra height.
	const quotaStripActive =
		shownQuotaReadings.length > 0 &&
		((bloomMode === "quotas" && quotaCompact) ||
			(quotaPinned && (quotaCompact || bloomMode !== "quotas")));
	// The strip and the collapsed badge share hoverQuotaAgent, which also
	// drops a selection that is no longer shown, so both highlight one agent.
	const stripSelectedId = hoverAgent?.id;
	const stripRef = useRef<HTMLDivElement>(null);
	const stripPad = useSlidingPad(stripRef, stripSelectedId, [
		shownQuotaReadings.length,
		quotaStripActive,
		integrated
	]);
	const restingStripWidthRef = useRef<number>((() => {
		const saved = Number(localStorage.getItem("bloom-resting-strip-width"));
		return Number.isFinite(saved) && saved >= 120 ? saved : 280;
	})());
	// In MyFinder's bar the strip may only use the width between MyFinder's own
	// menu and the clock. Its right edge stays where the resting strip ends, so
	// the room is what lies between that edge and the measured menu, less the
	// rest of the row, the hover controls and a gap.
	const stripMenuRight =
		integrated && myfinder.menuRight > 0
			? (myfinder.menuRight / displayDpi - (window.screen.width - window.innerWidth) / 2) / scale
			: 0;
	const stripRoom =
		stripMenuRight > 0
			? (window.innerWidth / scale + restingStripWidthRef.current) / 2 -
				stripMenuRight -
				STRIP_MENU_GAP -
				stripOther -
				STRIP_CONTROLS_WIDTH
			: Infinity;
	const stripFit = useMemo(
		() => fitQuotaStrip(shownQuotaReadings, stripRoom),
		[shownQuotaReadings, stripRoom]
	);
	const stripAgents = integrated ? stripFit.shown : shownQuotaReadings;
	const openQuotaPanel = () => {
		setIsOpen(true);
		setBloomMode("quotas");
		handleQuotaCompact(false);
	};
	const quotaStripControls = quotaStripActive ? (
		<span className="notch-quota-controls">
			<button
				type="button"
				className="notch-quota-ctl"
				tabIndex={-1}
				onMouseDown={(e) => e.preventDefault()}
				onClick={(e) => {
					e.stopPropagation();
					openQuotaPanel();
				}}
				aria-label="Expand quota space"
				data-testid="notch-quota-expand"
			>
				<Maximize2 size={12} />
			</button>
			<button
				type="button"
				className={`notch-quota-ctl${quotaPinned ? " active" : ""}`}
				tabIndex={-1}
				onMouseDown={(e) => e.preventDefault()}
				onClick={(e) => {
					e.stopPropagation();
					handleQuotaPin(!quotaPinned);
				}}
				aria-label={quotaPinned ? "Unpin quota space" : "Pin quota space"}
				aria-pressed={quotaPinned}
				data-testid="notch-quota-pin"
			>
				<Pin size={12} />
			</button>
		</span>
	) : null;
	const quotaStripMarks = quotaStripActive ? (
		<div
			className={`notch-quota-strip${integrated ? " quota-housing" : ""}`}
			data-stage={integrated ? stripFit.stage : undefined}
			data-testid="notch-quota-strip"
			ref={stripRef}
			// The housing between and around the agents opens the panel, like
			// the body of the resting pill does.
			onClick={
				integrated
					? (e) => {
							if (e.target !== e.currentTarget) return;
							e.stopPropagation();
							openQuotaPanel();
						}
					: undefined
			}
		>
			{!integrated && <div className="qs-pad" {...stripPad.padProps} />}
			{integrated && stripFit.stage !== "dot" && quotaStripControls}
			{stripAgents.map((a) => (
				<button
					key={a.id}
					type="button"
					className={`notch-quota-mark${integrated ? " quota-seg" : ""}${a.id === stripSelectedId ? " selected" : ""}`}
					data-state={a.state}
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					onClick={(e) => {
						e.stopPropagation();
						setQuotaSelected(a.id);
					}}
					onMouseEnter={(e) => showStatusTooltip(a.id, e.currentTarget, true)}
					onMouseLeave={hideStatusTooltip}
					aria-label={agentName(a.id)}
					data-testid={`notch-quota-mark-${a.id}`}
				>
					<QuotaRingMark a={a} />
					{integrated && stripShowsValue(a, stripFit.stage) && (
						<span
							className="quota-seg-value"
							data-band={stripHeadline(a) === undefined ? undefined : band(stripHeadline(a)!)}
						>
							{stripHeadline(a) ?? "—"}
							{stripHeadline(a) !== undefined && <span className="unit">%</span>}
						</span>
					)}
					{integrated && stripFit.stage === "full" && stripSecond(a) !== undefined && (
						<span className="quota-seg-second">{stripSecond(a)}</span>
					)}
					<AnimatePresence>
						{statusTooltip?.id === a.id &&
							(() => {
								const placed = statusTooltipMotion(statusTooltip.side, statusTooltip.offset);
								return (
									<motion.div
										className="status-tooltip quota-hover"
										style={placed.style}
										initial={placed.hidden}
										animate={placed.shown}
										exit={placed.hidden}
										transition={{ duration: 0.15 }}
										onUpdate={integrated ? () => reportNotchRect.current() : undefined}
									>
										<QuotaHoverCard a={a} />
									</motion.div>
								);
							})()}
					</AnimatePresence>
				</button>
			))}
			{integrated && stripFit.hidden > 0 && (
				<button
					type="button"
					className={stripFit.stage === "dot" ? "quota-dot" : "quota-seg quota-seg-more"}
					data-band={worstBand(shownQuotaReadings)}
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					onClick={(e) => {
						e.stopPropagation();
						openQuotaPanel();
					}}
					aria-label={`Open quota space. ${stripFit.hidden} more agents`}
					data-testid="notch-quota-more"
				>
					{stripFit.stage === "dot" ? null : `+${stripFit.hidden}`}
				</button>
			)}
		</div>
	) : null;

	// Calculate width dynamically based on enabled features
	const getRestingWidth = () => {
		if (isTimerMode) return integrated ? 360 : 300;
		if (bloomMode === "shelf") return 380;
		// The quota space sizes to its ring row: the standalone widget's 386
		// minimum, grown to the measured cell content for all seven agents.
		// An active compact strip measures the notch row itself instead.
		if (bloomMode === "quotas") {
			if (quotaStripActive) return clusterWidth > 0 ? clusterWidth : 220;
			return integrated ? (quotaCompact ? 360 : 480) : Math.max(386, Math.ceil(quotaSpaceWidth));
		}
		if (bloomMode === "overview") return 350;
		if (isMusicMode && isOpen) return integrated ? 360 : mediaLayout === "compact" ? 300 : 340;
		if ((showPowerPulse || showLowBatteryPulse || showUpdatePulse) && !isHovered) return 200;
		if (!isOpen && (bloomMode === "status" || bloomMode === "music")) {
			if (clusterWidth > 0) return clusterWidth;
			return 140;
		}

		return 140;
	};

	const getRestingHeight = () => {
		if (!isExpanded || !isVisible || isHidden) {
			return isImpacted ? 28.9 : 44.2;
		}
		// Announcement card: body is line-clamped, so a fixed size fits both cases.
		// MyFinder: the body measures its own content, header and tabs included.
		if (bloomMode === "timer") return integrated ? finderBodyHeight || 260 : 236;
		// Shelf expands on mode alone (no hover needed: there is no cursor
		// hover during an OS drag) and is sized to exactly one row of tiles,
		// plus room for the scrollbar once the tiles overflow it.
		if (bloomMode === "shelf") {
			if (shelfItems.length === 0) return 130;
			return shelfOverflowing ? 190 : 176;
		}
		// A compact strip lives in the notch row and never grows below it. With
		// no readings to draw the full panel, empty hint included, takes over
		// and the space follows its measured content height (the window is
		// resized to match by set_quota_panel_height inside the panel).
		if (bloomMode === "quotas")
			return quotaStripActive ? 36 : Math.max(Math.ceil(quotaSpaceHeight), integrated && quotaCompact ? 120 : 180);
		if (bloomMode === "overview") return 300;
		if (bloomMode === "status") return 36;
		if (isMusicMode && isOpen) {
			// MyFinder: the body measures its own content (player, lyrics,
			// queue drawer), so the dropdown is exactly as tall as it needs.
			if (integrated && finderBodyHeight > 0) return finderBodyHeight;
			const hasProgressBar = (mediaInfo.duration_ms ?? 0) > 0;
			let h = mediaLayout === "compact" ? (hasProgressBar ? 132 : 116) : 120;
			if (mediaLayout === "compact") {
				// A lyric sheet grows the island once, to its fixed 52 px viewport.
				if ((mediaInfo.lyrics?.lines.length ?? 0) > 0) h += 58;
				if (compactVolumeExpanded) h += 36;
				// The stack reports its own whole height (cards plus any open
				// drawer), so it replaces the single card's drawer growth.
				if (mediaStackHeight > 0)
					h = Math.max(h, Math.min(mediaStackHeight, mediaAvailableHeight - 24));
				else if (compactQueueExpanded)
					h = Math.max(
						h,
						Math.min(
							Math.max(queueIslandHeight, queueFloor),
							h + QUEUE_MAX_HEIGHT,
							mediaAvailableHeight - 24
						)
					);
			}
			if (integrated) h += 76;
			return h;
		}
		return 36;
	};

	// Alerts grow the island around the same resting size: an urgent quota
	// strip needs room for its rows; a finished timer pulses wider like the
	// battery events. Never narrower than the panel underneath wanted.
	const getDynamicWidth = () => {
		const base = Math.max(getRestingWidth(), !integrated && quotaStripActive ? clusterWidth : 0);
		if (alertExpanded) return Math.max(base, quotaAlertActive ? 300 : 200);
		return base;
	};

	const getDynamicHeight = () => {
		const base = getRestingHeight();
		if (quotaAlertActive) return Math.max(base, 44 + alertRows(shownQuotaReadings).length * 20);
		return base;
	};

	const isTimerMode = bloomMode === "timer";
	// The stack replaces the single card only for two or more playing players.
	const isStackMode = mediaCards.length >= 2 && mediaLayout === "compact";
	const glowArt = isStackMode ? stackFocusArt : albumArtUrl;

	// Close compact media player expansions when notch is unhovered or mode changes
	useEffect(() => {
		setCompactVolumeExpanded(false);
		setCompactQueueExpanded(false);
	}, [isHovered, isOpen, mediaLayout, bloomMode]);

	// Each drawer opening measures afresh; a hold never outlives the drawer.
	useEffect(() => {
		if (compactQueueExpanded) return;
		setQueueFloor(0);
		queueIslandHeightRef.current = Number.POSITIVE_INFINITY;
	}, [compactQueueExpanded]);

	// Falling back to the single card must drop the stack's height at once,
	// or the island would stay tall for a stack that is no longer rendered.
	useEffect(() => {
		if (mediaCards.length < 2) {
			setMediaStackHeight(0);
			setStackFocusArt(null);
			setStackFocusHeight(0);
		}
	}, [mediaCards.length]);

	useEffect(() => {
		if (!isOpen || !isMusicMode || mediaLayout !== "compact" || !compactQueueExpanded) return;
		let disposed = false;
		const resize = () => {
			void invoke<number>("media_panel_bounds", {
				height: ((mediaInfo.duration_ms ?? 0) > 0 ? 132 : 116) + QUEUE_MAX_HEIGHT + 24
			})
				.then((height) => {
					if (!disposed && Number.isFinite(height)) setMediaAvailableHeight(height);
				})
				.catch(() => {});
		};
		resize();
		window.addEventListener("resize", resize);
		return () => {
			disposed = true;
			window.removeEventListener("resize", resize);
			void invoke("media_panel_bounds", { height: 0 }).catch(() => {});
		};
	}, [isOpen, isMusicMode, mediaLayout, compactQueueExpanded, mediaInfo.duration_ms]);

	const canRecordRestingWidth =
		integrated &&
		!quotaStripActive &&
		bloomMode === "status" &&
		!showCompactMusicIcons &&
		!showPowerPulse &&
		!showLowBatteryPulse &&
		!showUpdatePulse &&
		clusterWidth > 0;
	if (canRecordRestingWidth) {
		restingStripWidthRef.current = clusterWidth;
	}
	useEffect(() => {
		if (canRecordRestingWidth) {
			localStorage.setItem("bloom-resting-strip-width", String(clusterWidth));
		}
	}, [canRecordRestingWidth, clusterWidth]);

	// The pill's target geometry: the animation springs toward it, and the
	// window region below is cut from it, so the region is already at the
	// final size while the pill animates.
	const stripHeight = integrated ? fittedBarHeight(myfinder.height, displayDpi, scale) : 36;
	const bodyTop = stripHeight + 8;
	const suppressEmptyPinnedDropdown =
		integrated &&
		quotaCompact &&
		quotaPinned &&
		bloomMode === "quotas" &&
		!hasQuotaSnapshot &&
		shownQuotaReadings.length === 0;
	const quotaDropdownSuppressed =
		bloomMode === "quotas" && (quotaStripActive || suppressEmptyPinnedDropdown);
	const finderPanelOpen =
		integrated && ((isOpen && !quotaDropdownSuppressed) || quotaAlertActive);
	const windowCssWidth = window.innerWidth / scale;
	const stripWidth = Math.min(clusterWidth || 220, windowCssWidth - 16);
	const centeredStripLeft = (windowCssWidth - stripWidth) / 2;
	const desiredStripShiftLeft =
		// Negative when the strip is narrower than the resting one: its right
		// edge stays put either way, so it only ever gives width back on the
		// menu's side.
		integrated && quotaStripActive ? (stripWidth - restingStripWidthRef.current) / 2 : 0;
	const stripShiftLeft = Math.min(desiredStripShiftLeft, Math.max(0, centeredStripLeft - 8));
	const pillTargetY = !isReady ? 250 : isVisible ? (isHidden ? -100 : integrated ? bodyTop : 0) : -150;
	const standaloneWidth = !isReady
		? 34
		: isExpanded && isVisible && !isHidden
			? getDynamicWidth()
			: isImpacted
				? 39.1
				: 30.6;
	const pillTargetWidth = integrated ? Math.min(Math.max(320, getDynamicWidth()), window.innerWidth / scale - 32) : standaloneWidth;
	const withoutStatusRow =
		bloomMode === "music" || bloomMode === "quotas" || bloomMode === "timer" || quotaAlertActive
			? 0
			: 36;
	const pillTargetHeight = integrated
		// +4: the panel's two 1px borders plus rounding. Its content is sized without
		// them, so without the allowance panels overflowed by a pixel and drew a scrollbar.
		? finderPanelOpen ? Math.min(Math.max(64, getDynamicHeight() - withoutStatusRow) + 4, window.innerHeight / scale - bodyTop - 24) : 0
		: !isReady ? 34 : getDynamicHeight();
	barConstraint.current = integrated ? {
		// The bar row and the hanging badge are input and paint; the rest of the
		// gap below stays outside the region (the native hover check bridges it),
		// or it would show as a haze.
		strip: { x: (centeredStripLeft - stripShiftLeft) * scale, y: 0, width: stripWidth * scale, height: bodyTop * scale },
		boundary: bodyTop * scale
	} : null;
	// Shadow and the edge flares beside the pill.
	pillTargetRef.current = notchPillBox(
		window.innerWidth,
		scale,
		pillTargetY,
		pillTargetWidth,
		pillTargetHeight,
		integrated ? 16 : 24
	);
	if (integrated && !finderPanelOpen) pillTargetRef.current = null;

	// Clip the notch window to what it draws (see window_region.rs): outside
	// these rectangles the window takes no clicks, whatever its click-through
	// state. Sent after every render, but only when the region changes.
	useEffect(() => {
		if (windowLabel === "main") updateNotchRegion();
	});

	const statusRow = (
// Standalone mode has no clock button, so clicking anywhere on the row outside its buttons opens or cycles panels.
<div className="main-row" onClick={!integrated ? cycleTimePanel : undefined}>
												<AnimatePresence mode="wait">
													{(showPowerPulse || showLowBatteryPulse || showUpdatePulse) &&
													!isHovered ? (
														showUpdatePulse ? (
															<motion.div
																key="update-pulse-view"
																initial={{ opacity: 0, scale: 0.95, filter: "blur(4px)" }}
																animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
																exit={{ opacity: 0, scale: 1.05, filter: "blur(4px)" }}
																className="power-pulse-content"
															>
																<GreenDownArrowIcon />
																<span className="label" style={{ color: "#32D74B" }}>
																	Update Available
																</span>
															</motion.div>
														) : (
															<motion.div
																key="pulse-view"
																initial={{ opacity: 0, scale: 0.95, filter: "blur(4px)" }}
																animate={{ opacity: 1, scale: 1, filter: "blur(0px)" }}
																exit={{ opacity: 0, scale: 1.05, filter: "blur(4px)" }}
																className="power-pulse-content"
															>
																{updateAvailable && <GreenDownArrowIcon />}
																<BatteryIcon
																	charging={isCharging}
																	level={batteryLevel}
																	threshold={lowBatteryThreshold}
																/>
																<span
																	className="label"
																	style={{ color: showLowBatteryPulse ? "#FF453A" : "inherit" }}
																>
																	{showLowBatteryPulse
																		? "Low Battery"
																		: isCharging
																			? "Charging"
																			: "On Battery"}{" "}
																	• {batteryLevel}%
																</span>
															</motion.div>
														)
													) : (
														<motion.div
															key="standard-view"
															ref={pillClusterRef}
															className={`main-row-inner pill-cluster${quotaStripActive ? " has-quota-strip" : ""}`}
															initial={{ opacity: 0 }}
															animate={{ opacity: 1 }}
															exit={{ opacity: 0 }}
														>
															{showCompactMusicIcons && settingsVisualizerEnabled && (
																<div className="pill-activity">
																	<Visualizer isPlaying={isPlaying} />
																</div>
															)}
															{integrated && showShelfBubble && <button type="button" className="pill-activity-button"
																tabIndex={-1} onMouseDown={(e) => e.preventDefault()} aria-label={`Open shelf: ${shelfItems.length} items`}
																onClick={(e) => { e.stopPropagation(); openNotch("shelf"); }}><Inbox size={12} /><span>{shelfItems.length}</span></button>}
															{integrated && showMusicBubble && <button type="button" className="pill-activity-button"
																tabIndex={-1} onMouseDown={(e) => e.preventDefault()} aria-label="Open music"
																onClick={(e) => { e.stopPropagation(); openNotch("music"); }}>{albumArtUrl ? <img src={albumArtUrl} alt="" /> : <Music size={14} />}</button>}
															{integrated && hoverAgent && !quotaStripActive ? (
																<QuotaChip a={hoverAgent}>{quotaBadge}</QuotaChip>
															) : (
																(quotaStripMarks ?? quotaBadge)
															)}
															{quotaStripMarks === null &&
																!showCompactMusicIcons &&
																statusWidgets.left.length > 0 && (
																	<div className="passive-features-group">
																		{statusWidgets.left.map(renderStatusWidget)}
																	</div>
																)}
															<div className="time-center" ref={timeCenterRef}>
																<div className="time-flip-container" onClick={cycleTimePanel}>
																	<AnimatePresence initial={false}>
																		{timerSeconds > 0 || isTimerFinished ? (
																			<motion.span
																				key="timer"
																				className={`time compact-timer ${isTimerFinished ? "timer-finished" : ""}`}
																				initial={{ rotateX: -90, opacity: 0 }}
																				animate={{ rotateX: 0, opacity: 1 }}
																				exit={{ rotateX: 90, opacity: 0 }}
																				transition={{ type: "spring", stiffness: 600, damping: 30 }}
																			>
																				{isTimerFinished && (
																					<BellRing
																						size={13}
																						strokeWidth={2.5}
																						className="timer-bell"
																					/>
																				)}
																				{formatTimerTime(timerSeconds)}
																			</motion.span>
																		) : null}
																	</AnimatePresence>
																</div>
															</div>
															{!integrated && quotaStripControls}
															{showCompactMusicIcons &&
															(settingsAlbumArtEnabled || showMusicOnStatusHover) ? (
																<AnimatePresence mode="wait">
																	<motion.div
																		key="album-art"
																		className="album-art-glow-wrapper"
																		initial={{ opacity: 0, scale: 0.8 }}
																		animate={{ opacity: 1, scale: 1 }}
																		exit={{ opacity: 0, y: -20, scale: 0.8, filter: "blur(8px)" }}
																		transition={{ duration: 0.12 }}
																	>
																		{settingsAlbumArtEnabled &&
																			albumArtUrl &&
																			settingsCompactGlowEnabled && (
																				<img
																					src={albumArtUrl}
																					alt=""
																					className="album-art-glow-bg"
																					draggable={false}
																				/>
																			)}
																		<button
																			className={`album-art album-art-activity${!isPlaying ? " paused" : ""}`}
																			aria-label="Open music"
																			onClick={(e) => {
																				e.stopPropagation();
																				openNotch("music");
																			}}
																			onContextMenu={(e) => {
																				e.preventDefault();
																				e.stopPropagation();
																				skipPrevious();
																			}}
																		>
																			<div className="album-art-inner">
																				<AnimatePresence mode="wait" initial={false}>
																					{settingsAlbumArtEnabled && albumArtUrl ? (
																						<motion.img
																							key={`compact-art-${albumArtKey}`}
																							src={albumArtUrl}
																							alt="Art"
																							draggable={false}
																							initial={{ rotateY: 90, opacity: 0 }}
																							animate={{ rotateY: 0, opacity: 1 }}
																							exit={{ rotateY: -90, opacity: 0 }}
																							transition={{ duration: 0.25, ease: "easeInOut" }}
																							style={{
																								width: "100%",
																								height: "100%",
																								objectFit: "cover"
																							}}
																						/>
																					) : (
																						<motion.div
																							key="compact-placeholder"
																							className="album-art-placeholder"
																							initial={{ rotateY: 90, opacity: 0 }}
																							animate={{ rotateY: 0, opacity: 1 }}
																							exit={{ rotateY: -90, opacity: 0 }}
																							transition={{ duration: 0.25, ease: "easeInOut" }}
																						>
																							<MusicNoteIcon className="music-placeholder-svg-small" />
																						</motion.div>
																					)}
																				</AnimatePresence>
																			</div>
																		</button>
																	</motion.div>
																</AnimatePresence>
															) : quotaStripMarks === null && !showCompactMusicIcons && statusWidgets.right.length > 0 ? (
																<div className="passive-features-group">
																	{statusWidgets.right.map(renderStatusWidget)}
																</div>
															) : null}
															{updateAvailable && showUpdateIndicator && (
																<div className="live-dot" />
															)}
														</motion.div>
													)}
												</AnimatePresence>
											</div>
	);

	const renderIntegratedChrome = (title: string, active: "music" | "timer") => (
		<>
			<div className="qs-header">
				<span className="qs-menu-title">{title}</span>
				<span className="qs-header-actions">
					<button
						type="button"
						className="qs-icon-btn"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						onClick={(e) => {
							e.stopPropagation();
							closeQuotaSpace();
						}}
						aria-label="Close"
					>
						<X size={12} />
					</button>
				</span>
			</div>
			<div className="qs-menu-nav navigation" aria-label="Panels">
				<button
					type="button"
					className="qs-menu-nav-tab"
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					aria-pressed="false"
					onClick={(e) => {
						e.stopPropagation();
						if (quotaPinned && quotaCompact) {
							handleQuotaCompact(false);
						}
						setBloomMode("quotas");
					}}
				>
					Quotas
				</button>
				<button
					type="button"
					className="qs-menu-nav-tab"
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					aria-pressed={active === "music"}
					disabled={!(settingsMusicModeEnabled && mediaInfo.has_media)}
					onClick={(e) => {
						e.stopPropagation();
						setBloomMode("music");
					}}
				>
					Music
				</button>
				<button
					type="button"
					className="qs-menu-nav-tab"
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					aria-pressed={active === "timer"}
					onClick={(e) => {
						e.stopPropagation();
						setBloomMode("timer");
					}}
				>
					Timer
				</button>
			</div>
		</>
	);

	return (
		<div className={`screen${integrated ? ` myfinder-integrated ${finderMaterial.light ? "myfinder-light" : "myfinder-dark"}` : ""}`} style={{ overflow: "hidden", ...(integrated ? finderMaterial.style : {}) } as React.CSSProperties}>
			{/* Screen Corners (Top) */}
			<AnimatePresence>
				{screenFree && settingsCornersEnabled && (
					<>
						<motion.div
							className="screen-corner top-left"
							initial={{ opacity: 0 }}
							animate={{ opacity: 1, filter: "blur(0px)" }}
							exit={{ opacity: 0, filter: "blur(10px)" }}
						/>
						<motion.div
							className="screen-corner top-right"
							initial={{ opacity: 0 }}
							animate={{ opacity: 1, filter: "blur(0px)" }}
							exit={{ opacity: 0, filter: "blur(10px)" }}
						/>
					</>
				)}
			</AnimatePresence>

			<div
				onMouseEnter={integrated ? () => { releaseDeferredRef.current = false; cancelFinderLeave(); setIsNotchHovered(true); setIsHovered(true); } : undefined}
				onMouseLeave={integrated ? releaseFinderHoverSoon : undefined}
				style={{
					zoom: scale,
					width: "100%",
					display: "flex",
					justifyContent: "center",
					position: "relative"
				}}
			>
				{integrated && (
					<motion.div
						ref={finderStripRef}
						className="myfinder-status"
						style={{ width: stripWidth, height: bodyTop, x: "-50%", marginLeft: -stripShiftLeft, "--myfinder-bar-height": `${stripHeight}px`, "--myfinder-native-height": `${myfinder.height / (displayDpi * scale)}px` } as React.ComponentProps<typeof motion.div>["style"]}
						initial={false}
						animate={{ y: isReady && isVisible && !isHidden ? 0 : -stripHeight - 12, opacity: isReady && isVisible && !isHidden ? 1 : 0 }}
						transition={{ duration: reduceMotion ? 0 : 0.14 }}
						onUpdate={() => reportNotchRect.current()}
						onMouseEnter={() => { releaseDeferredRef.current = false; setIsNotchHovered(true); setIsHovered(true); }}
						onClick={() => { if (!finderPanelOpen) openNotch(nextOpeningPanel()); else closeQuotaSpace(); }}
						onWheel={handleWheel}
					>
						{statusRow}
					</motion.div>
				)}

				<motion.div
					ref={bloomRef}
					className={`bloom ${integrated ? "myfinder-panel" : ""} ${isHovered ? "expanded" : ""} ${isImpacted ? "is-impacted" : ""} ${isDragHovering ? "drag-hover" : ""} ${isOpen && !quotaDropdownSuppressed ? "is-open" : ""}`}
					onMouseEnter={() => setIsNotchHovered(true)}
					onPointerDown={compactQueueExpanded ? suppressNextHoverRelease : undefined}
					onMouseLeave={() => {
						setIsNotchHovered(false);
						queuePointerY.current = -1;
					}}
					onMouseMove={
						isOpen && isMusicMode && mediaLayout === "compact" ? trackQueuePointer : undefined
					}
					onWheel={handleWheel}
					initial={{
						y: 250,
						width: 30.6,
						height: 44.2,
						borderTopLeftRadius: 18,
						borderTopRightRadius: 18,
						borderBottomLeftRadius: 18,
						borderBottomRightRadius: 18,
						opacity: 0
					}}
					animate={{
						y: pillTargetY,
						width: pillTargetWidth,
						height: pillTargetHeight,
						opacity: isVisible && (!integrated || finderPanelOpen) ? 1 : 0,
						borderTopLeftRadius: isImpacted ? 0 : 18,
						borderTopRightRadius: isImpacted ? 0 : 18,
						borderBottomLeftRadius: isTimerMode ? 28 : (isOpen && !quotaDropdownSuppressed ? 24 : 18),
						borderBottomRightRadius: isTimerMode ? 28 : (isOpen && !quotaDropdownSuppressed ? 24 : 18),
						filter: isVisible ? "blur(0px)" : "blur(8px)",
						pointerEvents: isVisible && (!integrated || finderPanelOpen) ? "auto" : "none"
					}}
					onClick={(e) => {
						e.stopPropagation();
						if (!isOpen) openNotch(nextOpeningPanel());
					}}
					onHoverStart={() => {
						releaseDeferredRef.current = false;
						setIsHovered(true);
						// Hovering the shelf cancels its post-drop auto-close.
						if (bloomMode === "shelf") clearShelfCollapse();
					}}
					onHoverEnd={integrated ? undefined : releaseNotchHover}
					style={{ originY: 0, ...(integrated ? { overflow: "auto" } : {}) }}
					onUpdate={() => { if (integrated) reportNotchRect.current(); }}
					transition={integrated ? { duration: reduceMotion ? 0 : 0.2, ease: [0.22, 1, 0.36, 1] } : {
						width: { type: "spring", stiffness: 400, damping: 31 },
						height:
							isMusicMode && mediaLayout === "compact"
								? (reduceMotion ? { duration: 0 } : { type: "spring", stiffness: 360, damping: 28 })
								: { type: "spring", stiffness: 450, damping: 29 },
						y: { type: "spring", stiffness: 550, damping: 45, mass: 0.8, restDelta: 0.001 },
						opacity: { duration: 0.2 },
						borderTopLeftRadius: { type: "spring", stiffness: 1000, damping: 40 },
						borderTopRightRadius: { type: "spring", stiffness: 1000, damping: 40 },
						borderBottomLeftRadius: { type: "spring", stiffness: 1000, damping: 40 },
						borderBottomRightRadius: { type: "spring", stiffness: 1000, damping: 40 },
						default: { type: "spring", stiffness: 500, damping: 30, mass: 1 }
					}}
				>
					{!integrated && <EdgeSurface edge="top" flares={isImpacted} />}
					<AnimatePresence>
						{!integrated && isMusicMode && settingsAmbienceEnabled && glowArt && isOpen && !isTimerMode && (
							<motion.div
								className="notch-ambient-glow"
								initial={{ opacity: 0 }}
								animate={{ opacity: 1 }}
								exit={{ opacity: 0 }}
								transition={{ duration: 0.15 }}
							>
								<AnimatePresence mode="wait">
									<motion.img
										key={glowArt}
										src={glowArt}
										alt=""
										draggable={false}
										initial={{ opacity: 0, scale: 1.1 }}
										animate={{ opacity: 1, scale: 1.8 }}
										exit={{ opacity: 0 }}
										transition={{ duration: 0.3 }}
									/>
								</AnimatePresence>
								{/* The queue's small text needs a calm surface, so the glow stays behind the player. */}
								<div
									className={`notch-ambient-dim${isStackMode || (mediaLayout === "compact" && compactQueueExpanded) ? " is-active" : ""}`}
									style={
										{
											"--dim-from": isStackMode
												? `${Math.max(0, stackFocusHeight - 56)}px`
												: `${(mediaInfo.duration_ms ?? 0) > 0 ? 104 : 88}px`
										} as React.CSSProperties
									}
								/>
							</motion.div>
						)}
					</AnimatePresence>
					<AnimatePresence mode="wait">
						{isExpanded && (
							<motion.div
								key="bloom-content"
								initial={{ opacity: 0 }}
								animate={{ opacity: 1 }}
								exit={{ opacity: 0 }}
								transition={{ duration: 0.1 }}
								style={{
									width: "100%",
									height: "100%",
									display: "flex",
									flexDirection: "column",
									alignItems: "center",
									position: "relative",
									borderRadius: "inherit"
								}}
							>
								{/* Faster Waiting Transition Area */}
								<AnimatePresence mode="wait">
									{isOpen && isMusicMode && !isTimerMode ? (
										<motion.div
											key="expanded-music"
											className={integrated ? "myfinder-panel-body" : "expanded-music-container"}
											ref={integrated ? finderBodyMeasureRef : undefined}
											style={{ transformOrigin: "50% 0%" }}
											{...panelHandoff}
										>
											{integrated && renderIntegratedChrome("Music", "music")}
											<div
												className={integrated ? "expanded-music-container" : undefined}
												style={integrated ? undefined : { display: "contents" }}
											>
											{mediaLayout === "compact" ? (
												mediaCards.length >= 2 ? (
													<MediaStack
														cards={mediaCards}
														otherAudio={otherAudio}
														queueExpanded={compactQueueExpanded}
														onQueueExpandedChange={setCompactQueueExpanded}
														onHeightChange={setMediaStackHeight}
														maxHeight={mediaAvailableHeight - 24}
														volume={displayVolume}
														onVolumeChange={handleVolumeChange}
														onFocusArtworkChange={setStackFocusArt}
														onFocusHeightChange={setStackFocusHeight}
													/>
												) : (
													<CompactMediaPlayer
														mediaInfo={mediaInfo}
														albumArtUrl={albumArtUrl}
														albumArtKey={albumArtKey}
														isPlaying={isPlaying}
														volume={
															mediaSessionPid != null
																? mediaMuted
																	? 0
																	: mediaVolume
																: displayVolume
														}
														volumeExpanded={compactVolumeExpanded}
														queueExpanded={compactQueueExpanded}
														onQueueExpandedChange={(expanded) => {
															setCompactQueueExpanded(expanded);
															if (expanded) setCompactVolumeExpanded(false);
														}}
														onQueueHeightChange={reportQueueIslandHeight}
														onVolumeExpandedChange={(expanded) => {
															setCompactVolumeExpanded(expanded);
															if (expanded) setCompactQueueExpanded(false);
														}}
														onTogglePlayPause={togglePlayPause}
														onVolumeChange={
															mediaSessionPid != null ? handleMediaVolumeChange : handleVolumeChange
														}
														prevFront={prevFront}
														prevBack={prevBack}
														nextFront={nextFront}
														nextBack={nextBack}
														onAnimatePrev={animatePrev}
														onAnimateNext={animateNext}
														onLayoutChange={(layout) => {
															setMediaLayout(layout);
															localStorage.setItem("bloom-media-layout", layout);
															invoke("save_setting", {
																key: "bloom-media-layout",
																value: layout
															}).catch(console.error);
															window.dispatchEvent(
																new CustomEvent("settings-changed", {
																	detail: { key: "media-layout", value: layout }
																})
															);
														}}
													/>
												)
											) : (
												<div className="compact-premium-layout">
													<div className="album-art-section">
														<motion.div
															className="premium-album-art"
															whileHover={{ scale: 1.05 }}
															whileTap={{ scale: 0.95 }}
															onClick={(e) => {
																e.stopPropagation();
																setMediaLayout("compact");
																localStorage.setItem("bloom-media-layout", "compact");
																invoke("save_setting", {
																	key: "bloom-media-layout",
																	value: "compact"
																}).catch(console.error);
																window.dispatchEvent(
																	new CustomEvent("settings-changed", {
																		detail: { key: "media-layout", value: "compact" }
																	})
																);
															}}
															style={{ cursor: "pointer" }}
														>
															<AnimatePresence mode="wait" initial={false}>
																{albumArtUrl ? (
																	<motion.img
																		key={`art-${albumArtKey}`}
																		src={albumArtUrl}
																		alt="Art"
																		initial={{ rotateY: 90, opacity: 0 }}
																		animate={{ rotateY: 0, opacity: 1 }}
																		exit={{ rotateY: -90, opacity: 0 }}
																		transition={{ duration: 0.3, ease: "easeInOut" }}
																		style={{ width: "100%", height: "100%", objectFit: "cover" }}
																	/>
																) : (
																	<motion.div
																		key="placeholder"
																		className="art-placeholder-mini"
																		initial={{ rotateY: 90, opacity: 0 }}
																		animate={{ rotateY: 0, opacity: 1 }}
																		exit={{ rotateY: -90, opacity: 0 }}
																		transition={{ duration: 0.3, ease: "easeInOut" }}
																	>
																		<MusicNoteIcon size={32} className="music-placeholder-svg" />
																	</motion.div>
																)}
															</AnimatePresence>
														</motion.div>
													</div>

													<div className="metadata-controls-section-middle">
														<div className="track-header-row">
															<div className="track-info-middle">
																<TitleMarquee title={mediaInfo.title} />
																<span className="premium-artist">{mediaInfo.artist}</span>
															</div>
															<div className="header-visualizer-wrap">
																<div className="header-visualizer">
																	<Visualizer isPlaying={isPlaying} bars={5} height={18} />
																</div>
																<motion.button
																	className="classic-audio-output-btn"
																	onClick={(e) => {
																		e.stopPropagation();
																		invoke("open_sound_settings").catch(() => {});
																	}}
																	whileTap={{ scale: 0.9 }}
																	title="Audio Output"
																>
																	<HeadphonesIcon size={20} style={{ opacity: 0.5 }} />
																</motion.button>
															</div>
														</div>

														<div className="controls-row-sleek">
															<motion.button
																className="sleek-btn previous-btn"
																onClick={(e) => {
																	e.stopPropagation();
																	animatePrev();
																}}
																whileHover={{ scale: 1.2 }}
																whileTap={{ scale: 0.9 }}
																transition={{ type: "spring", stiffness: 400, damping: 25 }}
															>
																<div
																	style={{
																		position: "relative",
																		width: 32,
																		height: 16,
																		overflow: "hidden"
																	}}
																>
																	<motion.div
																		animate={prevBack}
																		style={{
																			position: "absolute",
																			inset: 0,
																			display: "flex",
																			alignItems: "center",
																			justifyContent: "center"
																		}}
																	>
																		<SkipBackIcon size={32} />
																	</motion.div>
																	<motion.div
																		animate={prevFront}
																		style={{
																			position: "absolute",
																			inset: 0,
																			display: "flex",
																			alignItems: "center",
																			justifyContent: "center"
																		}}
																	>
																		<SkipBackIcon size={32} />
																	</motion.div>
																</div>
															</motion.button>

															<motion.button
																className="sleek-btn play-pause-btn-floating"
																onClick={(e) => {
																	e.stopPropagation();
																	togglePlayPause();
																}}
																whileHover={{ scale: 1.2 }}
																whileTap={{ scale: 0.95 }}
																transition={{ type: "spring", stiffness: 400, damping: 25 }}
															>
																<AnimatePresence mode="wait" initial={false}>
																	<motion.div
																		key={isPlaying ? "pause" : "play"}
																		initial={{ opacity: 0, scale: 0.8 }}
																		animate={{ opacity: 1, scale: 1 }}
																		exit={{ opacity: 0, scale: 0.8 }}
																		transition={{ duration: 0.15 }}
																		style={{
																			display: "flex",
																			alignItems: "center",
																			justifyContent: "center"
																		}}
																	>
																		{isPlaying ? <PauseIcon size={26} /> : <PlayIcon size={26} />}
																	</motion.div>
																</AnimatePresence>
															</motion.button>

															<motion.button
																className="sleek-btn next-btn"
																onClick={(e) => {
																	e.stopPropagation();
																	animateNext();
																}}
																whileHover={{ scale: 1.2 }}
																whileTap={{ scale: 0.9 }}
																transition={{ type: "spring", stiffness: 400, damping: 25 }}
															>
																<div
																	style={{
																		position: "relative",
																		width: 32,
																		height: 16,
																		overflow: "hidden"
																	}}
																>
																	<motion.div
																		animate={nextBack}
																		style={{
																			position: "absolute",
																			inset: 0,
																			display: "flex",
																			alignItems: "center",
																			justifyContent: "center"
																		}}
																	>
																		<SkipForwardIcon size={32} />
																	</motion.div>
																	<motion.div
																		animate={nextFront}
																		style={{
																			position: "absolute",
																			inset: 0,
																			display: "flex",
																			alignItems: "center",
																			justifyContent: "center"
																		}}
																	>
																		<SkipForwardIcon size={32} />
																	</motion.div>
																</div>
															</motion.button>
														</div>

														<div className="volume-slider-container">
															<VolumeLowIcon size={12} style={{ opacity: 0.5 }} />
															<div className="slider-track-premium">
																<input
																	type="range"
																	min="0"
																	max="1"
																	step="0.01"
																	value={displayVolume}
																	onChange={(e) => handleVolumeChange(parseFloat(e.target.value))}
																	onPointerDown={(e) => e.stopPropagation()}
																	onClick={(e) => e.stopPropagation()}
																	className="premium-slider"
																/>
																<div
																	className="slider-progress-fill"
																	style={{ width: `${displayVolume * 100}%` }}
																/>
															</div>
															<VolumeHighIcon size={14} style={{ opacity: 0.5 }} />
														</div>
													</div>
												</div>
											)}
											</div>
										</motion.div>
									) : (
										<motion.div
											key="standard-view-group"
											initial={{ opacity: 0, y: -5 }}
											animate={{ opacity: 1, y: 0 }}
											exit={{ opacity: 0, y: 5, transition: { duration: 0.1 } }}
											transition={{ duration: 0.2 }}
											style={{ width: "100%" }}
										>
											{!integrated && statusRow}
										</motion.div>
									)}
								</AnimatePresence>

								{/* Agent Quotas panel, cycled like the other notch panels.
								    An active compact strip draws the marks in the notch row
								    instead; with no readings it leaves this panel, and its
								    empty hint, on screen. */}
								<AnimatePresence>
									{bloomMode === "quotas" && !quotaStripActive && !suppressEmptyPinnedDropdown && (
										<motion.div
											key="quotas-view"
											// The island is the width contract: `.qs` reports its
											// ring-row need through scrollWidth while this wrapper
											// and the viewport stay clamped to the pill.
											style={{ transformOrigin: "50% 0%", width: "100%", minWidth: 0 }}
											{...panelHandoffUnscaled}
										>
											<div ref={quotaSpaceMeasureRef} className="quota-viewport">
												<QuotaPanel
													menu={integrated}
													compact={integrated && quotaCompact}
													// bar + gap above the panel, then 24 of room below it plus its 1px borders and rounding.
													menuReserve={(bodyTop + 32) * scale}
													readings={shownQuotaReadings}
													selected={quotaSelected}
													onSelect={setQuotaSelected}
													pinned={quotaPinned}
													onPin={handleQuotaPin}
													onCompact={() => handleQuotaCompact(true)}
													onExpand={() => handleQuotaCompact(false)}
													onClose={closeQuotaSpace}
													onModeChange={(mode) => setBloomMode(mode)}
													musicAvailable={settingsMusicModeEnabled && mediaInfo.has_media}
													emptyHint={
														quotaHiddenAgents.length > 0 && shownQuotaReadings.length === 0
															? "All agents are hidden in Settings."
															: undefined
													}
												/>
											</div>
										</motion.div>
									)}
								</AnimatePresence>

								{/* Overview panel: the live activities, cycled like the other notch panels */}
								<AnimatePresence>
									{bloomMode === "overview" && (
										<motion.div
											key="overview-view"
											initial={{ opacity: 0 }}
											animate={{ opacity: 1 }}
											exit={{ opacity: 0, filter: "blur(4px)", transition: { duration: 0.1 } }}
											transition={{ type: "spring", stiffness: 400, damping: 30 }}
										>
											<OverviewPanel
												rows={overviewRows({
													music: settingsMusicModeEnabled && mediaInfo.has_media,
													timer: timerActive,
													shelfCount: shelfItems.length,
													urgent: shownQuotaReadings.some(urgent)
												})}
												onOpen={(row) =>
													openNotch(
														row === "Music"
															? "music"
															: row === "Timer"
																? "timer"
																: row === "Shelf"
																	? "shelf"
																	: "quotas"
													)
												}
											/>
										</motion.div>
									)}
								</AnimatePresence>

								{/* Calendar & Timer Split View */}
								<AnimatePresence>
									{isTimerMode && (
										<motion.div
											className={integrated ? "myfinder-panel-body" : "calendar-timer-content"}
											ref={integrated ? finderBodyMeasureRef : undefined}
											onClick={(e) =>
												e.stopPropagation()
											} /* Block mode switches when clicking inside */
											style={{ transformOrigin: "50% 0%" }}
											{...panelHandoff}
										>
											{integrated && renderIntegratedChrome("Timer", "timer")}
											<div className="timer-column">
												<div className={`timer-section-new state-${timerState}`}>
													<div className="timer-main">
														<div className="timer-status">
															{isEditingTimer
																? "Set duration"
																: isTimerFinished
																	? "Time's up"
																	: timerEndTime
																		? `${timerState === "paused" ? "Paused · " : ""}ends ${timerEndTime}`
																		: "Click to edit"}
														</div>

														<div className={`timer-clock-row ${isEditingTimer ? "editing" : ""}`}>
															<div
																className={`timer-clock ${isEditingTimer && !timerEditValid ? "invalid" : ""}`}
																onClick={beginTimerEdit}
																title={isTimerRunning ? undefined : "Click to set a duration"}
															>
																{minuteDigitItems.map(({ digit, key }) => (
																	<RollDigit key={key} value={digit} compact={clockCompact} />
																))}
																<span className="timer-colon">:</span>
																{secondDigitItems.map(({ digit, key }) => (
																	<RollDigit key={key} value={digit} compact={clockCompact} />
																))}
															</div>
															<span className="timer-edit-underline" aria-hidden="true" />
															{isEditingTimer && (
																<input
																	ref={timerInputRef}
																	className="timer-edit-input"
																	type="text"
																	inputMode="numeric"
																	aria-label="Set timer duration"
																	value={formatTimerDigits(timerEditDigits)}
																	onChange={(e) =>
																		setTimerEditDigits(
																			e.target.value.replace(/\D/g, "").slice(0, 5)
																		)
																	}
																	onKeyDown={(e) => {
																		if (e.key === "Enter") commitTimerEdit(true);
																	}}
																	onBlur={() => commitTimerEdit(false)}
																	onWheel={(e) => e.stopPropagation()}
																/>
															)}
														</div>
													</div>

													<div className="timer-controls-row">
														<span className="timer-controls-spacer" aria-hidden="true" />
														<button
															onClick={handlePrimaryTimerAction}
															onMouseDown={(e) => e.preventDefault()}
															className="timer-btn-play"
															title={primaryTimerLabel}
														>
															{isTimerRunning ? (
																<Pause size={14} strokeWidth={2} fill="currentColor" />
															) : (
																<Play size={14} strokeWidth={2} fill="currentColor" />
															)}
															<span>{primaryTimerLabel}</span>
														</button>
														<button
															onClick={resetTimer}
															className="timer-btn-reset"
															disabled={timerSeconds === 0 && !isTimerFinished}
															title="Reset"
														>
															<RotateCcw size={14} strokeWidth={2.5} />
														</button>
													</div>

													<div className="timer-preset-group">
														{[5, 15, 25, 50].map((mins) => (
															<button
																key={mins}
																onClick={() => startTimer(mins * 60)}
																className={`timer-preset-segment ${lastDurationSeconds === mins * 60 && !isTimerFinished ? "active" : ""}`}
															>
																{mins}m
															</button>
														))}
													</div>
												</div>
											</div>
										</motion.div>
									)}
								</AnimatePresence>

								{/* Shelf Panel */}
								<AnimatePresence>
									{bloomMode === "shelf" && (
										<motion.div
											className="shelf-content"
											onClick={(e) => e.stopPropagation()}
											onWheel={handleShelfWheel}
											initial={{ opacity: 0 }}
											animate={{ opacity: 1 }}
											exit={{ opacity: 0, filter: "blur(4px)", transition: { duration: 0.1 } }}
											transition={{ type: "spring", stiffness: 400, damping: 30 }}
										>
											{shelfNotice && <div className="shelf-notice">{shelfNotice}</div>}
											{shelfItems.length === 0 ? (
												<div className="shelf-empty">
													<span>Drop files or selected text here</span>
												</div>
											) : (
												<div
													className={`shelf-items-grid ${shelfOverflowing ? "overflowing" : ""}`}
													ref={shelfGridRef}
												>
													<AnimatePresence mode="popLayout" onExitComplete={measureShelfOverflow}>
														{shelfItems.map((item) => {
															const preview = item.path ? shelfThumbs[item.path] : undefined;
															const thumb = preview?.thumb;
															return (
																<motion.div
																	key={item.id}
																	layout
																	initial={
																		shelfNewIdsRef.current.has(item.id)
																			? { opacity: 0, scale: 0.82 }
																			: false
																	}
																	animate={{ opacity: 1, scale: 1 }}
																	exit={{
																		opacity: 0,
																		scale: 0.82,
																		transition: { duration: 0.16, ease: "easeOut" }
																	}}
																	transition={{
																		type: "spring",
																		stiffness: 520,
																		damping: 38,
																		mass: 0.7
																	}}
																	className={`shelf-tile ${item.type === "text" || (!thumb && preview?.text) ? "is-text" : ""}`}
																	title={
																		item.type === "text" ? item.text?.slice(0, 1000) : item.path
																	}
																	onPointerDown={(e) => handleShelfPointerDown(e, item)}
																	onPointerMove={(e) => handleShelfPointerMove(e, item)}
																	onClick={(e) => handleShelfTileClick(e, item)}
																>
																	<div className="shelf-tile-preview">
																		{item.type === "text" ? (
																			<div className="shelf-tile-text">
																				<Type
																					size={11}
																					strokeWidth={2}
																					className="shelf-tile-text-icon"
																				/>
																				<span className="shelf-tile-text-body">{item.text}</span>
																			</div>
																		) : !thumb && preview?.text ? (
																			// A document or text file: its opening text,
																			// marked with its type's icon.
																			<div className="shelf-tile-text">
																				<ShelfTypeIcon
																					name={item.name}
																					folder={false}
																					size={11}
																					className="shelf-tile-text-icon"
																				/>
																				<span className="shelf-tile-text-body">{preview.text}</span>
																			</div>
																		) : (
																			<>
																				<div className="shelf-tile-placeholder">
																					{preview && !thumb ? (
																						<ShelfTypeIcon
																							name={item.name}
																							folder={preview.folder}
																						/>
																					) : null}
																				</div>
																				{thumb ? (
																					<img src={thumb} alt="" draggable={false} />
																				) : null}
																			</>
																		)}
																	</div>
																	<span className="shelf-tile-name">
																		{item.type === "text"
																			? `Text · ${item.text?.length ?? 0} chars`
																			: item.name}
																	</span>
																	<button
																		className="shelf-tile-remove"
																		title="Remove from shelf"
																		onClick={(e) => {
																			e.stopPropagation();
																			removeShelfItem(item.id);
																		}}
																	>
																		<X size={11} strokeWidth={2.5} />
																	</button>
																</motion.div>
															);
														})}
													</AnimatePresence>
												</div>
											)}
										</motion.div>
									)}
								</AnimatePresence>

								{/* Urgent quota strip over whatever the island shows;
									bloomMode is untouched, so the content underneath is
									exactly what returns when the alert clears. */}
								<AnimatePresence>
									{quotaAlertActive && (
										<motion.div
											key="quota-alert"
											style={{ position: "absolute", inset: 0, zIndex: 4 }}
											initial={{ opacity: 0 }}
											animate={{ opacity: 1 }}
											exit={{
												opacity: 0,
												filter: "blur(4px)",
												transition: { duration: 0.1 }
											}}
											transition={{ type: "spring", stiffness: 400, damping: 30 }}
										>
											<QuotaAlert
												readings={shownQuotaReadings}
												onOpen={() => {
													// Open the space already focused on the urgent agent.
													const first = shownQuotaReadings.find(urgent);
													if (first) setQuotaSelected(first.id);
													openNotch("quotas");
												}}
											/>
										</motion.div>
									)}
								</AnimatePresence>
							</motion.div>
						)}
					</AnimatePresence>
				</motion.div>
				{/* The shelf's items, as a bubble on the left of the collapsed pill */}
				<AnimatePresence onExitComplete={() => reportNotchRect.current()}>
					{showShelfBubble && !integrated && (
						<motion.button
							key="shelf-bubble"
							ref={shelfBubbleRef}
							type="button"
							className={`activity-bubble shelf-bubble ${isHovered ? "expanded" : ""}`}
							aria-label={`Open shelf: ${shelfItems.length} item${shelfItems.length === 1 ? "" : "s"}`}
							// Anchored at the pill's centre, offset past its left edge.
							style={{ right: "50%" }}
							initial={{ opacity: 0, scale: 0.4, x: 28, marginRight: getDynamicWidth() / 2 + 8 }}
							animate={{ opacity: 1, scale: 1, x: 0, marginRight: getDynamicWidth() / 2 + 8 }}
							exit={{ opacity: 0, scale: 0.4, x: 28 }}
							transition={{
								type: "spring",
								stiffness: 400,
								damping: 31,
								opacity: { duration: 0.15 }
							}}
							onAnimationComplete={() => reportNotchRect.current()}
							onClick={(e) => {
								e.stopPropagation();
								setIsHovered(true);
								openNotch("shelf");
							}}
						>
							<Inbox size={12} strokeWidth={2.4} />
							<span>{shelfItems.length}</span>
						</motion.button>
					)}
				</AnimatePresence>
				{/* Music in its own bubble while a timer holds the collapsed pill */}
				<AnimatePresence onExitComplete={() => reportNotchRect.current()}>
					{showMusicBubble && !integrated && (
						<motion.button
							key="music-bubble"
							ref={musicBubbleRef}
							type="button"
							className={`activity-bubble ${isHovered ? "expanded" : ""}`}
							aria-label={mediaInfo.title ? `Open music: ${mediaInfo.title}` : "Open music"}
							// Anchored at the pill's centre, offset past its right edge.
							style={{ left: "50%" }}
							initial={{ opacity: 0, scale: 0.4, x: -28, marginLeft: getDynamicWidth() / 2 + 8 }}
							animate={{ opacity: 1, scale: 1, x: 0, marginLeft: getDynamicWidth() / 2 + 8 }}
							exit={{ opacity: 0, scale: 0.4, x: -28 }}
							transition={{
								type: "spring",
								stiffness: 400,
								damping: 31,
								opacity: { duration: 0.15 }
							}}
							onAnimationComplete={() => reportNotchRect.current()}
							onClick={(e) => {
								e.stopPropagation();
								setIsHovered(true);
								openNotch("music");
							}}
						>
							{settingsAlbumArtEnabled && albumArtUrl ? (
								<img src={albumArtUrl} alt="" draggable={false} />
							) : (
								<Music size={14} strokeWidth={2.2} />
							)}
						</motion.button>
					)}
				</AnimatePresence>
			</div>
		</div>
	);
}

export default App;
