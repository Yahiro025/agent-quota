import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { invoke } from "@tauri-apps/api/core";
import { ChevronDown, ChevronRight, Clock, Maximize2, Minimize2, Pin, RefreshCw, X } from "lucide-react";
import { band, type QuotaAgent } from "./protocol";
import { AgentMark, agentName } from "./marks";
import { agentRings, anyBusy, basis, defaultSelection, detailRows } from "./quotaView";
import { useSlidingPad } from "./useSlidingPad";
import Counter from "../components/reactbits/Counter";
import ShinyText from "../components/reactbits/ShinyText";
import SpotlightCard from "../components/reactbits/SpotlightCard";
import "./QuotaSpace.css";

export interface QuotaPanelProps {
	readings: QuotaAgent[];
	selected: string | undefined;
	onSelect: (id: string) => void;
	pinned: boolean;
	onPin: (pinned: boolean) => void;
	// Collapses the whole space into the notch's own row, where the marks live.
	onCompact: () => void;
	onClose: () => void;
	// Replaces the empty state's subline when readings exist but every agent
	// is hidden in settings, so the panel does not claim to be waiting.
	emptyHint?: string;
	/** MyFinder's flat menu presentation; standalone keeps its ring row. */
	menu?: boolean;
	/** MyFinder menu compact chip-grid presentation. */
	compact?: boolean;
	/** Restores the expanded MyFinder menu from compact chip mode. */
	onExpand?: () => void;
	/** MyFinder menu only: root CSS px the window must keep free around the
	 *  content (pill offset below the window top, bottom room, borders). Added
	 *  to the content height when asking the host for a window height. */
	menuReserve?: number;
	/** Panel navigation for integrated mode. */
	onModeChange?: (mode: "quotas" | "music" | "command-center") => void;
	/** Whether active media is available for the Music tab. */
	musicAvailable?: boolean;
}

// Same ring paint as the standalone widget: green below 70, amber from 70,
// red from 90. Thresholds live in `band`; these are only paint.
const BAND_COLOR = {
	green: "#32D74B",
	amber: "#FFD60A",
	red: "#FF453A"
} as const;

// Text colors use CSS variables so light mode reaches >= 4.5:1 contrast
// while dark mode stays byte-identical to the original paint.
const BAND_TEXT_COLOR = {
	green: "var(--usage-normal-text, #32D74B)",
	amber: "var(--usage-warning-text, #FFD60A)",
	red: "var(--usage-critical-text, #FF453A)"
} as const;

// The standalone formats observed/last-good instants as HH:mm local time.
const timeText = (iso: string): string => {
	const d = new Date(iso);
	return Number.isNaN(d.getTime())
		? ""
		: d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
};

// DrawGroups in widget/paint.go lists every window label once, 5h first.
const groupLabels = (a: QuotaAgent): string[] => {
	const labels: string[] = [];
	for (const g of a.groups ?? []) {
		for (const w of g.windows) {
			if (labels.includes(w.label)) continue;
			if (w.label === "5h") labels.unshift(w.label);
			else labels.push(w.label);
		}
	}
	return labels;
};

const groupLabelName = (label: string): string =>
	label === "week" ? "Week" : label === "day" ? "Day" : label;

// Two concentric rings: the outer track is always the longer window and
// the inner the shorter one, matching the standalone widget's ring pair.
// Both tracks render even when a ring has no arc so the pair's signature
// never collapses to a single circle.
const RING_ARC = (used: number, r: number) => {
	const pct = Math.min(Math.max(Math.round(used), 0), 100);
	const c = 2 * Math.PI * r;
	return { c, fill: (pct / 100) * c, color: BAND_COLOR[band(pct)], band: band(pct) };
};

function Ring({
	inner,
	outer,
	stale
}: {
	inner: number | undefined;
	outer: number | undefined;
	stale?: boolean;
}) {
	const dim = stale ? 0.55 : undefined;
	const [animated, setAnimated] = useState(false);
	useEffect(() => {
		const raf = requestAnimationFrame(() => {
			setAnimated(true);
		});
		return () => cancelAnimationFrame(raf);
	}, []);
	const animClass = animated ? " qs-ring-animated" : "";
	const outerArc = outer === undefined ? undefined : RING_ARC(outer, 12.25);
	const innerArc = inner === undefined ? undefined : RING_ARC(inner, 9.5);
	return (
		<svg viewBox="0 0 28 28" className="qs-ring-svg" aria-hidden="true">
			<circle cx="14" cy="14" r={12.25} className="qs-ring-track" />
			<circle cx="14" cy="14" r={9.5} className="qs-ring-track" />
			{outerArc && (
				<circle
					cx="14"
					cy="14"
					r={12.25}
					className={`qs-ring-fill qs-ring-outer${animClass}`}
					data-band={outerArc.band}
					stroke={outerArc.color}
					strokeDasharray={`${outerArc.fill} ${outerArc.c}`}
					opacity={dim}
					transform="rotate(-90 14 14)"
				/>
			)}
			{innerArc && (
				<circle
					cx="14"
					cy="14"
					r={9.5}
					className={`qs-ring-fill qs-ring-inner${animClass}`}
					data-band={innerArc.band}
					stroke={innerArc.color}
					strokeDasharray={`${innerArc.fill} ${innerArc.c}`}
					opacity={dim}
					transform="rotate(-90 14 14)"
				/>
			)}
		</svg>
	);
}

