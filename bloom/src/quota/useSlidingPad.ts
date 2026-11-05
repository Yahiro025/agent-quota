import {
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
	type CSSProperties,
	type DependencyList,
	type RefObject
} from "react";
import { useReducedMotion } from "framer-motion";

/**
 * useSlidingPad
 *
 * Generic hook for a measured, transform-safe sliding selection pad.
 * Glides under the selected item using pure layout metrics (offsetLeft, offsetTop,
 * offsetWidth, offsetHeight) unaffected by container transforms or scaling.
 *
 * Public API:
 * @param containerRef Ref to the container element (e.g. .qs-rings grid or .notch-quota-strip flex row)
 * @param selectedKey Key/id of the selected item (e.g. agent.id)
 * @param deps Additional dependencies that may change layout geometry (e.g. [readings])
 *
 * @returns Stable SlidingPadResult:
 *   - `style`: CSS variables (`--pad-x`, `--pad-y`, `--pad-w`, `--pad-h`)
 *   - `ready`: boolean flag indicating initial layout measurement completed
 *   - `instant`: boolean flag disabling transition during initial mount, resize, or deps shifts
 *   - `rect`: measured { x, y, width, height } relative to container
 *   - `padProps`: spreadable props for the pad element (`style`, `data-ready`, `data-instant`, `aria-hidden`)
 *
 * Consumer Wiring:
 * 1. Attach `ref={containerRef}` to the container element (needs `position: relative`).
 * 2. Add an absolute pad element inside the container: `<div className="qs-pad" {...pad.padProps} />`
 * 3. Mark child items with `.selected`, `data-selected="true"`, or `data-key={itemKey}`.
 */

export interface SlidingPadRect {
	x: number;
	y: number;
	width: number;
	height: number;
}

export interface SlidingPadStyle extends CSSProperties {
	"--pad-x"?: string;
	"--pad-y"?: string;
	"--pad-w"?: string;
	"--pad-h"?: string;
}

export interface SlidingPadResult {
	/** Style object with CSS variables --pad-x, --pad-y, --pad-w, --pad-h */
	style: SlidingPadStyle;
	/** True after the first layout measurement has completed */
	ready: boolean;
	/** True when transitions are disabled (initial mount, resize, deps update) */
	instant: boolean;
	/** Measured bounding box of the active item relative to the container */
	rect: SlidingPadRect | null;
	/** Pre-built props ready to spread directly onto the pad element */
	padProps: {
		style: SlidingPadStyle;
		"data-ready"?: string;
		"data-instant"?: string;
		"data-reduced-motion"?: string;
		"aria-hidden"?: true;
	};
}

const useIsomorphicLayoutEffect =
	typeof window !== "undefined" ? useLayoutEffect : useEffect;

/**
 * Finds the currently active item inside a container element.
 * Checks data-key, data-pad-key, data-selected, .selected class, and aria-selected.
 */
export function findSelectedElement(
	container: HTMLElement,
	selectedKey?: string | number | null
): HTMLElement | null {
	if (selectedKey !== undefined && selectedKey !== null && selectedKey !== "") {
		const byKey = container.querySelector<HTMLElement>(
			`[data-key="${selectedKey}"], [data-pad-key="${selectedKey}"]`
		);
		if (byKey) return byKey;
	}
	return (
		container.querySelector<HTMLElement>('[data-selected="true"]') ||
		container.querySelector<HTMLElement>(".selected") ||
		container.querySelector<HTMLElement>('[aria-selected="true"]')
	);
}

/**
 * Extracts offset metrics (layout coords relative to offsetParent, unaffected by transforms).
 */
export function measureElement(el: HTMLElement): SlidingPadRect {
	return {
		x: el.offsetLeft,
		y: el.offsetTop,
		width: el.offsetWidth,
		height: el.offsetHeight
	};
}

/**
 * Compares two SlidingPadRects for field-level equality.
 */
export function areRectsEqual(
	a: SlidingPadRect | null,
	b: SlidingPadRect | null
): boolean {
	if (a === b) return true;
	if (!a || !b) return false;
	return a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height;
}

/**
 * Checks whether container dimensions have changed compared to a previous measurement.
 * Returns false on the initial observation (prev === null) so initial callbacks can be ignored.
 */
export function hasSizeChanged(
	prev: { width: number; height: number } | null,
	next: { width: number; height: number }
): boolean {
	if (!prev) return false;
	return prev.width !== next.width || prev.height !== next.height;
}

