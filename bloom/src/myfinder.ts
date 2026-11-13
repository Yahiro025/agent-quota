import type { RegionRect } from "./windowRegion";

export interface MyFinderState {
	revision: number;
	available: boolean;
	hidden: boolean;
	height: number;
	color: [number, number, number] | null;
	/** Right edge of MyFinder's menu, physical pixels from the monitor's left. 0: unknown. */
	menuRight: number;
}

export const NO_MYFINDER: MyFinderState = {
	revision: 0,
	available: false,
	hidden: false,
	height: 0,
	color: null,
	menuRight: 0
};

/** Local CSS units, before Bloom's zoom; never scale the whole app to fit. */
export function fittedBarHeight(physicalHeight: number, dpi: number, scale: number): number {
	return Math.max(24, physicalHeight / (Math.max(1, dpi) * Math.max(0.25, scale)));
}

/** All regions, including animation holds/tooltips, obey the narrow top row. */
export function constrainBarRegions(
	rects: RegionRect[],
	strip: RegionRect,
	boundary: number
): RegionRect[] {
	return rects.flatMap((r) => {
		const result: RegionRect[] = [];
		const bottom = r.y + r.height;
		if (r.y < boundary) {
			const x = Math.max(r.x, strip.x);
			const right = Math.min(r.x + r.width, strip.x + strip.width);
			const top = Math.max(0, r.y, strip.y);
			const end = Math.min(bottom, boundary, strip.y + strip.height);
			if (right > x && end > top) result.push({ x, y: top, width: right - x, height: end - top });
		}
		const y = Math.max(r.y, boundary);
		if (bottom > y) result.push({ ...r, y, height: bottom - y });
		return result;
	});
}

export function myfinderIdleHidden(state: MyFinderState, open: boolean, hovered: boolean): boolean {
	return state.available && state.hidden && !open && !hovered;
}

function luminance(rgb: number[]): number {
	const channels = rgb.map((v) => {
		const s = v / 255;
		return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
	});
	return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
}

export function myfinderMaterial(color: MyFinderState["color"]) {
	const bar = color ?? [180, 180, 190];
	const light = luminance(bar) > 0.179;
	const panel = bar.map((v) => Math.round(v * 0.26 + (light ? 240 : 24) * 0.74));
	return {
		light,
		style: {
			// Opaque: backdrop-filter cannot blur another window, so any alpha
			// lets the app behind the panel ghost through its text.
			"--bloom-bg": `rgb(${panel.join(" ")})`,
			"--bloom-bg-expanded": `rgb(${panel.join(" ")})`,
			"--bloom-text": light ? "#202428" : "#ecece7",
			"--bloom-text-muted": light ? "#48515a" : "#a6adae",
			"--bloom-border": light ? "#aeb2bc" : "#353b40",
			"--bloom-group-bg": light ? "#0000000c" : "#ffffff0c",
			"--bloom-accent": light ? "#365d70" : "#a0bdc6",
			"--myfinder-strip-ink": light ? "#000000" : "#ffffff",
			"--usage-normal-text": light ? "#47725a" : "#78ad89",
			"--usage-warning-text": light ? "#80662e" : "#cfb77b",
			"--usage-critical-text": light ? "#91524d" : "#d18c84"
		}
	};
}
