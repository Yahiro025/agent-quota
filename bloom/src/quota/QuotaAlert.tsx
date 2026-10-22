import { useEffect, useState } from "react";
import type { QuotaAgent } from "./protocol";
import { alertRows } from "./quotaView";
import "../components/QuickPanel.css";

// Same paint as QuotaPanel: green below 70, amber from 70, red from 90.
const BAND_COLOR = {
	green: "#32D74B",
	amber: "#FFD60A",
	red: "#FF453A"
} as const;

// Compact strip of the urgent readings, layered over the island until every
// reading drops below 90. The strip itself is the dismiss: a click opens the
// Agent Quotas panel (a user action, so it may move the panel).
export function QuotaAlert({
	readings,
	onOpen
}: {
	readings: QuotaAgent[];
	onOpen: () => void;
}) {
	const [now, setNow] = useState(() => new Date());
	useEffect(() => {
		const timer = window.setInterval(() => setNow(new Date()), 30000);
		return () => window.clearInterval(timer);
	}, []);

	return (
		<div
			className="quota-alert"
			role="button"
			title="Open Agent Quotas"
			onClick={(e) => {
				e.stopPropagation();
				onOpen();
			}}
		>
			<ul className="quota-alert-list">
				{alertRows(readings, now).map((row) => (
					<li key={row.id} className="quota-alert-row">
						<span className="quota-alert-name">{row.id}</span>
						{row.reset && (
							<span className="quota-alert-reset">
								resets in {row.reset.countdown}
							</span>
						)}
						<span
							className="quota-alert-pct"
							style={{ color: BAND_COLOR[row.band ?? "red"] }}
						>
							{Math.round(row.used ?? 0)}%
						</span>
					</li>
				))}
			</ul>
		</div>
	);
}