export function useSlidingPad<T extends HTMLElement = HTMLElement>(
	containerRef: RefObject<T | null>,
	selectedKey: string | number | undefined | null,
	deps: DependencyList = []
): SlidingPadResult {
	const reduceMotion = useReducedMotion();
	const [rect, setRect] = useState<SlidingPadRect | null>(null);
	const [ready, setReady] = useState(false);
	const [instant, setInstant] = useState(true);

	const prevKeyRef = useRef<string | number | undefined | null>(undefined);
	const prevContainerRef = useRef<HTMLElement | null>(null);
	const rafRef = useRef<number | null>(null);
	const rectRef = useRef<SlidingPadRect | null>(null);
	rectRef.current = rect;

	const selectedKeyRef = useRef(selectedKey);
	selectedKeyRef.current = selectedKey;

	const measure = (forceInstant: boolean) => {
		const container = containerRef.current;
		if (!container) {
			if (rectRef.current !== null) {
				rectRef.current = null;
				setRect(null);
			}
			setReady(false);
			return;
		}

		const target = findSelectedElement(container, selectedKeyRef.current);
		if (!target) {
			if (rectRef.current !== null) {
				rectRef.current = null;
				setRect(null);
			}
			return;
		}

		const nextRect = measureElement(target);

		if (forceInstant) {
			setInstant(true);
			if (!areRectsEqual(rectRef.current, nextRect)) {
				rectRef.current = nextRect;
				setRect(nextRect);
			}
			setReady(true);
			if (rafRef.current !== null) {
				cancelAnimationFrame(rafRef.current);
			}
			rafRef.current = requestAnimationFrame(() => {
				setInstant(false);
				rafRef.current = null;
			});
		} else {
			setInstant(false);
			if (!areRectsEqual(rectRef.current, nextRect)) {
				rectRef.current = nextRect;
				setRect(nextRect);
			}
			setReady(true);
		}
	};

	const measureRef = useRef(measure);
	measureRef.current = measure;

	// Layout effect for selection changes and deps updates
	useIsomorphicLayoutEffect(() => {
		const container = containerRef.current;
		if (!container) {
			if (rectRef.current !== null) {
				rectRef.current = null;
				setRect(null);
			}
			setReady(false);
			prevContainerRef.current = null;
			prevKeyRef.current = undefined;
			return;
		}

		const containerMounted = prevContainerRef.current !== container;
		prevContainerRef.current = container;

		const isInitial = prevKeyRef.current === undefined;
		const keyChanged = prevKeyRef.current !== selectedKey;
		prevKeyRef.current = selectedKey;

		// First measurement must not animate from 0,0.
		// When container mounts or remounts later, measure with instant.
		// Selection changes animate; deps changes without selection change stay instant.
		const forceInstant =
			isInitial || containerMounted || !keyChanged || Boolean(reduceMotion);
		measure(forceInstant);

		return () => {
			if (rafRef.current !== null) {
				cancelAnimationFrame(rafRef.current);
				rafRef.current = null;
			}
		};
	}, [selectedKey, reduceMotion, ...deps]);

	// ResizeObserver on the container: re-measures after container resize or
	// scale changes must NOT animate across the panel.
	useIsomorphicLayoutEffect(() => {
		const container = containerRef.current;
		if (!container || typeof ResizeObserver === "undefined") return;

		let prevSize: { width: number; height: number } | null = null;

		const observer = new ResizeObserver((entries) => {
			const entry = entries[0];
			const currentSize = {
				width: entry ? entry.contentRect.width : container.offsetWidth,
				height: entry ? entry.contentRect.height : container.offsetHeight
			};

			if (!prevSize) {
				// Ignore initial callback fired upon observation
				prevSize = currentSize;
				return;
			}

			if (hasSizeChanged(prevSize, currentSize)) {
				prevSize = currentSize;
				measureRef.current(true);
			}
		});

		observer.observe(container);
		return () => {
			observer.disconnect();
		};
	}, [containerRef.current, ...deps]);

	const isInstant = instant || Boolean(reduceMotion);
	const isReady = ready && rect !== null;

	const style: SlidingPadStyle = {
		"--pad-x": `${rect?.x ?? 0}px`,
		"--pad-y": `${rect?.y ?? 0}px`,
		"--pad-w": `${rect?.width ?? 0}px`,
		"--pad-h": `${rect?.height ?? 0}px`
	};

	const padProps = {
		style,
		"data-ready": isReady ? "true" : undefined,
		"data-instant": isInstant ? "true" : undefined,
		"data-reduced-motion": reduceMotion ? "true" : undefined,
		"aria-hidden": true as const
	};

	return {
		style,
		ready: isReady,
		instant: isInstant,
		rect,
		padProps
	};
}