// The ring pair with the agent's brand mark centred inside: the same tile
// the quota space draws per agent, reused by the collapsed hover badge.
export function QuotaRingMark({ a }: { a: QuotaAgent }) {
	const { inner, outer } = agentRings(a);
	return (
		<span className="qs-ring">
			<Ring inner={inner} outer={outer} stale={a.state === "stale"} />
			<span className="qs-mark">
				<AgentMark id={a.id} />
			</span>
		</span>
	);
}

// MyFinder's 24px bar has no room for a ring around a legible logo, so there
// the housing itself is the gauge: one black capsule whose outline and tint
// fill left to right with the headline usage. `children` is the agent mark,
// which App.tsx owns because clicking it cycles agents.
export function QuotaChip({ a, children }: { a: QuotaAgent; children: ReactNode }) {
	const used = (a.state === "ok" || a.state === "stale") && typeof a.used === "number" ? Math.round(a.used) : undefined;
	const fill = used === undefined ? undefined : Math.min(Math.max(used, 0), 100);
	return (
		<span
			className="quota-chip"
			data-state={a.state}
			style={
				fill === undefined
					? undefined
					: ({ "--quota-fill": `${fill}%`, "--quota-color": BAND_COLOR[band(fill)] } as CSSProperties)
			}
		>
			{children}
			<span className="quota-chip-value">
				{used === undefined ? (
					"—"
				) : (
					<>
						<span>{used.toLocaleString()}</span>
						<span className="unit">%</span>
					</>
				)}
			</span>
		</span>
	);
}

const switchEase = [0.16, 1, 0.3, 1] as const;

export function QuotaHoverCard({ a, place }: { a: QuotaAgent; place?: string }) {
	const [now, setNow] = useState(() => new Date());
	const reduce = useReducedMotion();
	useEffect(() => {
		const timer = window.setInterval(() => setNow(new Date()), 1000);
		return () => window.clearInterval(timer);
	}, []);
	const rows = a.state === "ok" || a.state === "stale" ? detailRows(a, now, false) : [];
	const transition = reduce ? { duration: 0 } : { duration: 0.22, ease: switchEase };
	return (
		<div className="qh" data-testid="quota-hover-card">
			<AnimatePresence mode="popLayout" initial={false}>
				<motion.div
					key={a.id}
					initial={reduce ? false : { opacity: 0, y: 6 }}
					animate={{ opacity: 1, y: 0 }}
					exit={reduce ? undefined : { opacity: 0, y: -6 }}
					transition={transition}
				>
					<div className="qh-title">
						<span>
							{agentName(a.id)}
							{a.plan ? <span className="qh-plan"> · {a.plan}</span> : null}
						</span>
						{place ? <span className="qh-place">{place}</span> : null}
					</div>
					{a.state === "waiting" && <div className="qh-line">Checking…</div>}
					{a.state === "failed" && (
						<div className="qh-line qh-error">{a.err || a.detail || "Unavailable"}</div>
					)}
					{rows.length > 0 && (
						<div className="qh-rows">
							{rows.map((row, i) => (
								<div key={i} className="qh-row">
									<span className="qh-label">{row.label}</span>
									<span className="qh-usage">
										{typeof row.used === "number" && (
											<span className="qh-bar" aria-hidden="true">
												<span
													className="qh-fill"
													style={{
														width: `${Math.min(Math.max(Math.round(row.used), 0), 100)}%`,
														backgroundColor: BAND_COLOR[band(Math.round(row.used))]
													}}
												/>
											</span>
										)}
										<span className="qh-val">{row.value}</span>
									</span>
									{row.reset && (
										<span className="qh-reset">
											<span>{row.reset}</span>
											{row.resetClock ? (
												<span className="qh-reset-at">{row.resetClock}</span>
											) : null}
										</span>
									)}
								</div>
							))}
						</div>
					)}
					{rows.length === 0 && (a.state === "ok" || a.state === "stale") && (
						<div className="qh-line">{basis(a)}</div>
					)}
					{a.state === "stale" && a.err && <div className="qh-line qh-stale">{a.err}</div>}
				</motion.div>
			</AnimatePresence>
		</div>
	);
}

