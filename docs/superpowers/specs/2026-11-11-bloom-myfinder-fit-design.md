# Bloom fitted into MyFinder

Status: implementation authorized by the user's “nice, implement it” after reviewing the integrated MyFinder material concept. Code and automated checks are complete; live native interaction verification and installation are pending.

## Latest approved material (supersedes earlier black-housing references)

The final accepted concept is a transparent centre status strip using MyFinder's foreground contrast, with a matching, lightly translucent menu beneath it. The resting black housing and opaque-black menu requirements in the original review below are superseded for MyFinder integration. Standalone Bloom keeps its existing theme. MyFinder still owns the full-width bar; Bloom does not paint a replacement bar or change MyFinder preferences.

The implementation observes MyFinder's validated native window and a few visible material pixels outside Bloom's centre footprint. It retains only transient RGB values, with no captured image or derived settings file. When sampling is unavailable it retains the last valid material for the same window/process/monitor, or a neutral light fallback. Native visibility, cloaking, zero alpha, and an off-screen bar position gate idle Bloom. Real auto-hide/reveal behavior must still be checked on the installed MyFinder build.

The user explicitly requested implementation of this reviewed concept; the earlier review/plan approval gates at the end of this historical specification are resolved by that instruction. Implementation notes and verification are recorded in `docs/superpowers/plans/2026-11-11-bloom-myfinder-fit.md`.

## Intent and approved direction

Fit Bloom's existing notch experience into the center of Bennett's MyFinder bar. Keep MyFinder's application menus and system controls usable. Keep Bloom's existing panels and status content, including the clock inside the housing.

The user has approved these outcomes:

- D1: retain MyFinder's bar and integrate Bloom at its center.
- D2: match the resting black housing to MyFinder's detected height, subject to readable sizing.
- D3: keep the strip inside the bar at its resting width; widen expanded panels only beneath the bar. Painting and native input geometry must agree.
- D4: hide idle Bloom when MyFinder hides. When MyFinder returns, permit Bloom's existing Fixed/Smart/Peek behavior again. Preserve a pinned or actively used panel's existing close rules; fullscreen suppression takes priority.

The user's subsequent instruction, "make it fit," focuses the remaining work on those outcomes. The sizing and fallback details below are proposed specifications of that fit, to be reviewed together in this document.

## What already exists

Bloom already pins its main window to the monitor's physical top edge above MyFinder. `services.rs` owns the native hooks, positioning, fullscreen state and click-through decisions. `App.tsx` owns the status cluster and panel state. `windowRegion.ts` and `window_region.rs` connect frontend geometry to native clipping. `EdgeSurface.tsx` and its CSS own the attached housing material and contour.

The current status row is 36 logical pixels high. One animated parent width currently grows both the panel and the strip at the top edge. That model cannot implement D3 by adjusting height alone.

Read-only inspection on 2026-11-11 found the running `Dock_64` process's `MyFinderApp` at physical rectangle `(0,0)-(1920,30)`, reporting 120 DPI. Its height is therefore 24 Windows logical pixels. Windows UI Automation exposed zero immediate children, so this probe establishes bar geometry, not individual menu or icon positions. The supplied screenshot leaves the center clear.

Use Bloom's bundled Inter, existing status typography, authentic agent marks, matte-black housing and current usage colors. The current learned preferences and Bloom source take precedence over older native-widget descriptions in `DESIGN.md`.

## Layout and units

```text
Monitor top
[MyFinder application / menus] [Bloom status strip] [MyFinder controls]
                                      |
                         [wider Bloom panel below]
```

Keep the current target monitor; this change does not add monitor selection. Select a MyFinder bar only when it intersects that monitor's top edge and covers its center. Read its bounds through Win32; do not infer a bar from the Windows work-area inset alone.

Keep physical screen pixels, webview CSS pixels and Bloom's user scale explicit. Let `B` be the bar's physical height, `D` the webview display scale and `S` Bloom's existing UI scale. The strip's local height is `max(B / (D * S), 24)` logical pixels. At the inspected 125% display scale and default Bloom scale, this produces exactly 30 physical pixels.

The 24-pixel floor preserves an 11-pixel minimum status text size, a 16-pixel mark and vertical breathing room. Keep the existing larger status text where it fits; do not scale the whole application down to force a fit. Respect an increased Bloom UI scale: when a bar is too short, the readable strip may extend below it. Start the panel beneath the greater of strip height and bar height. This is the explicit readability exception approved in D2.

Width continues to follow the enabled status content and existing horizontal spacing. Preserve the activity, agent mark, weather, clock/timer, battery and update-indicator rules. Do not add an overflow chevron or paint a full-width background. Limit the existing bottom radius to half the fitted strip height so the contour remains valid.

