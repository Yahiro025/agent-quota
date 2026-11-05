# Bloom and Agent Quota — Design Spec

Date: 2026-10-22
Status: Locked in conversation on 2026-10-22. Written so implementation can start.
Source checkout: Bloom 3.10.0 at `bloom/`, commit `7c4f5fb` as inspected.
Rollback: the current native Agent Quota window (`go run .` without `--quota-service`) stays.

## Problem

Agent Quota can sit at the top centre of the primary monitor, over MyDockFinder, as a Seam notch. Bloom is already a top-edge notch with media, calendar, timer, shelf, Wi-Fi, Bluetooth, volume, brightness, shortcuts, weather, system widgets, themes, scaling, notch modes, settings, autostart, and edge HUDs. The two products should be one notch. Bloom's dock and its taskbar replacement stay out, because a dock is already installed.

## Experience

Bloom's visual design and opening behavior stay. A click opens the remembered panel, or Bloom's default panel when nothing is remembered. Agent Quotas and an optional Overview are additional panels in that same navigation, styled like the panels that already exist. Music, timer, shelf, and the edge HUDs keep their current interactions.

Quota is absent from the resting notch. It appears when the Agent Quotas panel is opened, or when an urgent alert expands the notch. Urgent means a reported usage at or above 90, or a timer that has completed. Usage from 70 up to but not including 90 stays inside the Agent Quotas panel and does not expand the notch. The bands are the existing ones: green below 70, amber from 70, red from 90.

An alert shows on top of the current panel and then returns to that panel. It does not replace the panel the person had open. Alerts wait while any of these are in progress: typing in a text input, dragging, Bluetooth pairing, or a connection attempt that has not finished.

Passive motion and clicks do not take keyboard focus. A deliberate click on a text input may focus that input. Leaving the input returns the notch to focus-free behavior.

The native taskbar view from Agent Quota is not part of this product. One presentation: the notch.

## Retained Bloom capabilities

- Media controls and the visualizer
- Calendar and timer
- File and text shelf
- Wi-Fi and Bluetooth
- Volume mixer and brightness
- Shell shortcuts
- Weather and system widgets
- Themes, scaling, and notch modes
- Settings and autostart
- Edge HUDs

## Excluded

- Bloom's dock window, dock startup, AppBar registration, taskbar hiding, taskbar recovery, and taskbar shutdown restoration
- Bloom's update service and announcement feed
- A second presentation mode (native taskbar readout, Bloom dock)

Bloom source notices and asset notices stay with the reused files.

## Architecture

One Tauri app, derived from the Bloom checkout, owns windows, settings, Windows controls, and which panel or alert is visible. A packaged Go helper owns the seven existing quota readers and their polling. The helper creates no window. If the helper stops, quota shows the last snapshot or an unavailable state, and Bloom's other tools keep working.

The helper is the existing `agentquota` module started with `--quota-service`. That mode does not detach, does not take the `Local\AgentQuotaRunning` mutex, and does not open a window, so the native rollback app can run at the same time.

Credentials stay inside the existing readers. The pipe carries usage, freshness, errors, and reset instants. It never carries tokens, keyring contents, or raw credential files.

### Protocol

Version: `1`. Transport: the parent's stdin and the child's stdout, one JSON object per line, UTF-8, compact encoding. The child's stderr is logs only and is not part of the protocol.

Parent to child:

- `{"v":1,"op":"hello"}`
- `{"v":1,"op":"refresh","id":"all"}`
- `{"v":1,"op":"refresh","id":"<agent>"}` where `<agent>` is one of `claude`, `kiro`, `codex`, `cursor`, `copilot`, `devin`, `antigravity`
- `{"v":1,"op":"shutdown"}`

Child to parent:

- `{"v":1,"op":"ready","agents":["claude","kiro","codex","cursor","copilot","devin","antigravity"]}` in that order
- `{"v":1,"op":"snapshot","readings":[ ...seven agents... ]}`
- `{"v":1,"op":"bye"}`
- `{"v":1,"op":"error","message":"<reason>"}`

A snapshot reading uses these fields:

