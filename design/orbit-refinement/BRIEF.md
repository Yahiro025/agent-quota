# Orbit, refined

The user prefers A / Orbit from the comparison board at port 4187 and wants a
macOS level of visual and interaction craft. This study refines the three
existing concepts while keeping Orbit as the recommended foundation.

Audience: someone writing code who needs to know which of seven coding agents
still has quota, without interrupting their work. Success means reading usage
and its window immediately, understanding failures, and opening detail only
when needed. Mode: Operate.

The native application remains Windows-only, CPU-painted and focus-free. The
study uses Inter and the application's licensed artwork. Opaque graphite keeps
desktop text out of reading surfaces. No new product dependencies, credential
access, quota requests, or native application changes are involved.

- A / Orbit keeps seven rings in a stable rail and a vertically aligned inspector.
  Its detail line separates the short and long windows; a reset identifies its
  window. The compact version borrows Focus's selected-agent tab.
- B / Ledger uses linear marks and a table with fixed numeric columns. It answers
  the comparison question directly, with a wider inspector and higher density.
- C / Focus uses a selected-agent tab, a vertical agent index, and explicit quota
  windows. It trades one extra selection for a smaller collapsed footprint.

The inspection view shows one real-size widget in context. Side-by-side view
compares compact tabs and inspectors without shrinking the product. Normal,
critical, stale/failed, checking, empty, light-desktop and reduced-motion states
are part of the study. Data is frozen illustrative data from the original board.
Unknown resets stay unknown. Refresh demonstrates feedback without fetching data.

Pinning, reset toggles, model-group disclosure and a recoverable failure path are
interactive. No Escape or click-outside dismissal is added. Per-agent reset
choices persist across surfaces during this browser session. Notes and the
working selection stay in local browser storage; no data is transmitted.

Run `node design/orbit-refinement/build.mjs`, then
`node design/orbit-refinement/serve.mjs`. Open http://127.0.0.1:4188/.
The original board remains available on port 4187.

## Native handoff

The user explicitly selected A / Orbit in chat after reviewing this refinement.
`approved.json` records that choice. The native implementation now follows it.
Do not interpret a browser's saved working selection as approval of a native
implementation. Structured quota-window and group displays require typed reader
fields; do not parse formatted summary strings inside the painter. Reuse the
same reset instant in the pill and inspector. Map elevation to CPU-painted
shadows and solid surfaces, without adding browser or GPU runtime dependencies.

## Verification, 15 October 2026

- Built artwork from the existing licensed agent sources. Node syntax checks
  passed for `build.mjs`, `serve.mjs`, and `study.js`; `git diff --check` passed.
- Opened and exercised the real browser study. All three panels rendered normal,
  critical, stale/failed, loading, and empty states without internal overflow.
- Checked per-agent reset isolation across all three concepts: Claude changed
  from countdown to 16:38 while Antigravity retained its own countdown.
- Exercised Orbit pinning and refresh feedback, both model-group rows, Focus's
  four Antigravity quota windows, and Devin's inline sign-in recovery guidance.
- Inspected dark and light desktops. The reading surface remains opaque.
- Checked at 1536 px and a 390 px viewport override. The comparison board owns
  its horizontal scrolling; the page and the three inspectors do not overflow.
  Restored the original viewport afterward.
- Corrected Devin's black source SVG for dark tiles. Confirmed computed meter
  colors are amber `rgb(255, 179, 64)` at 70% and red `rgb(255, 69, 58)` at 90%.
- Measured normal Orbit text contrast against its reading surface: agent names
  15.98:1; details and resets 8.70:1; freshness and plan labels 6.53:1.
- Final Orbit geometry is a 528 px full rail and a 386 px inspector. The final
  browser capture is `orbit-desktop.jpg` next to this brief (local generated
  artifact). Browser console errors and native behavior are not inferred from
  these layout checks.

The single Impeccable detector pass reported an Inter warning and design-token
advisories against the application's existing DESIGN.md. Inter is explicitly
required by this brief. This isolated comparison surface intentionally explores
new graphite shades, spacing, and type roles alongside its own board controls;
these are proposed values, not approved edits to the native design system. The
detector result is not described as clean. Reduced-motion behavior is defined in
CSS, but an enabled OS reduced-motion setting was not exercised. At this
prototype-only verification stage no Go sources had changed; native validation
is recorded separately after the user's explicit selection.