The panel retains each existing panel's preferred content width and user scale. Its body begins below the bar/strip boundary and grows horizontally around the same center. It remains visually connected to the narrow strip, with existing rounded body corners. Keep shadows below that boundary; no wide shadow or transparent hit rectangle may spread across MyFinder's bar row.

Clamp the entire visible body, including shadow, to the host monitor. When content exceeds available height, scroll inside the body. On an exceptionally narrow viewport or large user scale, allow horizontal scrolling of the overflowing status cluster while retaining the clock/timer as a visible anchor. Do not silently remove enabled widgets or truncate quota facts. This uses ordinary CSS scrolling, not a new navigation system.

## Geometry and input ownership

Compute one frontend layout containing the narrow strip and the below-bar body. Use it for visual placement and for the geometry reported to Rust. Extend the existing geometry owner instead of giving each panel its own MyFinder calculations.

Native clipping and click/hover decisions must both respect the split silhouette. A bounding rectangle around the entire expanded shape is insufficient: it would again intercept neighboring MyFinder controls. Existing rectangular hover padding and the 700ms held-region mechanism must not restore a wide top strip during animation. Any retained old region must obey the narrow bar-row boundary too.

Keep the same boundary through opening, closing, panel switching, tooltips, file dragging and DPI changes. Include auxiliary bubbles and tooltip surfaces only where they are actually drawn. Preserve pointer continuity from the strip into the body; moving through the connection must not briefly close the panel.

Keep the window focus-free. Agent-mark clicks cycle through snapshot order, including failed and waiting agents, without opening the panel. Other pill clicks retain their current panel action. Keep pinning, hover delays, wheel navigation, explicit close and file-drop behavior. Do not add Escape or click-outside dismissal.

## MyFinder observation and visibility

Rust owns one read-only, in-memory observation of MyFinder's window identity, target monitor, valid bar bounds and visibility. Extend the existing native event/reconciliation path. Event callbacks request reconciliation; they do not independently write competing snapshots. Use the existing two-second maintenance cadence for missed-event recovery, not a second high-frequency discovery loop.

Validate positive geometry, monitor association and current window/process identity. A reused handle or display change invalidates the old association. Publish only changed observations. The frontend receives the latest observation initially and on change; it does not query MyFinder every animation frame.

Treat visibility as visible, hidden or unknown. Use actual window visibility and on-monitor geometry, including any supported cloaking/transparency signal needed by the installed MyFinder. Do not confuse the mouse leaving the bar with a hidden bar. The actual auto-hide transition must be verified live before declaring D4 working; the present read-only probe observed only the visible state.

When MyFinder is known hidden, gate off idle Bloom even if its mode is Fixed. While MyFinder is visible, retain Bloom's existing mode behavior. A known-hidden bar also gates event peeks and edge dwell for idle Bloom; a mere top-edge pass must not reveal Bloom independently. When MyFinder is visible again, existing Smart/Peek rules decide whether Bloom appears. An active or pinned Bloom panel retains its normal close/hover rules; after it becomes idle, apply the gate. Existing fullscreen suppression remains authoritative.

Keep the last valid bar height across a hidden interval so reveal does not resize the strip. If MyFinder is absent, an observation fails, or identity cannot be validated, use the existing standalone Bloom behavior rather than treating an unknown state as hidden. Reconcile when MyFinder returns. Do not persist derived bar geometry or alter MyFinder's settings, visibility, window parent, position or process.

## State coverage

| State | What the user sees |
| --- | --- |
| MyFinder visible, Bloom idle | The fitted strip, when allowed by the existing Bloom mode |
| MyFinder visible, panel open | A narrow strip connected to the wider body beneath the bar |
| MyFinder hidden, Bloom idle | No Bloom strip, peek or invisible input blocker |
| MyFinder hides during active/pinned use | The existing panel remains usable until its usual close rules apply |
| MyFinder returns | Its measured-height strip becomes eligible under the current Bloom mode |
| MyFinder absent or observation unknown | Existing standalone Bloom behavior; no permanent blank notch |
| Fullscreen | Existing fullscreen rules take priority over MyFinder eligibility |
| Quota loading | Existing waiting/checking state; other readers remain independent |
| Quota error/partial/stale | Existing local failure reason and last-good reading; no global failure count |
| No visible quota agents | Existing settings guidance/empty hint |
| Oversized panel | Body scrolls within the monitor; strip and explicit controls remain reachable |

Quota rows remain flat: label, usage bar and percent, reset on the right. Footer: "Just checked". Retain countdown plus exact reset time in the hover card. Retain current reset-format behavior and per-reader deadlines. The integration changes no quota facts or credential handling.

## User journey

