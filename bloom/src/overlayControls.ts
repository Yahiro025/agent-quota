/**
 * Geometry for the combined right-edge control surface (volume + brightness).
 *
 * The card hangs on the screen's right edge, vertically centered. Each
 * enabled control (volume, brightness) owns one 42px column; the per-app
 * mixer opens to the LEFT of the card — inward, away from the edge — so the
 * expanded surface stays on-screen. Native hit-testing in services.rs uses
 * the same numbers.
 */
export const CONTROL_COLUMN_WIDTH = 42;
export const CONTROLS_CARD_HEIGHT = 196;

/** Enabled overlay flags → the card's CSS width (0 columns → 0). */
export function controlsCardWidth(volumeOn: boolean, brightnessOn: boolean): number {
	return CONTROL_COLUMN_WIDTH * (Number(volumeOn) + Number(brightnessOn));
}

export interface CssRect {
	x: number;
	y: number;
	width: number;
	height: number;
}

/**
 * The card's box in window CSS pixels: flush with the right edge, centered
 * vertically. `windowW`/`windowH` are the overlay window size.
 */
export function controlsCardRect(
	windowW: number,
	windowH: number,
	cardWidth: number
): CssRect {
	return {
		x: windowW - cardWidth,
		y: (windowH - CONTROLS_CARD_HEIGHT) / 2,
		width: cardWidth,
		height: CONTROLS_CARD_HEIGHT
	};
}

/**
 * Where the expanded surface's left edge lands once the mixer panel opens:
 * `cardX - panelWidth`. Must stay >= 0 or the panel would slide off-screen.
 */
export function expandedLeftEdge(windowW: number, cardWidth: number, panelWidth: number): number {
	return windowW - cardWidth - panelWidth;
}
