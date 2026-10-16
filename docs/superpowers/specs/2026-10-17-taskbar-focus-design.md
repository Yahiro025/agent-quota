# Taskbar Focus design

Status: Focus selected and written specification approved by the user on 2026-10-17.

## Intent and selected reference

Dragging Agent Quota onto the taskbar should turn it into a compact status readout beside the system indicators. Usage must remain readable without opening a popup. Clicking the readout opens Focus: a vertical agent rail beside one selected agent's details.

The user selected **Focus** from the structural comparison in [.impeccable/designs/taskbar-structures.html](../../../.impeccable/designs/taskbar-structures.html). This supersedes Menu, Compare, and the earlier near-identical image refinements. The earlier choice of A still defines the compact taskbar readout. This selection approves the layout direction; the prototype's sample values and letter placeholders are not final assets.

## Always-visible readout

- Preserve the authentic selected-agent mark, inner five-hour ring, outer weekly ring, primary percentage, and reset countdown. Keep the existing threshold colours and independent pool values.
- Use two lines for percentage and reset time, matching the user's supplied pill. Fit the taskbar's available height without flattening these into one line or hiding them in a tooltip.
- Omit unavailable pools. Preserve legitimate zero values, unlimited labels, stale data treatment, and meaningful failure states from the existing widget.
- Clicking opens or closes Focus. Selecting an agent in Focus updates both the panel and taskbar readout from the same reading.

## Focus panel

Use a narrow vertical rail for the seven agents, with authentic marks, readable selected state, and accessible agent names. The main area shows the selected agent's name and available quota windows, with used percentages and reset times. Preserve useful provider-specific detail for agents that do not expose both windows; do not manufacture a weekly or five-hour allowance.

Start from the prototype's approximately 468 logical-pixel width and 58-pixel rail. Final dimensions follow actual text metrics and DPI. Anchor the popup above the taskbar readout and constrain it to the work area. Preserve the existing matte-black material, bundled Inter, restrained separators, and CPU rendering. Use the prototype for hierarchy and interaction rather than its substitute typography and marks.

Keep the window focus-free. Preserve the existing no-activation behavior and explicit close/toggle controls; do not introduce Escape or click-outside dismissal. Selection and countdown/clock behavior remain consistent across surfaces.

## Docking behavior

The existing floating/top-edge Seam modes remain available. During a drag, detect the primary taskbar as a separate target rather than clamping the drag before it reaches that target. Show a clear docking preview, dock on release, and allow dragging the readout back into the desktop to restore floating mode. Save docking state separately from the last valid floating position.

Taskbar geometry or DPI changes require repositioning. An unavailable taskbar or failed attachment must recover to a usable floating readout. Docking must not cover task buttons, system icons, the clock, or Show Desktop. Auto-hide must not leave a permanently visible detached readout over a hidden taskbar. Unsupported arrangements fall back to floating mode.

## Native integration constraint

A standard notification icon cannot host the required wide, two-line readout. An independent popup placed over occupied taskbar content does not satisfy this design either.

The exact safe placement mechanism remains a feasibility dependency, not an established capability of the HTML preview. Before implementing docking, verify a pure-Go, out-of-process approach on this Windows 10 machine that provides enough unoccupied space, survives taskbar relayout and Explorer restart, and respects auto-hide. Do not inject a DLL into Explorer or silently resize its private child windows to force room. If these constraints cannot be met, return a concrete limitation and revised integration option for review before implementing a different interaction.

Current code clamps placement to the primary work area in `main.go`; `internal/win/win.go` owns capture-based dragging and native window behavior. Docking requires an explicit placement mode, taskbar geometry discovery, and lifecycle handling. `widget/` remains a pure CPU renderer; no browser or Node process belongs in the installed runtime.

## Refresh and resource budget

Reuse the accepted scheduler: local/API readers every 30 seconds, Kiro and Antigravity CLI readers every five minutes, and manual refresh immediately. Preserve one in-flight read per agent. Docking and Focus must not add another polling loop, browser runtime, or continuously repainting animation. Repaint on data, interaction, countdown changes, or native geometry events.

## Verification and completion

Render closed/open Focus states, all seven selections, missing and zero pools, stale/failure states, and representative DPI settings. Verify drag-to-dock, undocking, saved-state restoration, focus retention in another app, edge clamping, taskbar relayout, Explorer restart, and auto-hide on the real machine. Confirm the system tray and task buttons remain usable.

Run formatting, vet, and the Go suite after implementation. Validate the installed binary and compare idle resource use with the current native app. A successful HTML preview alone is not evidence of native taskbar integration.

## Scope

This change covers the chosen Focus panel and taskbar placement. It does not redesign the quota readers, change refresh intervals, add charts or history, or introduce a second overview layout.