| Step | User action | Intended experience | Design support |
| --- | --- | --- | --- |
| 1 | Glance at MyFinder | Bloom belongs in the bar | Measured height, centered black strip, existing status content |
| 2 | Click an agent mark | Predictable agent cycling | Existing cycle action; no accidental panel open |
| 3 | Open quota/media/calendar/timer/shelf | Familiar content with room to operate | Wider body below the unchanged strip width |
| 4 | Use an adjacent MyFinder control | No blocked click or lost focus | Split visual, shadow and native input geometry |
| 5 | Let MyFinder hide | One coordinated resting surface | Idle Bloom follows; active work is not interrupted |
| 6 | Resume after hide, restart or display change | Stable size and recoverable controls | Cached valid height, observation reconciliation, standalone fallback |

The five-second impression is a fitted bar, the five-minute experience preserves current actions, and long-term use requires no second settings file to keep synchronized.

## Design system and accessibility

This is an Operate surface. Reuse the current Inter hierarchy, opaque black reading surface, restrained tonal selection, flat quota rows and usage thresholds. No new palette, glass effect, decorative gradient, card grid or brand treatment is introduced. Preserve at least 4.5:1 contrast for ordinary text and existing semantic accessible names; never communicate usage health by color alone.

Status targets may fill the fitted row and extend horizontally within their own space. They must never expand invisibly over MyFinder. This is a dense desktop pointer surface; it does not claim 44-pixel touch targets. Preserve accessible labels, button semantics and current screen-reader exposure. Avoid duplicate focusable status content when visually separating the header/body.

Respect reduced motion for the new strip/body transitions: no spring travel or intermediate geometry is required when reduced motion is enabled. Update the final painted and native regions together. Check display scaling at 100%, 125%, 150% and 200%, plus Bloom's configured UI scale. Recompute once from authoritative geometry after a scale/monitor change, rather than retaining unrelated CSS and physical sizes.

The existing non-activating notch, including a quota badge with `tabIndex={-1}`, does not provide complete conventional keyboard navigation. This fit change must not claim to solve that limitation, remove existing labels, or steal keyboard focus. A new keyboard interaction model is outside this fit specification.

## Separate state before serialization

MyFinder owns its settings and windows. Bloom owns its preferences and presentation. The quota helper owns reader state and publishes snapshots through the existing pipe. Join these facts only at Bloom's presentation boundary; do not create a shared configuration file, cross-process writer or additional quota cache.

MyFinder observation has one owner in Rust and is transient. Existing Bloom preference changes continue through `save_setting`, whose backend already serializes the read/modify/write transaction and atomically replaces settings. Its file watcher uses the same lock. Reuse that protection where sharing is real; do not add a MyFinder/Bloom lock around independent state.

## NOT in scope

- Replacing, reparenting into, injecting into or restyling MyFinder. The integration uses Bloom's existing top-level window.
- A new MyFinder plugin/API, screenshot/OCR occupancy detector, or automatic rearrangement of app menus and tray controls. No reliable individual occupancy signal was exposed by the probe.
- A guarantee against every future crowded MyFinder configuration. The current supplied layout has center space; expansion must not consume additional bar-row width.
- New quota readers, duplicated system controls, a full-width Bloom menu bar or the floating Go quota window.
- A new monitor picker, touch layout, global shortcut system or conventional keyboard navigation model.
- Changes to unrelated current working-tree edits, including taskbar/dock work. Keep the installed helper running and use the existing safe shutdown path if later deployment requires closing Bloom.

## Verification required before calling the implementation complete

Use targeted pure geometry tests for physical/CSS/user-scale conversion, a 30px bar at 125%, the readability floor, narrow/negative-origin monitor bounds and split strip/body clipping. Regression tests must prove that adjacent bar-row points remain outside Bloom before, during and after expand/collapse, including held regions and reduced motion.

Test the visibility state machine with visible, hidden, unknown, absent, pinned, active and fullscreen inputs. Verify actual MyFinder auto-hide/reveal live, including whether it changes native visibility, position, alpha or another observable state. If no reliable signal is observable, report that limitation; do not substitute pointer position or claim D4 passed.

Run the frontend build/typecheck, relevant geometry/quota tests, and Rust locked check/tests for the touched native code. Observe actual panel expansion, adjacent MyFinder clicks, agent cycling, hover-reset detail, every existing panel, user scale changes and MyFinder restart. Do not launch the floating quota window. Capture live evidence separately from conceptual mockups.

## Implementation Tasks

These are review-derived work packages, not the detailed implementation plan. Estimates assume reuse of the existing geometry and native event paths.

