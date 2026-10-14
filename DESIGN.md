---
version: alpha
name: Agent Quota
description: Hover's compact quota island and graphite inspector, drawn on the CPU for Windows.
colors:
  primary: "#ffffff"
  background: "#000000"
  surface: "#131116"
  text: "#ffffff"
  secondary: "#ffffffa6"
  muted: "#ffffff73"
  border: "#ffffff14"
  success: "#32d74b"
  warning: "#ffb340"
  danger: "#ff453a"
typography:
  body:
    fontFamily: "Inter"
    fontSize: "11.5px"
  title:
    fontFamily: "Inter"
    fontSize: "12.5px"
rounded:
  pill: "20px"
  panel: "16px"
  button: "7px"
spacing:
  agent-gap: "12px"
  panel-gap: "8px"
  panel-inset: "16px"
components:
  pill: { backgroundColor: "{colors.background}", textColor: "{colors.text}", rounded: "{rounded.pill}" }
  panel: { backgroundColor: "{colors.surface}", textColor: "{colors.secondary}", rounded: "{rounded.panel}" }
  caption: { textColor: "{colors.muted}", typography: "{typography.body}" }
  divider: { backgroundColor: "{colors.border}", height: "1px" }
  quota-ring: { size: "24px", backgroundColor: "{colors.success}" }
  quota-ring-warning: { size: "24px", backgroundColor: "{colors.warning}" }
  quota-ring-critical: { size: "24px", backgroundColor: "{colors.danger}" }
  logo-tile: { size: "14px" }
  quota-row: { height: "56px" }
---

# Agent Quota design

## Overview

The reference is the quota strip in `../Hover-main/internal/ui/notchview.go`
and the agent tiles in `../Hover-main/internal/ui/marks.go`. The user asked
for the same look as Hover. Preserve its Inter type, small coloured agent
tiles, thin circular tracks, black pill, and quiet graphite panel. This is
a desktop utility for checking seven coding agents while working, not a dashboard.
English UI; display the readers' local date and time formatting.

Runtime ownership is Model B: `widget/widget.go` owns geometry, surfaces and
type roles; `internal/marks/marks.go` owns agent identity and ring colours;
`internal/text` owns the bundled fonts and glyph metrics. This document
mirrors those sources. No CSS or browser adapters are involved.

## Colors

Black isolates the quota strip from whatever application is behind it.
The opaque graphite inspector prevents background text from showing through.
White at full, 65%, and 45% opacity distinguishes numbers, details and captions.
Green, amber and red retain Hover's 70% and 90% usage thresholds.

## Typography

Bundle the donor's Inter Regular, Medium and SemiBold under its OFL.
Pill numbers are 11.5 px semibold; percent suffixes are 10.5 px semibold
and dimmer. Row labels are 12.5 px semibold; details are 11.5 px regular.
Shape Unicode punctuation and measure glyph widths; never truncate bytes.
Rasterize at the monitor's physical DPI rather than stretching a low-resolution image.

## Layout

The pill is 40 logical pixels tall. Each ring is 24 px, followed by a 5 px
gap and a fixed numeric slot; agents are 12 px apart. Stable slots prevent
the strip from jumping when a reading arrives. The inspector is at least
392 px wide, with 16 px insets and 56 px rows that hold two detail lines.
It grows leftwards and above the pill when needed to fit the primary monitor.
Placement uses the primary work area, excluding the taskbar and docked bars.
Unusable saved coordinates recover to the top-right inset; valid drag positions
are preserved. Launching the exe again reveals and pins the existing inspector,
then the new process exits without taking keyboard focus.

## Elevation & Depth

Use subtle one-pixel white edges to define the black and graphite surfaces.
Keep desktop content out of the inspector. Vector antialiasing and premultiplied
alpha make rounded corners and rings smooth without a GPU.

## Shapes

Use a capsule for the quota strip, 16 px inspector corners, and donor logo
tiles with radius 30% of their size. Rings use a 2 px stroke and start at noon.

## Components

Known agents use real vector marks, never initials. Claude, Kiro, Codex and
Cursor use the donor artwork and colours. Copilot uses Octicons; Devin uses
its published mark; Antigravity uses its supplied official PNG mark. Asset
sources and licences live with `internal/marks/assets`.

Waiting and Failed use an em dash, with a visible reason in the inspector.
Stale retains the last good percentage and shows the latest error and age.
No agents enabled shows an instruction to use the right-click menu.
Refresh is a real button with a separate hit rectangle and hover treatment.

Hover opens after 400 ms, leaving closes after 300 ms, and a click pins it.
The window stays focus-free; no Escape or click-outside dismissal. Dragging
saves the position on release. All painting and interaction stays on the window thread.

## Do's and Don'ts

- Match Hover's font, palette, logo tiles and ring proportions.
- Check the pure screenshots and the running layered window at fractional DPI.
- Do not use bitmap fonts, placeholder initials, translucent text-heavy panels,
  or byte-count truncation.
- Do not add the donor's office, chat, GPU toolkit, or other features.
