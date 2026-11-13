export interface LyricLine {
	t: number;
	text: string;
}

export interface Lyrics {
	track_id: string;
	synced: boolean;
	lines: LyricLine[];
}

/// The line being sung at `positionMs`: the last one whose start has passed.
/// Binary search because a long sheet ticks four times a second.
export function activeLyricLine(lines: LyricLine[], positionMs: number): number {
	let low = 0,
		high = lines.length - 1,
		found = -1;
	while (low <= high) {
		const mid = (low + high) >> 1;
		if (lines[mid].t <= positionMs) {
			found = mid;
			low = mid + 1;
		} else {
			high = mid - 1;
		}
	}
	return found;
}
