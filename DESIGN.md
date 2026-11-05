---
name: Agent Quota
description: Seam's quiet matte-black quota overview, painted on the CPU for Windows.
colors:
  surface: "#000000"
  selection: "#101010"
  control-hover: "#141414"
  divider: "#181818"
  text: "#f4f2f6"
  secondary: "#b9b3c1"
  muted: "#a19aa9"
  error: "#f4b2ab"
  stale: "#edc28b"
  usage-normal: "#32d74b"
  usage-warning: "#ffb340"
  usage-critical: "#ff453a"
  ring-track: "#ffffff26"
typography:
  title: { fontFamily: "Inter", fontSize: "12.5px", fontWeight: 500 }
  number: { fontFamily: "Inter", fontSize: "11.5px", fontWeight: 600 }
  body: { fontFamily: "Inter", fontSize: "11px", fontWeight: 400 }
  label: { fontFamily: "Inter", fontSize: "10.5px", fontWeight: 500 }
  caption: { fontFamily: "Inter", fontSize: "10px", fontWeight: 400 }
  reset: { fontFamily: "Inter", fontSize: "8px", fontWeight: 500 }
rounded:
  shoulder: "8px"
  lower-corner: "14px"
  selection: "8px"
  control: "5px"
spacing:
  grid-inset: "16px"
  detail-inset: "24px"
  rail-agent-gap: "6px"
  rail-value-gap: "3px"
  group-row: "22px"
components:
  seam-overview: { backgroundColor: "{colors.surface}", textColor: "{colors.text}", width: "528px", height: "132px" }
  seam-rail: { backgroundColor: "{colors.surface}", width: "528px + 6px per dual-pool agent", height: "44px" }
  compact-tab: { backgroundColor: "{colors.surface}", width: "112px + 6px for dual pools", height: "38px" }
  agent-selected: { backgroundColor: "{colors.selection}", textColor: "{colors.text}", rounded: "{rounded.selection}" }
  header-control-hover: { backgroundColor: "{colors.control-hover}", textColor: "{colors.text}", rounded: "{rounded.control}", width: "24px", height: "22px" }
  refresh-hover: { backgroundColor: "{colors.selection}", textColor: "{colors.muted}", rounded: "{rounded.control}", width: "106px", height: "24px" }
  divider: { backgroundColor: "{colors.divider}", height: "1px" }
  quota-ring: { size: "26px" }
  rail-quota-ring: { size: "28px" }
  logo-tile: { size: "18px" }
---

# Design System: Agent Quota

## Overview

**Creative North Star: "Seam"**

Seam is a compact, quiet overview of seven coding agents at the top centre of the primary monitor. One continuous black surface carries the overview and selected agent's inline detail. The user selected Seam, then **Pure matte black**. The approved [material reference](design/notch-overview/seam-material-variants/02-pure-matte-black.png) and [native contract](design/notch-overview/NATIVE-SEAM.md) record that choice. Only the outer contour derives from the supplied dropdown reference.

The notch contour applies while the visible rail touches the monitor's top edge. Below that edge, Seam uses a rounded floating pill at rest and a rounded overview when open. This is positional, independent of the user's Compact tab setting. Moving back to the top edge restores the notch. The mouse-down anchor stays stable as floating shadow padding appears or disappears. A top appbar such as MyDockFinder or a top-docked taskbar reserves the edge in the work area, so Seam uses the monitor bounds for its top ceiling and draws over the bar as one continuous shape.

The established Orbit identity survives in bundled Inter, authentic marks, fine rings and usage thresholds. The app explicitly selects Seam; detached Orbit rendering remains a compatibility path when `View.Seam` is false. English UI and local reset-time formatting remain. Product constraints come from `AGENTS.md` and the [widget spec](docs/specs/2026-10-13-agent-quota-widget-design.md).

**Key Characteristics:**

- Opaque matte black with restrained tonal selection.
- Stable agent columns with explicit quota-window meaning.
- A continuous notch contour and inline detail on demand.
- CPU rendering at physical DPI and focus-free native interaction.

[widget/seam.go](widget/seam.go) owns Seam layout, hit rectangles, labels and painting. [widget/widget.go](widget/widget.go) owns the public view and shared geometry dispatch; [widget/paint.go](widget/paint.go) owns shared text, ring, mark and group-table painting. [internal/raster/notch.go](internal/raster/notch.go) owns contour and matching containment. `internal/marks` owns identity and ring colours; `internal/text` owns bundled fonts and shaping. `main.go` owns view state, native placement and settings; `seam_settings.go` owns the one-time migration. No CSS or browser adapter is runtime source.

