import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { invoke } from "@tauri-apps/api/core";
import { ChevronDown, ChevronRight, Clock, Minimize2, Pin, RefreshCw, X } from "lucide-react";
import { band, type QuotaAgent } from "./protocol";
import { AgentMark, agentName } from "./marks";
import { agentRings, anyBusy, basis, defaultSelection, detailRows } from "./quotaView";
import { useSlidingPad } from "./useSlidingPad";
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
	return { c, fill: (pct / 100) * c, color: BAND_COLOR[band(pct)] };
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
	return (
		<svg viewBox="0 0 28 28" className="qs-ring-svg" aria-hidden="true">
			<circle cx="14" cy="14" r={12.25} className="qs-ring-track" />
			<circle cx="14" cy="14" r={9.5} className="qs-ring-track" />
			{outer !== undefined && (
				<circle
					cx="14"
					cy="14"
					r={12.25}
					className={`qs-ring-fill qs-ring-outer${animClass}`}
					stroke={RING_ARC(outer, 12.25).color}
					strokeDasharray={`${RING_ARC(outer, 12.25).fill} ${RING_ARC(outer, 12.25).c}`}
					opacity={dim}
					transform="rotate(-90 14 14)"
				/>
			)}
			{inner !== undefined && (
				<circle
					cx="14"
					cy="14"
					r={9.5}
					className={`qs-ring-fill qs-ring-inner${animClass}`}
					stroke={RING_ARC(inner, 9.5).color}
					strokeDasharray={`${RING_ARC(inner, 9.5).fill} ${RING_ARC(inner, 9.5).c}`}
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
	emptyHint
}: QuotaPanelProps) {
	const [now, setNow] = useState(() => new Date());
	// In-memory only: countdown is the default; a click on any reset display
	// switches every reset to the wall clock, like the standalone widget.
	const [clock, setClock] = useState(false);
	const [groupsOpen, setGroupsOpen] = useState(false);
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
			const h = pill
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
	}, []);

	const selectedId = readings.some((a) => a.id === selected)
		? selected
		: defaultSelection(readings);
	const sel = readings.find((a) => a.id === selectedId);
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
		? "Checking…"
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

	return (
		<div
			className="qs"
			ref={rootRef}
			data-testid="quota-space"
			onClick={(e) => e.stopPropagation()}
		>
			<div className="qs-header">
				<span className="qs-caption">Used</span>
				<span className="qs-header-actions">
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
					<button
						type="button"
						className={`qs-icon-btn${pinned ? " active" : ""}`}
						tabIndex={-1}
						onMouseDown={(e) => e.preventDefault()}
						onClick={() => onPin(!pinned)}
						aria-label={pinned ? "Unpin quota space" : "Pin quota space"}
						aria-pressed={pinned}
						title={pinned ? "Unpin" : "Pin"}
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
						title="Close"
						data-testid="quota-close"
					>
						<X size={12} />
					</button>
				</span>
			</div>

			{readings.length === 0 ? (
				<div className="qs-empty">
					<span className="qs-empty-title">No agents enabled</span>
					<span className="qs-empty-sub">{emptyHint ?? "Waiting for the quota helper."}</span>
				</div>
			) : (
				<>
					<div className="qs-rings" ref={ringsRef}>
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
					</div>

					{sel !== undefined && (
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
												title={`Refresh ${agentName(sel.id)}`}
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
								title="Toggle countdown / clock"
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