- `id` (string), `state` (`waiting`, `ok`, `stale`, or `failed`), `busy` (bool)
- `used` (number, present only when a percentage exists)
- `detail`, `err`, `plan`, `summary`, `resetLabel` (strings, omitted when empty)
- `reset5h`, `observedAt`, `lastGoodAt` (RFC3339Nano UTC, omitted when zero)
- `windows`: `{label, used}[]`
- `groups`: `{name, windows}[]`
- `details`: `{label, value, resetAt, resetText}` with `resetAt` RFC3339Nano UTC omitted when zero, `resetText` omitted when empty

`state` comes from `quota.Book.StateOf`. `stale` keeps the last good reading in `used` and the latest failure in `err`. `failed` has no `used`. Countdown and clock text are not sent. The UI formats both from `reset5h` for the five-hour instant, and from each detail's `resetAt` when that detail has one. The same instant feeds both formats.

`hello` replies with `ready`, then one snapshot of the current book, then starts a refresh of every enabled agent. `refresh` with `all` forces every enabled agent. `refresh` with one id forces that agent only. Unknown `v`, unknown `op`, or an unknown refresh id produces `error` and leaves the process running. A blank line is ignored. `shutdown` replies with `bye` and exits the service loop. The service polls with the existing schedule: 30 seconds, and the existing longer gap for Kiro and Antigravity. One read per agent stays in flight.

### Placement and fullscreen

The notch window stays on the primary monitor's physical origin (`primary_monitor` position), full monitor width, topmost, so it overlays a top appbar such as MyDockFinder. It must not be clamped to `SPI_GETWORKAREA`.

While a fullscreen app covers the primary monitor, the notch retracts and does not take focus; a top-edge dwell reveals it in every notch mode, and it retracts again after the hover grace. Exclusive D3D fullscreen (`SHQueryUserNotificationState` 3) hides the notch window outright, because a topmost window can knock the app out of exclusive mode. It shows again when that condition ends. Detection follows the rules already locked for Agent Quota: unambiguous quiet-hours notification states for exclusive fullscreen and presentation mode, plus a borderless foreground window covering the primary monitor, excluding tool windows, no-activate appbars, and the shell class names listed in `docs/superpowers/specs/2026-10-21-top-edge-seam-design.md` section 4.

### Focus assist

The circular control at `bloom/src/App.tsx` that calls `setDndActive` only changes local React state. No supported write path for Windows Focus assist was verified. The control becomes a shortcut that opens `ms-settings:quiethours`, using the same launch style as `open_battery_saver_settings`. Its visible name is "Focus assist". It does not draw an "on" state, because this app is not the source of that state.

### Helper lifecycle

Bloom spawns one helper. The installed app is `bloom.exe` alone: the helper bytes are embedded in it and written to `%LOCALAPPDATA%\com.sehaz.bloom\agentquota.exe` before spawn. The helper remains its own process so a reader failure cannot close the notch. Unexpected exit keeps the last snapshot and starts the helper again. Quota UI treats a missing helper as unavailable without affecting other panels.

## Validation

- Unchanged Bloom panels keep their typography, geometry, transitions, navigation, and settings.
- Music, timer, shelf, and a quota alert can be active together. The alert restores the previous panel.
- Foreground stays put during passive use. A clicked text input accepts typing and paste, including a password field, and focus returns when editing ends.
- File drag in and out, previews, persistence, a missing file, a Wi-Fi failure, and Bluetooth pairing still behave as they do in Bloom.
- The notch stacks above MyDockFinder on the primary monitor, hides for fullscreen, survives a DPI change and a restart, and a second Bloom launch reveals the running instance. The installed dock and the Windows taskbar are not hidden or registered as app bars.
- Quota freshness, grouped resets, a reader failure, helper exit, and helper recovery match this protocol. Existing Go tests stay green. Bloom's frontend typecheck and Rust checks stay green.
- Resource use is not claimed against unmodified Bloom until it is measured on this machine.

## Sequencing

The helper protocol is usable without the notch. Dock removal is usable without quota. The quota panel needs the protocol. Alerts need the panel. Overview, the Focus assist shortcut, and disabling updates are independent of the pipe once the notch shell still starts.
