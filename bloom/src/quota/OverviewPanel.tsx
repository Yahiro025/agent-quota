import { Gauge, Inbox, Music, Timer } from "lucide-react";
import "../components/QuickPanel.css";

// The overview lists the notch's live activities; it is never the panel a
// click opens. Opening still uses the remembered panel or this default,
// declared once so the check "overview is not the default" has a source.
export const DEFAULT_PANEL = "calendar";
export const defaultPanel = (): string => DEFAULT_PANEL;

// Row labels in display order. Each input already answers "is it on":
// a media session, a live timer, a filled shelf, an urgent quota reading.
export function overviewRows(input: {
	music: boolean;
	timer: boolean;
	shelfCount: number;
	urgent: boolean;
}): string[] {
	const rows: string[] = [];
	if (input.music) rows.push("Music");
	if (input.timer) rows.push("Timer");
	if (input.shelfCount > 0) rows.push("Shelf");
	if (input.urgent) rows.push("Quota");
	return rows;
}

const ROW_ICON: Record<string, typeof Music> = {
	Music: Music,
	Timer: Timer,
	Shelf: Inbox,
	Quota: Gauge
};

// Rows are already the live set, so the panel only draws and forwards a
// pick; the caller maps the row name onto the activity's own view.
export function OverviewPanel({
	rows,
	onOpen
}: {
	rows: string[];
	onOpen: (row: string) => void;
}) {
	return (
		<div className="qp" onClick={(e) => e.stopPropagation()}>
			<div className="qp-body">
				{rows.length === 0 ? (
					<div className="qp-empty">Nothing on right now</div>
				) : (
					<ul className="qp-list">
						{rows.map((row) => {
							const Icon = ROW_ICON[row];
							return (
								<li key={row} className="qp-row">
									<button className="qp-row-main" onClick={() => onOpen(row)}>
										<span className="qp-row-icon">{Icon && <Icon />}</span>
										<span className="qp-row-text">
											<span className="qp-row-name">{row}</span>
										</span>
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