## Colors

Pure black is the reading surface; small tonal lifts identify selection and controls without introducing glass. The frontmatter is normative. The sidecar's synthesized tonal ramps are documentation previews, not additional native colours.

### Primary

- **Normal usage green:** quota arc below 70%.
- **Warning amber:** quota arc and healthy reading value from 70%.
- **Critical red:** quota arc and healthy reading value from 90%.

### Neutral

- **Surface:** fully opaque black throughout rail and overview.
- **Selection:** selected or hovered agent cell and refresh hover.
- **Control hover:** pin/close hover and pinned state.
- **Divider:** inset line above selected detail.
- **Text:** primary numbers, names and emphasized controls.
- **Secondary:** header and inline detail.
- **Muted:** quota basis, reset captions and refresh status.
- **Ring track:** low-alpha white inside the opaque surface.
- **Error / Stale:** warm text for failure detail and last-good age.

**The Usage Colour Rule.** Green, amber and red encode measured usage at the 70% and 90% thresholds. Agent artwork keeps its own identity colours; a mark's colour does not imply quota health.

## Typography

Use the donor's bundled Inter Regular, Medium and SemiBold under its OFL. Numbers lead, names identify, and muted captions explain the basis. There is no display-face role.

### Hierarchy

- **Title:** empty-state heading only.
- **Number:** usage and percent sign share one semibold run in Seam.
- **Body:** `Used` header, inline detail, failure text and empty-state help.
- **Label:** overview agent names.
- **Caption:** quota basis, model table and refresh status.
- **Reset:** narrow resting-rail five-hour caption.

**The Measured Text Rule.** Shape Unicode punctuation, measure glyph widths and elide at rune boundaries. Rasterize at physical DPI instead of stretching an existing image. The narrow rail caption starts with `5h`; hour countdowns use `HhMM` to retain every minute digit within their 37-logical-pixel slot.

## Layout

All sizes are logical pixels before DPI scaling. The seven-agent rail starts at 528 by 44 and adds six pixels for each dual-pool slot. The open overview has a 24-high header, 72-high overview band, a selected-detail area that grows to fit every wrapped row, and a 36-high control footer. The inset body is 16 pixels narrower than the overview. Legacy group disclosure adds 20 plus 22 per group when the reading has no complete detail rows. The empty overview is 280 wide; nonempty overviews use at least 386 or the full rail width, whichever is larger.

Overview cells divide the width minus two grid insets equally between visible agents, in reader-registry order. Each has a 26-pixel ring, neighbouring value, name and actual basis. The rail uses 28-pixel rings, a fixed 31-pixel numeric slot, a three-pixel value gap and six pixels between agents. The selected compact tab is 112 by 38, or 118 by 38 for dual pools; opening expands to the full overview while retaining its horizontal centre. The empty resting rail is 152 by 44.

The default surface begins flush at the primary monitor's top edge. Sixteen pixels at each horizontal side and below reserve exterior shadow, excluded from stated surface sizes. The seven-agent attached bitmap starts at 560 pixels wide at 100% DPI, plus the dual-pool slot space; its height depends on the selected details. Floating surfaces reserve sixteen pixels on every side. A floating overview can open above its rail near the bottom of the work area. Placement clamps the complete bitmap to the Seam area: the monitor's top edge with the work area's horizontal and bottom bounds, so taskbars and docked bars on those sides stay clear. Taskbar Focus keeps the plain work area.

A two-second maintenance tick owns z-order and fullscreen coexistence. It re-asserts topmost over same-band appbars such as MyDockFinder, without showing, moving or activating the window, and it suspends while the context menu is open. It also hides Seam while a borderless fullscreen app covers the primary monitor, collapsing an unpinned overview first, and restores it without activation when that app exits. Detection ignores titled windows, tool windows and shell/desktop surfaces, so an ordinary maximised window and the appbar itself are never mistaken for fullscreen.

For `layout_version < 1`, the one-time Seam migration clears the former position and compact preference, preserves selection and visibility, then records version 1. For `layout_version < 2`, a second one-time step clears only the saved position so an anchor stored at the work area's top edge (`y = 30` under MyDockFinder) re-snaps to the monitor's top edge (`y = 0`), preserving compact, selection, visibility and docking, then records version 2. New or unusable Seam positions recover to top-centre. Later valid saved drag positions and compact preferences survive. Settings store the visible resting-rail anchor rather than its shadow margin; drag release saves the position. Compact toggling preserves the centre.