- [ ] **T1 (P1, human: ~2h / Codex: ~30min)** — Extend native observation with validated MyFinder bounds/visibility and change-only publication. Source: D2/D4. Files: `bloom/src-tauri/src/services.rs`, `types.rs`, existing initialization/command registration as needed. Verify native observation/state tests and live MyFinder hide/reveal.
- [ ] **T2 (P1, human: ~2h / Codex: ~30min)** — Compute fitted strip and below-bar body geometry with explicit unit conversion. Source: D2/D3. Files: `bloom/src/windowRegion.ts` or a focused helper beside it, `bloom/scripts/windowRegion.test.ts`. Verify DPI, user scale, readability and viewport boundaries.
- [ ] **T3 (P1, human: ~2h / Codex: ~30min)** — Render the split housing and match native clipping/hover/click regions throughout transitions. Source: D3. Files: `bloom/src/App.tsx`, `App.css`, `components/EdgeSurface.*`, `windowRegion.ts`, existing native region/interaction owners. Verify adjacent-bar exclusions, pointer continuity and all panels.
- [ ] **T4 (P1, human: ~1h / Codex: ~20min)** — Apply the MyFinder idle gate to existing visibility modes with active/pinned and fullscreen precedence. Source: D4. Files: `bloom/src/App.tsx`, relevant pure state helper/tests. Verify the state table and real auto-hide/reveal.
- [ ] **T5 (P1, human: ~1h / Codex: ~20min)** — Complete build checks and live fit verification; retain proof and rollback information for any later deployment. Source: D2-D4. Verify the targeted checks and interactions listed above.

No separate TODO file changes are proposed. No dependency is required by this design.

## Design Review Completion Summary

All seven passes were evaluated against the fit scope. Scores describe specification coverage, not tested software or release readiness.

| Item | Before | After | Finding or evidence |
| --- | --- | --- | --- |
| System audit | — | — | Existing Bloom design and native integration reused; unrelated diff excluded |
| Step 0 impression | 4/10 | — | Initially only intent and partial placement existed |
| Pass 1: information architecture | 5/10 | 9/10 | D2/D3 define narrow bar strip and below-bar hierarchy |
| Pass 2: states | 4/10 | 9/10 | D4 plus visible/hidden/unknown/active/fullscreen coverage |
| Pass 3: journey | 5/10 | 9/10 | Six-step storyboard carries existing actions through fit/hide/recovery |
| Pass 4: generic-design risk | 9/10 | 9/10 | Operate; no hard-rejection pattern. Existing flat UI reused |
| Pass 5: design-system alignment | 7/10 | 9/10 | Existing typography/material retained; fitted radius and split contour specified |
| Pass 6: responsive/accessibility | 4/10 | 8/10 | Units, readable floor, overflow, contrast and reduced motion specified; existing keyboard limitation disclosed |
| Pass 7: decision register | — | — | D1-D4 resolved in conversation; written specification review still pending |
| Overall, minimum of passes 1-6 | 4/10 | 8/10 | Design coverage only; no implementation claim |

Pass 4 litmus: product identity yes (existing black housing and authentic marks); visual anchor yes (center strip); scanability yes (status then panel); each region has one job yes; decorative cards necessary no; motion hierarchy yes (body unfolds below a stable strip); works without decorative shadow yes. No hard-rejection hits.

Mockups: two conceptual HTML studies were produced, four layouts total. The bar-height direction and below-bar expansion direction were selected in conversation; final pixels have not been approved or verified live. References are under `C:/Users/Bennett/.gstack/projects/Yahiro025-agent-quota/designs/myfinder-20261111/` as `integration-review.html` and `expansion-study.html`. The final design combines those selected directions with D4.

Not-in-scope items and existing-component reuse are recorded above. D2/D3/D4 contribute three behavior decisions; D1 is the integration scope decision. There are no separately deferred TODO proposals. Native auto-hide signal verification and full keyboard-navigation limitations remain explicit practical limits, not completed tests.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
| --- | --- | --- | --- | --- | --- |
| Design review | `/plan-design-review` | Fit, interaction and design gaps | 1 for this design | Issues open | Coverage 4/10 to 8/10; D1-D4 recorded; written review pending |
| Engineering review | Not run | Architecture and implementation checks | 0 for this design | Pending | No implementation plan or code reviewed |
| Outside review | Not run | Independent coverage | 0 for this design | Not requested | Main-agent source review only |

**OUTSIDE COVERAGE:** no delegated or external design review was performed. Native source/metadata inspection and HTML rendering were performed by the main agent. The gstack task aggregator was unavailable because its Bash environment could not locate Bun; the tasks above remain the authoritative review synthesis.

**VERDICT:** design specification prepared for user review; implementation is not cleared. Engineering review is required during planning.

**UNRESOLVED DECISIONS:**
- User review and approval of this written specification, including its concrete readability and fallback rules, is pending before implementation planning.
