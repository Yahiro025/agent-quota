# Centered housing notch

Date: 2026-10-28. Accepted with Impeccable. Operate refinement of the existing Bloom notch.

## Housing

The notch stays a centered black island, flush to the top of the primary monitor, only as wide as its contents. Resting shape: opaque black, 18px bottom radius. An open panel hangs under that same housing, centered, matte black, and wider only when its content needs the room. The window is not a menu bar and does not paint a strip across the monitor. Cursor keeps the top-right corner.

No clear ears. No overflow chevron. EdgeSurface stays the centered housing.

## Resting pill, left to right

1. Live activity: the music waveform, when a track is the compact live activity.
2. Quota: the pinned agent's ring mark, or the busiest reading when nothing is pinned.
3. Weather.
4. The clock.
5. Battery.
6. One live-state dot, shown when an update is available.

A timer in the pill replaces the clock and moves music to its bubble. Hover only peeks. Clicking the pill opens the last panel. Clicking the clock still opens the calendar once the notch is open. Peek, smart-hide, pin, and wheel-between-panels stay.

## Quota

The quota panel is unchanged: flat rows, a bar and percent beside the label, reset on the right, footer "Just checked", no "1 failed".

## Shutdown

Bloom still replaces the Windows taskbar. Closing it goes through `restore_taskbar_and_exit` by posting `WM_CLOSE` to the top-level window titled exactly `bloom`. The floating `agentquota.exe` is not launched. The `--quota-service` helper may be stopped only after Bloom has quit, so a new build can replace a locked helper.
