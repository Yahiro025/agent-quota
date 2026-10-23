import { useEffect, useRef, useState } from "react";
import { invoke } from "@tauri-apps/api/core";
import { ChevronDown, ChevronRight, Clock, Pin, RefreshCw, X } from "lucide-react";
import { band, type QuotaAgent } from "./protocol";
import { AgentMark, agentName } from "./marks";
import {
	anyBusy,
	basis,
	defaultSelection,
	detailLines,
	failedCount,
	formatReset
} from "./quotaView";
import "./QuotaSpace.css";

export interface QuotaPanelProps {
	readings: QuotaAgent[];
	selected: string | undefined;
	onSelect: (id: string) => void;
	pinned: boolean;
	onPin: (pinned: boolean) => void;
	onClose: () => void;
}

// Same ring paint as the standalone widget: green below 70, amber from 70,
// red from 90. Thresholds live in `band`; these are only paint.
const BAND_COLOR = {
	green: "#32D74B",
	amber: "#FFD60A",
	red: "#FF453A"
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

function Ring({ used }: { used: number | undefined }) {
	const pct = typeof used === "number" ? Math.round(used) : undefined;
	const r = 11;
	const c = 2 * Math.PI * r;
	const fill = pct === undefined ? 0 : (Math.min(Math.max(pct, 0), 100) / 100) * c;
	return (
		<svg viewBox="0 0 28 28" className="qs-ring-svg" aria-hidden="true">
			<circle cx="14" cy="14" r={r} className="qs-ring-track" />
			{pct !== undefined && (
				<circle
					cx="14"
					cy="14"
					r={r}
					className="qs-ring-fill"
					stroke={BAND_COLOR[band(pct)]}
					strokeDasharray={`${fill} ${c}`}
					transform="rotate(-90 14 14)"
				/>
			)}
		</svg>
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
	onClose
}: QuotaPanelProps) {
	const [now, setNow] = useState(() => new Date());
	// In-memory only: countdown is the default; a click on any reset display
	// switches every reset to the wall clock, like the standalone widget.
	const [clock, setClock] = useState(false);
	const [groupsOpen, setGroupsOpen] = useState(false);
	const rootRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const timer = window.setInterval(() => setNow(new Date()), 1000);
		return () => window.clearInterval(timer);
	}, []);

	// The panel drives the notch's height while mounted. `.qs` is
	// unconstrained (the scroll cap lives on the wrapping viewport), so the
	// measured height is always the true content height; Rust clamps it to
	// the monitor. devicePixelRatio converts zoomed CSS px to physical px.
	useEffect(() => {
		const el = rootRef.current;
		if (!el) return;
		const report = () => {
			invoke("set_quota_panel_height", {
				height: Math.round(el.getBoundingClientRect().height * window.devicePixelRatio)
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
	const failed = failedCount(readings);

	// A helper that is down or restarting rejects the invoke; that is a
	// no-op, and the next snapshot repaints anyway.
	const refresh = (id: string | null) => {
		invoke("refresh_quota", { id }).catch(() => {});
	};

	const groupsToggle =
		sel !== undefined &&
		(sel.details?.length ?? 0) === 0 &&
		(sel.groups?.length ?? 0) > 0 &&
		(sel.state === "ok" || sel.state === "stale");

	const resetText = sel?.reset5h ? formatReset(sel.reset5h, now, clock) : "";

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
					<span className="qs-empty-sub">Waiting for the quota helper.</span>
				</div>
			) : (
				<>
					<div className="qs-rings">
						{readings.map((a) => {
							const pct = typeof a.used === "number" ? Math.round(a.used) : undefined;
							return (
								<button
									key={a.id}
									type="button"
									className={`qs-cell${a.id === selectedId ? " selected" : ""}`}
									tabIndex={-1}
									onMouseDown={(e) => e.preventDefault()}
									onClick={() => onSelect(a.id)}
									title={agentName(a.id)}
									data-testid={`quota-cell-${a.id}`}
								>
									<span className="qs-cell-top">
										<span className="qs-ring">
											<Ring used={a.used} />
											<span className="qs-mark">
												<AgentMark id={a.id} />
											</span>
										</span>
										<span
											className={`qs-pct${pct === undefined ? " empty" : ""}`}
											style={
												pct === undefined
													? undefined
													: { color: BAND_COLOR[band(pct)] }
											}
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
						<div className="qs-detail" data-testid="quota-detail">
							<div className="qs-detail-title">
								{agentName(sel.id)}
								{sel.plan ? ` · ${sel.plan}` : ""}
							</div>
							{detailLines(sel, now, clock).map((line, i) => (
								<div
									key={i}
									className={`qs-line${sel.state === "failed" ? " qs-error" : ""}`}
								>
									{line}
								</div>
							))}
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
							{sel.observedAt && (
								<div className="qs-line qs-observed">
									Source observed {timeText(sel.observedAt)}
								</div>
							)}
							{sel.state === "stale" && (
								<>
									{sel.lastGoodAt && (
										<div className="qs-line qs-stale">
											Last good {timeText(sel.lastGoodAt)}
										</div>
									)}
									{sel.err && <div className="qs-line qs-error">{sel.err}</div>}
								</>
							)}
							{(groupsToggle || resetText !== "" || (sel.state === "failed" && sel.help)) && (
								<div className="qs-controls">
									{groupsToggle && (
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
											{groupsOpen ? (
												<ChevronDown size={11} />
											) : (
												<ChevronRight size={11} />
											)}
										</button>
									)}
									{resetText !== "" && (
										<button
											type="button"
											className="qs-ctl"
											tabIndex={-1}
											onMouseDown={(e) => e.preventDefault()}
											onClick={() => setClock((v) => !v)}
											title="Toggle countdown / clock"
											data-testid="quota-reset"
										>
											<Clock size={11} />
											{resetText === "now"
												? "5h · now"
												: clock
													? `5h · at ${resetText}`
													: `5h · in ${resetText}`}
										</button>
									)}
									{sel.state === "failed" && sel.help && (
										<button
											type="button"
											className="qs-ctl"
											tabIndex={-1}
											onMouseDown={(e) => e.preventDefault()}
											onClick={() => refresh(sel.id)}
											title={`Refresh ${agentName(sel.id)}`}
											data-testid="quota-hint"
										>
											{sel.help}
										</button>
									)}
								</div>
							)}
						</div>
					)}
				</>
			)}

			<div className="qs-divider" />
			<div className="qs-footer">
				{failed > 0 && (
					<span className="qs-failed" data-testid="quota-failed">
						{failed} failed
					</span>
				)}
				<button
					type="button"
					className="qs-refresh"
					tabIndex={-1}
					onMouseDown={(e) => e.preventDefault()}
					onClick={() => refresh(null)}
					disabled={busy}
					data-testid="quota-refresh"
				>
					<RefreshCw size={11} className={busy ? "qs-spin" : undefined} />
					{busy ? "Checking…" : "Refresh"}
				</button>
			</div>
		</div>
	);
}
