import { useEffect, useState } from "react";
import { listen } from "@tauri-apps/api/event";
import { parseSnapshot, type QuotaAgent } from "./protocol";
import { quotaView, type QuotaRow } from "./quotaView";
import "../components/QuickPanel.css";

// Band colours match the Go ring painter's meaning: green below 70, amber
// from 70, red from 90. The thresholds live in `band`; these are only paint.
const BAND_COLOR = {
	green: "#32D74B",
	amber: "#FFD60A",
	red: "#FF453A"
} as const;

const subText = (row: QuotaRow, clock: boolean): string => {
	const parts: string[] = [];
	if (row.plan) parts.push(row.plan);
	if (row.err) parts.push(row.err);
	else if (row.detail) parts.push(row.detail);
	else if (row.state === "waiting") parts.push(row.busy ? "reading…" : "waiting");
	if (row.reset) {
		const when = clock ? `at ${row.reset.clock}` : `in ${row.reset.countdown}`;
		parts.push(row.resetLabel ? `${row.resetLabel} ${when}` : `resets ${when}`);
	}
	for (const w of row.windows) parts.push(`${w.label} ${Math.round(w.used)}%`);
	return parts.join(" · ");
};

const detailText = (d: QuotaRow["details"][number], clock: boolean): string => {
	const parts = [d.label, d.value];
	if (d.reset) parts.push(clock ? `at ${d.reset.clock}` : `in ${d.reset.countdown}`);
	else if (d.resetText) parts.push(d.resetText);
	return parts.join(" · ");
};

export function QuotaPanel() {
	const [readings, setReadings] = useState<QuotaAgent[]>([]);
	const [now, setNow] = useState(() => new Date());
	// Per-row countdown/clock toggle, in memory only; countdown is default.
	const [clockRows, setClockRows] = useState<ReadonlySet<string>>(new Set());

	useEffect(() => {
		// The Rust event carries the readings array, not a protocol line; wrap
		// it so parseSnapshot stays the single decoder. A payload that does
		// not parse leaves the previous readings in place.
		const unlisten = listen<QuotaAgent[]>("quota-snapshot", (event) => {
			const line = JSON.stringify({ v: 1, op: "snapshot", readings: event.payload });
			const next = parseSnapshot(line);
			if (next !== null) setReadings(next);
		});
		return () => {
			unlisten.then((f) => f());
		};
	}, []);

	useEffect(() => {
		const timer = window.setInterval(() => setNow(new Date()), 1000);
		return () => window.clearInterval(timer);
	}, []);

	const view = quotaView(readings, "quotas", now);

	const toggleRow = (id: string) => {
		setClockRows((prev) => {
			const next = new Set(prev);
			if (next.has(id)) next.delete(id);
			else next.add(id);
			return next;
		});
	};

	return (
		<div className="qp" onClick={(e) => e.stopPropagation()}>
			<div className="qp-body">
				{view.panel.length === 0 ? (
					<div className="qp-empty">Quota unavailable</div>
				) : (
					<ul className="qp-list">
						{view.panel.map((row) => {
							const clock = clockRows.has(row.id);
							return (
								<li key={row.id} className="qp-row">
									<button
										className="qp-row-main"
										onClick={() => toggleRow(row.id)}
										title="Click to toggle countdown / clock"
									>
										<span className="qp-row-text">
											<span className="qp-row-name">{row.id}</span>
											<span className="qp-row-sub">{subText(row, clock)}</span>
											{row.details.map((d, i) => (
												<span key={i} className="qp-row-sub">
													{detailText(d, clock)}
												</span>
											))}
										</span>
										{row.used !== undefined && (
											<span
												className="qp-row-pct"
												style={{ color: BAND_COLOR[row.band ?? "green"] }}
											>
												{Math.round(row.used)}%
											</span>
										)}
									</button>
								</li>
							);
						})}
					</ul>
				)}
			</div>
		</div>
	);
}