// Meters light whole segments, like an instrument's LED bar. Any reading
// above zero lights at least one, so 1% never looks like 0%.
const METER_SEGMENTS = 24;

function Meter({ used, className }: { used: number; className?: string }) {
	const pct = Math.min(Math.max(Math.round(used), 0), 100);
	const lit = pct === 0 ? 0 : Math.max(1, Math.round((pct * METER_SEGMENTS) / 100));
	return (
		<span
			className={`qs-meter${className ? ` ${className}` : ""}`}
			data-band={band(pct)}
			style={{ "--lit": `${(lit / METER_SEGMENTS) * 100}%` } as CSSProperties}
			aria-hidden="true"
		>
			<span />
		</span>
	);
}

// "resets in 5d 13h" reads as a caption over its value in the display.
const splitReset = (reset: string): { caption: string; value: string } =>
	/^resets in /.test(reset)
		? { caption: "Resets in", value: reset.slice("resets in ".length) }
		: { caption: "Resets", value: reset.replace(/^resets /, "") };

// The MyFinder menu ends in a black inset display for the selected agent: the
// one surface where the ring pair returns at full size and the headline
// percent rolls. It is never keyed by agent, so switching agents rolls the
// digits and sweeps the rings from the previous reading instead of replaying
// the intro.
function QuotaDisplay({
	a,
	now,
	clock,
	onRefresh,
	groupsOpen,
	onToggleGroups
}: {
	a: QuotaAgent;
	now: Date;
	clock: boolean;
	onRefresh: (id: string) => void;
	groupsOpen: boolean;
	onToggleGroups: () => void;
}) {
	const known = a.state === "ok" || a.state === "stale";
	const used = known && typeof a.used === "number" ? Math.round(a.used) : undefined;
	const rows = known ? detailRows(a, now, clock) : [];
	const primary =
		rows.find((row) => row.used === a.used && row.reset) ?? rows.find((row) => row.reset);
	const reset = primary?.reset ? splitReset(primary.reset) : undefined;
	const groups = known && (a.groups?.length ?? 0) > 0;
	const note = a.state === "failed" ? a.err || a.detail : undefined;
	return (
		<SpotlightCard
			className="qs-display"
			theme="dark"
			intensity={0.1}
			spotlightSize={280}
			softness={0.9}
			borderGlow={0.35}
			grain={0}
			flare={false}
			proximity={0}
			data-state={a.state}
			data-testid="quota-detail"
		>
			<div className="qs-display-head">
				<QuotaRingMark a={a} />
				<div className="qs-display-reading">
					<span className="qs-display-name">
						{/* ShinyText parses its colours on a canvas, so they are literal
						    here; the display is black in both themes. It is mounted only
						    while a check runs because its shimmer is a frame loop. */}
						{a.busy ? (
							<ShinyText text={agentName(a.id)} color="#8e8e93" shineColor="#f5f5f7" speed={1.6} />
						) : (
							agentName(a.id)
						)}
						{a.plan ? <span className="qs-display-plan"> · {a.plan}</span> : null}
					</span>
					{used === undefined ? (
						<span className="qs-display-status">
							{a.state === "waiting" ? (
								<ShinyText text="Checking…" color="#8e8e93" shineColor="#f5f5f7" speed={1.6} />
							) : (
								"Unavailable"
							)}
						</span>
					) : (
						<span
							className="qs-display-value"
							data-band={band(Math.min(Math.max(used, 0), 100))}
						>
							<Counter
								value={used}
								fontSize={34}
								fontWeight={600}
								duration={0.8}
								bounce={0.1}
								stagger={0.05}
								fade={0.2}
							/>
							<span className="qs-display-unit">% used</span>
						</span>
					)}
				</div>
				{reset && (
					<div className="qs-display-reset">
						<span className="qs-display-caption">{reset.caption}</span>
						<span className="qs-display-countdown">{reset.value}</span>
						{primary?.resetClock && (
							<span className="qs-display-caption">{primary.resetClock}</span>
						)}
					</div>
				)}
			</div>
			{rows.length > 0 && (
				<div className="qs-display-rows">
					{rows.map((row, i) => (
						<div key={i} className="qs-metric-row">
							<span className="qs-metric-label">{row.label}</span>
							{typeof row.used === "number" ? (
								<>
									<Meter used={row.used} />
									<span className="qs-metric-val">{row.value}</span>
								</>
							) : (
								<span className="qs-metric-val qs-metric-text">{row.value}</span>
							)}
							<span className="qs-metric-reset">{row.reset ?? ""}</span>
						</div>
					))}
				</div>
			)}
			{(note || (a.state === "failed" && a.help)) && (
				<div className="qs-display-note">
					{note && <span className="qs-line qs-error">{note}</span>}
					{a.state === "failed" && a.help && (
						<button
							type="button"
							className="qs-display-action"
							tabIndex={-1}
							onMouseDown={(e) => e.preventDefault()}
							onClick={() => onRefresh(a.id)}
							data-testid="quota-hint"
						>
							<RefreshCw size={11} />
							{a.help}
						</button>
					)}
				</div>
			)}
			{a.state === "stale" && (a.lastGoodAt || a.err) && (
				<div className="qs-display-note">
					{a.lastGoodAt && <span className="qs-line qs-stale">Last good {timeText(a.lastGoodAt)}</span>}
					{a.err && <span className="qs-line qs-error">{a.err}</span>}
				</div>
			)}
			{groups && (
				<div className="qs-display-groups">
					<button
						type="button"
						className="qs-ctl"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						onClick={onToggleGroups}
						aria-expanded={groupsOpen}
						data-testid="quota-groups-toggle"
					>
						{(a.groups ?? []).length} model groups
						{groupsOpen ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
					</button>
					{groupsOpen && (
						<div className="qs-groups" data-testid="quota-groups">
							<div
								className="qs-group-row qs-group-head"
								style={{ gridTemplateColumns: `1fr repeat(${groupLabels(a).length}, 48px)` }}
							>
								<span />
								{groupLabels(a).map((label) => (
									<span key={label} className="qs-group-col">
										{groupLabelName(label)}
									</span>
								))}
							</div>
							{(a.groups ?? []).map((g) => (
								<div
									key={g.name}
									className="qs-group-row"
									style={{ gridTemplateColumns: `1fr repeat(${groupLabels(a).length}, 48px)` }}
								>
									<span className="qs-group-name">{g.name}</span>
									{groupLabels(a).map((label) => {
										const w = g.windows.find((x) => x.label === label);
										return (
											<span key={label} className="qs-group-col">
												{w ? `${Math.round(w.used)}%` : "—"}
											</span>
										);
									})}
								</div>
							))}
						</div>
					)}
				</div>
			)}
		</SpotlightCard>
	);
}

// The whole open view of the standalone Agent Quota widget (widget/seam.go)
// inside Bloom's notch: header, one ring tile per agent, the selected agent's
// detail block, then a footer. Nothing here may take focus — every control is
// a focus-free button, there are no key handlers and no Esc/click-outside.
export function QuotaPanel({
	readings,
	selected,
	onSelect,
	pinned,
	onPin,
	onCompact,
	onClose,
	emptyHint,
	menu = false,
	compact = false,
	onExpand,
	menuReserve = 0,
	onModeChange,
	musicAvailable = true
}: QuotaPanelProps) {
	const [now, setNow] = useState(() => new Date());
	// In-memory only: countdown is the default; a click on any reset display
	// switches every reset to the wall clock, like the standalone widget.
	const [clock, setClock] = useState(false);
	const [groupsOpen, setGroupsOpen] = useState(false);
	// Menu only: the row under the pointer, so one highlight can glide between rows.
	const [hoverRow, setHoverRow] = useState<string | null>(null);
	const [checkedAt, setCheckedAt] = useState(() => Date.now());
	const rootRef = useRef<HTMLDivElement>(null);
	const ringsRef = useRef<HTMLDivElement>(null);
	const reduceMotion = useReducedMotion();

	useEffect(() => {
		const timer = window.setInterval(() => setNow(new Date()), 1000);
		return () => window.clearInterval(timer);
	}, []);

	useEffect(() => {
		setCheckedAt(Date.now());
	}, [readings]);

	// The panel drives the notch's height while mounted. `.qs` is
	// unconstrained (the scroll cap lives on the wrapping viewport), so the
	// measured height is always the true content height; Rust clamps it to
	// the monitor. devicePixelRatio converts zoomed CSS px to physical px.
	useEffect(() => {
		const el = rootRef.current;
		if (!el) return;
		const report = () => {
			// The panel sits below the island's status row, so the window must
			// fit `.qs`'s bottom edge inside `.bloom`, not `.qs`'s own height.
			const pill = el.closest(".bloom");
			// Menu: the content's own height plus what the host keeps around it.
			// Its raw bottom edge would move with the pill's animated translate
			// and omit the pill's offset, so the window came out too short and
			// the panel scrolled.
			const h = menu
				? el.getBoundingClientRect().height + menuReserve
				: pill
					? el.getBoundingClientRect().bottom - pill.getBoundingClientRect().top
					: el.getBoundingClientRect().height;
			invoke("set_quota_panel_height", {
				height: Math.round(h * window.devicePixelRatio)
			}).catch(() => {});
		};
		report();
		const observer = new ResizeObserver(report);
		observer.observe(el);
		return () => {
			observer.disconnect();
			invoke("set_quota_panel_height", { height: 0 }).catch(() => {});
		};
	}, [menu, menuReserve]);

	const selectedId = readings.some((a) => a.id === selected)
		? selected
		: defaultSelection(readings);
	const sel = readings.find((a) => a.id === selectedId);
	const inspected = (hoverRow ? readings.find((a) => a.id === hoverRow) : undefined) ?? sel;
	const busy = anyBusy(readings);

	const pad = useSlidingPad(ringsRef, selectedId, [readings]);

	const selectedIndex = readings.findIndex((a) => a.id === selectedId);
	const selectionRef = useRef<{
		id: string | undefined | null;
		index: number;
		dir: "next" | "prev" | "none";
	}>({ id: selectedId, index: selectedIndex, dir: "none" });

	let slideDirection: "next" | "prev" | "none" = "none";
	if (reduceMotion) {
		slideDirection = "none";
		selectionRef.current = { id: selectedId, index: selectedIndex, dir: "none" };
	} else if (selectedId !== selectionRef.current.id) {
		const prev = selectionRef.current;
		let dir: "next" | "prev" | "none" = "none";
		if (
			prev.id !== undefined &&
			prev.id !== null &&
			selectedId !== undefined &&
			selectedId !== null &&
			selectedIndex !== -1 &&
			prev.index !== -1 &&
			prev.index !== selectedIndex
		) {
			dir = selectedIndex > prev.index ? "next" : "prev";
		}
		selectionRef.current = { id: selectedId, index: selectedIndex, dir };
		slideDirection = dir;
	} else {
		selectionRef.current.index = selectedIndex;
		slideDirection = selectionRef.current.dir;
	}

	const minutesAgo = Math.floor((now.getTime() - checkedAt) / 60000);
	const checkedText = busy
		? "Checking\u2026"
		: minutesAgo < 1
			? "Just checked"
			: minutesAgo < 60
				? `${minutesAgo} min ago`
				: `${Math.floor(minutesAgo / 60)} h ago`;

	// A helper that is down or restarting rejects the invoke; that is a
	// no-op, and the next snapshot repaints anyway.
	const refresh = (id: string | null) => {
		invoke("refresh_quota", { id }).catch(() => {});
	};

	const groupsToggle =
		sel !== undefined &&
		(sel.groups?.length ?? 0) > 0 &&
		(sel.state === "ok" || sel.state === "stale");

	// The countdown/clock toggle shows when any visible row carries a reset.
	const hasReset =
		sel !== undefined &&
		(sel.state === "ok" || sel.state === "stale") &&
		Boolean(sel.reset5h || (sel.details ?? []).some((d) => d.resetAt));

	const inspectedKnown = inspected !== undefined && (inspected.state === "ok" || inspected.state === "stale");
	const inspectedRows = inspectedKnown ? detailRows(inspected, now, clock) : [];
	const inspectedPrimary =
		inspectedRows.find((row) => row.used === inspected?.used && row.reset) ??
		inspectedRows.find((row) => row.reset);
	const inspectedReset = inspectedPrimary?.reset?.replace(/^resets (in )?/, "");
	const inspectedSummary =
		inspectedRows.length > 0
			? inspectedRows
					.slice(0, 2)
					.map((row) => `${row.label} ${row.value.replace(/ used$/i, "")}`)
					.join(" \u00b7 ")
			: inspectedKnown
				? basis(inspected)
				: "";

	return (
		<div
			className={`qs${menu ? " qs-menu" : ""}${menu && compact ? " qs-menu-compact" : ""}`}
			ref={rootRef}
			data-testid="quota-space"
			onClick={(e) => e.stopPropagation()}
		>
			<div className="qs-header">
				{menu && <span className="qs-menu-title">Agent quota</span>}
				{/* The menu's display says "% used" beside its own number. */}
				{!menu && <span className="qs-caption">Used</span>}
				<span className="qs-header-actions">
					{menu ? (
						<button
							type="button"
							className="qs-icon-btn"
							tabIndex={-1}
							onMouseDown={(e) => e.preventDefault()}
							onClick={compact ? (onExpand ?? onCompact) : onCompact}
							aria-label={compact ? "Expand quota space" : "Compact quota space"}
							data-testid={compact ? "quota-expand" : "quota-compact"}
						>
							{compact ? <Maximize2 size={12} /> : <Minimize2 size={12} />}
						</button>
					) : (
						<button
							type="button"
							className="qs-icon-btn"
							tabIndex={-1}
							onMouseDown={(e) => e.preventDefault()}
							onClick={onCompact}
							aria-label="Compact quota space"
							title="Compact"
							data-testid="quota-compact"
						>
							<Minimize2 size={12} />
						</button>
					)}
					<button
						type="button"
						className={`qs-icon-btn${pinned ? " active" : ""}`}
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						onClick={() => onPin(!pinned)}
						aria-label={pinned ? "Unpin quota space" : "Pin quota space"}
						title={menu ? undefined : pinned ? "Unpin" : "Pin"}
						aria-pressed={pinned}
						data-testid="quota-pin"
					>
						<Pin size={12} />
					</button>
					<button
						type="button"
						className="qs-icon-btn"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						onClick={onClose}
						aria-label="Close"
						title={menu ? undefined : "Close"}
						data-testid="quota-close"
					>
						<X size={12} />
					</button>
				</span>
			</div>

			{menu && onModeChange && (
				<div className="qs-menu-nav navigation" aria-label="Panels">
					<button
						type="button"
						className="qs-menu-nav-tab"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						aria-pressed="true"
						onClick={() => onModeChange("quotas")}
						data-testid="quota-nav-quotas"
					>
						Quotas
					</button>
					<button
						type="button"
						className="qs-menu-nav-tab"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						aria-pressed="false"
						disabled={musicAvailable === false}
						onClick={() => onModeChange("music")}
						data-testid="quota-nav-music"
					>
						Music
					</button>
					<button
						type="button"
						className="qs-menu-nav-tab"
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						aria-pressed="false"
						onClick={() => onModeChange("command-center")}
						data-testid="quota-nav-controls"
					>
						Controls
					</button>
				</div>
			)}

			{readings.length === 0 ? (
				<div className="qs-empty">
					<span className="qs-empty-title">No agents enabled</span>
					<span className="qs-empty-sub">{emptyHint ?? "Waiting for the quota helper."}</span>
				</div>
			) : (
				<>
					{menu && compact ? (
						<>
							<div className="qs-menu-chips" onMouseLeave={() => setHoverRow(null)}>
								{readings.map((a) => {
									const known = a.state === "ok" || a.state === "stale";
									const pct = known && typeof a.used === "number" ? Math.round(a.used) : undefined;
									return (
										<button
											key={a.id}
											type="button"
											className="qs-chip-btn"
											data-selected={a.id === selectedId}
											data-state={a.state}
											data-band={pct === undefined ? undefined : band(Math.min(Math.max(pct, 0), 100))}
											tabIndex={-1}
											onMouseDown={(e) => e.preventDefault()}
											onClick={() => onSelect(a.id)}
											onMouseEnter={() => setHoverRow(a.id)}
											aria-label={`${agentName(a.id)}, ${pct === undefined ? (a.state === "waiting" ? "Checking" : "Unavailable") : `${pct}% used${a.state === "stale" ? ", last good reading" : ""}`}`}
											aria-pressed={a.id === selectedId}
											data-testid={`quota-cell-${a.id}`}
										>
											<QuotaChip a={a}>
												<span className="qs-chip-mark">
													<AgentMark id={a.id} />
												</span>
											</QuotaChip>
										</button>
									);
								})}
							</div>
							{inspected !== undefined && (
								<div
									className="qs-compact-detail"
									data-state={inspected.state}
									data-testid="quota-compact-detail"
								>
									<span className="qs-compact-identity">
										<span className="qs-compact-name">{agentName(inspected.id)}</span>
										{inspected.plan ? <span className="qs-compact-plan"> &middot; {inspected.plan}</span> : null}
									</span>
									<span className="qs-compact-meta">
										{inspected.state === "waiting" && (
											<span className="qs-compact-status">Checking&hellip;</span>
										)}
										{inspected.state === "failed" && (
											<>
												<span className="qs-compact-error">
													{inspected.err || inspected.detail || "Unavailable"}
												</span>
												{inspected.help && (
													<button
														type="button"
														className="qs-compact-retry"
														tabIndex={-1}
														onMouseDown={(e) => e.preventDefault()}
														onClick={() => refresh(inspected.id)}
														data-testid="quota-hint"
													>
														<RefreshCw size={10} />
														<span>{inspected.help}</span>
													</button>
												)}
											</>
										)}
										{inspectedKnown && (
											<>
												{inspected.state === "stale" && inspected.lastGoodAt ? (
													<span className="qs-compact-stale">Last good {timeText(inspected.lastGoodAt)}</span>
												) : null}
												{inspectedSummary ? <span className="qs-compact-summary">{inspectedSummary}</span> : null}
												{inspectedReset ? <span className="qs-compact-reset">{inspectedReset}</span> : null}
											</>
										)}
									</span>
								</div>
							)}
						</>
					) : menu ? <div className="qs-menu-rows" onMouseLeave={() => setHoverRow(null)}>
						{readings.map((a) => {
							const known = a.state === "ok" || a.state === "stale";
							const pct = known && typeof a.used === "number" ? Math.round(a.used) : undefined;
							const rows = known ? detailRows(a, now, clock) : [];
							const primary = rows.find((row) => row.used === a.used) ?? rows.find((row) => row.reset);
							return <button key={a.id} type="button" className="qs-menu-row"
								data-selected={a.id === selectedId} data-state={a.state}
								data-band={pct === undefined ? undefined : band(Math.min(Math.max(pct, 0), 100))}
								tabIndex={-1} onMouseDown={(e) => e.preventDefault()} onClick={() => onSelect(a.id)}
								onMouseEnter={() => setHoverRow(a.id)}
								aria-label={`${agentName(a.id)}, ${pct === undefined ? a.state === "waiting" ? "Checking" : "Unavailable" : `${pct}% used${a.state === "stale" ? ", last good reading" : ""}`}`}
								aria-pressed={a.id === selectedId} data-testid={`quota-cell-${a.id}`}>
								{/* One shared highlight glides to the hovered row instead of each row fading its own. */}
								{hoverRow === a.id && (
									<motion.span
										layoutId="qs-row-hover"
										className="qs-row-hover"
										aria-hidden="true"
										initial={false}
										transition={reduceMotion ? { duration: 0 } : { type: "spring", stiffness: 620, damping: 46, mass: 0.6 }}
									/>
								)}
								<span className="qs-menu-identity"><span className="qs-menu-mark"><AgentMark id={a.id} /></span>{agentName(a.id)}</span>
								<span className="qs-menu-reading">{pct === undefined ? <span>{a.state === "waiting" ? "Checking…" : "Unavailable"}</span> : <>
									<Meter used={pct} className="qs-menu-bar" />
									<span>{pct.toLocaleString()}%</span>
								</>}</span>
								<span className="qs-menu-reset">{primary?.reset?.replace(/^resets (in )?/, "") ?? ""}</span>
							</button>;
						})}
					</div> : <div className="qs-rings" ref={ringsRef}>
						<div className="qs-pad" {...pad.padProps} />
						{readings.map((a) => {
							const pct = typeof a.used === "number" ? Math.round(a.used) : undefined;
							return (
								<button
									key={a.id}
									type="button"
									className={`qs-cell${a.id === selectedId ? " selected" : ""}`}
									data-selected={a.id === selectedId ? "true" : undefined}
									data-key={a.id}
									tabIndex={-1}
									onMouseDown={(e) => e.preventDefault()}
									onClick={() => onSelect(a.id)}
									aria-label={agentName(a.id)}
									title={agentName(a.id)}
									data-testid={`quota-cell-${a.id}`}
								>
									<span className="qs-cell-top">
										<QuotaRingMark a={a} />
										<span
											className={`qs-pct${pct === undefined ? " empty" : ""}`}
											data-band={pct === undefined ? undefined : band(pct)}
											style={pct === undefined ? undefined : { color: BAND_TEXT_COLOR[band(pct)] }}
										>
											{pct === undefined ? "—" : `${pct}%`}
										</span>
									</span>
									<span className="qs-name">{agentName(a.id)}</span>
									<span className="qs-basis">{basis(a)}</span>
								</button>
							);
						})}
					</div>}

					{sel !== undefined && menu && !compact && (
						<QuotaDisplay
							a={sel}
							now={now}
							clock={clock}
							onRefresh={refresh}
							groupsOpen={groupsOpen}
							onToggleGroups={() => setGroupsOpen((v) => !v)}
						/>
					)}
					{sel !== undefined && !menu && (
						<div
							key={sel.id}
							className="qs-detail"
							data-direction={slideDirection}
							data-testid="quota-detail"
						>
							<div className="qs-detail-head">
								<div className="qs-detail-title">
									{agentName(sel.id)}
									{sel.plan ? ` · ${sel.plan}` : ""}
								</div>
							</div>
							{sel.state === "waiting" && <div className="qs-line">Checking usage…</div>}
							{sel.state === "failed" && (
								<>
									<div className="qs-line qs-error">{sel.err || sel.detail || "Unavailable"}</div>
									{sel.help && (
										<div className="qs-controls">
											<button
												type="button"
												className="qs-ctl qs-hint"
												tabIndex={-1}
												onMouseDown={(e) => e.preventDefault()}
												onClick={() => refresh(sel.id)}
												title={menu ? undefined : `Refresh ${agentName(sel.id)}`}
												data-testid="quota-hint"
											>
												{sel.help}
											</button>
										</div>
									)}
								</>
							)}
							{(sel.state === "ok" || sel.state === "stale") && (
								<>
									<div className="qs-metrics">
										{detailRows(sel, now, clock).map((row, i) => (
											<div key={i} className="qs-metric-row">
												<span className="qs-metric-label">{row.label}</span>
												<span className="qs-metric-usage">
													{typeof row.used === "number" && (
														<span className="qs-metric-bar" aria-hidden="true">
															<span
																className="qs-metric-fill"
																style={{
																	width: `${Math.min(Math.max(Math.round(row.used), 0), 100)}%`,
																	backgroundColor: BAND_COLOR[band(Math.round(row.used))]
																}}
															/>
														</span>
													)}
													<span className="qs-metric-val">{row.value}</span>
												</span>
												{row.reset && <span className="qs-metric-reset">{row.reset}</span>}
											</div>
										))}
									</div>
									{groupsToggle && (
										<div className="qs-controls">
											<button
												type="button"
												className="qs-ctl"
												tabIndex={-1}
												onMouseDown={(e) => e.preventDefault()}
												onClick={() => setGroupsOpen((v) => !v)}
												aria-expanded={groupsOpen}
												data-testid="quota-groups-toggle"
											>
												{(sel.groups ?? []).length} model groups
												{groupsOpen ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
											</button>
										</div>
									)}
									{groupsOpen && groupsToggle && (
										<div className="qs-groups" data-testid="quota-groups">
											<div
												className="qs-group-row qs-group-head"
												style={{
													gridTemplateColumns: `1fr repeat(${groupLabels(sel).length}, 48px)`
												}}
											>
												<span />
												{groupLabels(sel).map((label) => (
													<span key={label} className="qs-group-col">
														{groupLabelName(label)}
													</span>
												))}
											</div>
											{(sel.groups ?? []).map((g) => (
												<div
													key={g.name}
													className="qs-group-row"
													style={{
														gridTemplateColumns: `1fr repeat(${groupLabels(sel).length}, 48px)`
													}}
												>
													<span className="qs-group-name">{g.name}</span>
													{groupLabels(sel).map((label) => {
														const w = g.windows.find((x) => x.label === label);
														return (
															<span key={label} className="qs-group-col">
																{w ? `${Math.round(w.used)}%` : "—"}
															</span>
														);
													})}
												</div>
											))}
										</div>
									)}
									{sel.state === "stale" && (
										<>
											{sel.lastGoodAt && (
												<div className="qs-line qs-stale">Last good {timeText(sel.lastGoodAt)}</div>
											)}
											{sel.err && <div className="qs-line qs-error">{sel.err}</div>}
										</>
									)}
								</>
							)}
						</div>
					)}
					<div className="qs-footer">
						{hasReset && (
							<button
								type="button"
								className="qs-ctl"
								tabIndex={-1}
								onMouseDown={(e) => e.preventDefault()}
								onClick={() => setClock((v) => !v)}
								aria-label="Toggle countdown / clock"
								title={menu ? undefined : "Toggle countdown / clock"}
								data-testid="quota-reset"
							>
								<Clock size={10} />
								<span>{clock ? "Clock" : "Countdown"}</span>
							</button>
						)}
						<button
							type="button"
							className="qs-footer-refresh"
							tabIndex={-1}
							onMouseDown={(e) => e.preventDefault()}
							onClick={() => refresh(null)}
							disabled={busy}
							aria-label={busy ? "Checking…" : "Refresh"}
							data-testid="quota-refresh"
						>
							<RefreshCw size={12} className={busy ? "qs-spin" : undefined} />
							<span>{checkedText}</span>
						</button>
					</div>
				</>
			)}
		</div>
	);
}