## Elevation & Depth

The material is flat and opaque; selection is a tonal patch. There is no glass rim, translucent reading region or backdrop blur. A soft exterior black shadow remains: the CPU painter applies `ShadowRound` to the inset body with a 14-pixel radius, 10-pixel blur extent, three-pixel downward offset and maximum alpha 65/255. This is a native mask, not a CSS shadow token. Transparent shoulders, corners and exterior shadow reject input.

**The Solid Reading Surface Rule.** Desktop content never shows through a reading surface. Antialiased edges and the exterior shadow may carry alpha.

## Shapes

The attached signature is a full-width straight upper connection, concave reverse-rounded shoulders, straight inset sides and rounded lower corners. The shoulder inset and curve are eight pixels; lower corners are fourteen. It remains one continuous surface at rest. Floating pills round all four corners to half their height; floating overviews use fourteen-pixel corners. Painting and containment share the same shape and parameters, so floating corners and shadow remain click-through.

Selected cells have eight-pixel corners and a two-pixel inset within their hit rectangles. Header and refresh patches use five-pixel corners. Authentic logo tiles retain 30%-of-size corners. Rings use a 1.65-pixel stroke, start at noon and progress clockwise; ring radius is 42.5% of its allotted size.

## Components

### Overview and selected detail

When a source reports both five-hour and weekly pools, its inner ring and headline percentage show five-hour usage; a concentric weekly ring sits three logical pixels farther out. Each arc independently uses the existing usage thresholds and 1.65-pixel stroke. The logo and inner ring retain their size. Dual-pool rail slots grow by six logical pixels, as does a selected dual-pool compact tab; overview width includes that space. A missing pool is omitted. Grouped sources use the highest reported percentage within each window. Providers without five-hour data retain their existing headline and basis. Stale readings dim both arcs.

The existing native timer requests local and API readings every 30 seconds. Kiro and Antigravity use a five-minute cadence because their CLI process trees have much higher temporary resource costs. Manual Refresh requests every provider immediately. Intervals run from the start of each attempt, preserving the next tick even when a read takes several seconds. One in-flight read per provider and existing deadlines remain. A source's observation time can still be older than the last check when the provider has not published new usage.

Names and quota basis stay visible under rings. Readings with a five-hour pool name `5h`, matching the headline. Other window-backed readings name the highest-usage window; other bases include Credits, Plan, Premium, Unlimited and Models. A five-hour reset explicitly names `5h`. Rail and inline reset derive from `Reading.Reset5h`. Clicking the selected reset target toggles countdown/clock for both, in memory; each launch starts with countdowns. Unknown resets stay absent.

Selected detail shows the reported plan, every quota category, available quantities, each category's reset, and the source observation time when supplied. Rows wrap without eliding facts. A reset moves to a new line when it does not fit beside its quota. A stale reading retains its complete last good details and adds its age and full error. `quota.Detail` owns source facts; the pure `selectedDetailLayout` module owns text positions, height, and control targets behind `Render(View)`.

### Native controls

Pin and Close have independent 24-by-22 header hit targets. Refresh has a 106-by-24 footer target and reports checking, freshness or failure count; its hit target is disabled while a visible reader is busy. Agent selection opens the overview while preserving its pinned state; changing agents clears group/recovery disclosure. Group, recovery and reset actions each have a separate target; group disclosure and five-hour reset split the detail region when both exist. Controls do not initiate dragging.

Hover reveals after 400 ms; leaving closes after 300 ms unless pinned. A resting-rail click pins/unpins. Capture dragging begins after the four-pixel movement threshold; only the rail or unoccupied open header can initiate it. The layered window remains focus-free, including second-launch reveal. Do not add Escape or click-outside dismissal. Painting and interaction stay on the window thread.

### Identity and data states

Known agents use authentic marks from `internal/marks/assets`, with sources and licences retained there. Waiting and Failed show an em dash and Checking or Unavailable basis; failures retain a reason and inline recovery. Stale keeps the last good percentage and a half-alpha arc, with last-good age and latest error in selected detail. Empty state directs users to choose agents through the right-click menu. Reader failures remain isolated. Credentials remain read-only, uncached and absent from logs; readers retain their deadlines and single-flight reads; the native timer requests local/API readings every 30 seconds and Kiro/Antigravity every five minutes.

Complete Antigravity detail shows every enabled bucket and its own reset. Duplicate buckets remain distinct. Legacy readings without complete details can disclose groups in aligned columns. A missing window uses an em dash, never an invented zero. Readers supply structured detail rows, windows, groups, summaries, and resets; the painter does not infer quota data from formatted detail strings.

