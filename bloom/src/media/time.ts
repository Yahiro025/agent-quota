// Player clocks: `m:ss`, and `h:mm:ss` once the value passes an hour. The
// remaining time keeps the conventional leading minus sign (U+2212, not a
// hyphen) so the two ends of the scrubber read as one line.
const pad = (n: number) => n.toString().padStart(2, "0");

export function formatTime(ms: number): string {
	const total = Math.max(0, Math.floor(ms / 1000));
	const hours = Math.floor(total / 3600);
	const minutes = Math.floor((total % 3600) / 60);
	const seconds = total % 60;
	return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`;
}

export function formatRemaining(elapsedMs: number, durationMs: number): string {
	return `\u2212${formatTime(Math.max(0, durationMs - elapsedMs))}`;
}
