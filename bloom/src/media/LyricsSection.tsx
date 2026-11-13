import { useEffect, useRef } from "react";
import { invoke } from "@tauri-apps/api/core";
import { activeLyricLine, type Lyrics } from "./lyrics";
import "./LyricsSection.css";

// A fixed three-line viewport: the notch grows once per track, never per line.
export function LyricsSection({
	lyrics,
	generation,
	positionMs
}: {
	lyrics: Lyrics;
	generation: string;
	positionMs: number;
}) {
	const boxRef = useRef<HTMLDivElement>(null);
	const lineRefs = useRef<(HTMLElement | null)[]>([]);
	const active = lyrics.synced ? activeLyricLine(lyrics.lines, positionMs) : -1;

	useEffect(() => {
		const box = boxRef.current;
		const line = active >= 0 ? lineRefs.current[active] : null;
		if (!box || !line) return;
		const behavior = matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";
		box.scrollTo({ top: line.offsetTop - (box.clientHeight - line.offsetHeight) / 2, behavior });
	}, [active]);

	const seek = (ms: number) => {
		if (!lyrics.synced) return;
		void invoke("media_action", { generation, op: "seek", positionMs: ms }).catch(() => {});
	};

	return (
		<div
			ref={boxRef}
			className="cmp-lyrics"
			role="list"
			aria-label="Lyrics"
			onClick={(e) => e.stopPropagation()}
			onWheel={(e) => e.stopPropagation()}
		>
			{lyrics.lines.map((line, index) => {
				const className = `cmp-lyric${index === active ? " is-active" : ""}${
					lyrics.synced ? " is-seekable" : ""
				}`;
				const ref = (el: HTMLElement | null) => {
					lineRefs.current[index] = el;
				};
				return lyrics.synced ? (
					<button
						key={`${line.t}-${index}`}
						ref={ref}
						className={className}
						aria-current={index === active ? "true" : undefined}
						title="Play from this line"
						onClick={() => seek(line.t)}
					>
						{line.text}
					</button>
				) : (
					<span key={`${line.t}-${index}`} ref={ref} className={className}>
						{line.text}
					</span>
				);
			})}
		</div>
	);
}