## Do's and Don'ts

### Do:

- **Do** preserve Seam's continuous contour, opaque material, bundled Inter and authentic marks.
- **Do** identify quota basis and the five-hour window separately when they differ.
- **Do** align hit testing with the painted contour and each independent control.
- **Do** preserve valid saved drag positions and focus-free native behaviour.
- **Do** distinguish native Render captures from live-window evidence.

### Don't:

- **Don't** reintroduce the detached graphite inspector, glass rim or translucent reading surface into Seam.
- **Don't** replace real marks with initials, use bitmap fonts or truncate text by byte count.
- **Don't** invent missing windows or resets, or make one reader's failure block the others.
- **Don't** add GPU, web, office or chat dependencies to this native utility.

The native captures in `.impeccable/review/seam/` document material, geometry and data states at 100%, 125%, 150% and 200% DPI. Gray/light composites are diagnostic backgrounds, not live-desktop captures. `live-window.png` separately records the parent agent's installed-window appearance check at 125% DPI. Independent finish review used source and pure captures, without live-window inspection. This documenter checked source and pure captures and ran no live interaction checks. Subsequent detach/reopen regression checks, full vet and uncached tests pass; native screen-paint assertions require the interactive Windows desktop. The unfinished brand preview experiment is preserved outside compiled packages under `docs/scratch/brand-preview`.


## Taskbar Focus shipped contract (2026-10-17)

On the primary bottom Windows 10 taskbar, an optional native deskband reserves a two-line readout beside the tray. Its authentic mark, independent five-hour/weekly rings, primary percentage and available reset label stay visible. Clicking toggles a matte-black Focus popup above it: 58-pixel agent rail and 468-pixel detail workspace, using Inter and the existing quota facts. Long facts scroll inside the workspace; the rail, explicit close and refresh controls stay fixed. Reset countdown/clock is shared by the panel and readout. Floating Seam modes remain available.

Go owns rendering, credentials, settings and the existing quota schedule; the native DLL receives only bounded premultiplied pixels and emits input/geometry events. It requests its slot from Explorer rather than covering task buttons. The initial Windows toolbar confirmation is retained; ordinary docking collapses/expands the enabled band. Docking intent is saved separately from the floating anchor. Insufficient room/height, unavailable hosts, mismatched native builds and unsupported arrangements fall back to floating mode. Auto-hide suppresses the popup; window styles preserve no activation and no click-outside/Escape dismissal.

This is a legacy Windows 10 shell integration. Physical-pointer, live DPI, crowded-taskbar and actual Explorer-restart acceptance remain unobserved; do not describe renderer or COM tests as those live checks. Revision-specific evidence: [verification](docs/verification/taskbar-focus/verification.md).


Taskbar material correction (2026-10-17): the docked readout has no black pill or rectangular fill. Transparent marks, rings and text sit over Windows' own taskbar material, repainted by the native host. The Focus popup and floating Seam widget keep their existing black surfaces.

Focus pool indicators (2026-10-17): each reported five-hour and weekly value has a thin horizontal usage bar directly underneath. The fill represents used quota, independently per pool, with the existing green/amber/red thresholds. Empty tracks preserve true zero; stale fills are dimmed, and absent pools do not gain a bar.

Readout ownership correction (2026-10-17): deferred native layout acknowledges every changed frame, including negotiations that retain the same shell rectangle. Once the native frame fits, Go dismisses its floating fallback. Regression and real Explorer integration evidence is in [duplicate-readout/RESULT.md](docs/verification/taskbar-focus/production/duplicate-readout/RESULT.md).

Taskbar drag latency correction (2026-10-17): forwarded movement keeps the latest queued position rather than repainting obsolete positions. Release, cancellation, geometry and connection boundaries remain ordered. A live-window backlog reproduction improves from 388.8 ms to 3.5 ms for 120 queued moves; this is input/presentation-path evidence, not a physical-pointer smoothness observation. See [drag-latency/RESULT.md](docs/verification/taskbar-focus/production/drag-latency/RESULT.md).

Keyboard selection (2026-10-17): with the pointer over an open panel, Left and Right select the previous and next displayed agent. Selection wraps and advances once per press. It preserves open and pin state. Modified arrows, shadow pixels, hidden panels, and native menus do not start navigation. The widget retains NOACTIVATE. See [keyboard verification](docs/verification/keyboard-navigation/RESULT.md).
