# Taskbar Focus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. The main agent implements and verifies, per project instructions. Steps use checkbox syntax for tracking.

**Goal:** Dock the existing dual-ring quota readout beside Windows taskbar indicators and open the selected Focus layout from it.

**Architecture:** Keep quota acquisition and the pure CPU renderer. Introduce explicit docking state and a native taskbar adapter, gated by a feasibility check before product changes. Keep floating Seam behavior intact and recover to it when attachment is unavailable.

**Tech Stack:** Windows-only Go 1.27, existing x/sys Win32 bindings, CPU raster painter, bundled Inter and authentic agent marks; no new runtime dependencies.

**Spec:** [Approved Taskbar Focus design](../specs/2026-10-17-taskbar-focus-design.md).

**Execution outcome, 2026-10-17:** Task 1 returned FAIL under the approved constraints. See [feasibility evidence](../../verification/taskbar-focus/feasibility.md). Tasks 2–5 were not started, as required by the gate. The feature is not implemented; the next action is review of a revised integration option.

## Global Constraints

- Preserve the authentic selected-agent mark, inner five-hour ring, outer weekly ring, primary percentage, and reset countdown.
- Use two lines for percentage and reset time.
- Keep the window focus-free; do not introduce Escape or click-outside dismissal.
- Do not inject a DLL into Explorer or silently resize its private child windows to force room.
- No browser or Node process belongs in the installed runtime.
- Local/API readers every 30 seconds, Kiro and Antigravity CLI readers every five minutes, and manual refresh immediately; one in-flight read per agent.
- Preserve unrelated uncommitted work. Commit only reviewed task changes; never stage the entire dirty checkout.

## Review Focus

- A busy taskbar has insufficient space: keep task buttons, tray, clock, and Show Desktop usable; reject docking rather than cover them (Tasks 1, 3, 5).
- Explorer disappears or restarts during a drag: discard stale geometry and restore a usable floating window (Tasks 1, 3, 5).
- DPI changes mid-drag: keep the pointer anchor stable and the entire Focus panel visible (Tasks 2, 3, 5).
- Saved docking state starts on an unsupported taskbar: preserve the last floating position and recover without a launch loop (Tasks 3, 5).
- An agent has only one pool, no numeric allowance, long error text, or stale data: preserve truthful readable detail and selection (Tasks 2, 4, 5).

## File responsibilities

- `docs/verification/taskbar-focus/feasibility.md`: native placement evidence and pass/fail decision.
- `docs/scratch/taskbar-focus/`: isolated disposable diagnostic harness, if read-only geometry inspection is insufficient; do not ship it.
- `widget/focus.go`, `widget/focus_test.go`: Focus geometry, painting, and hit targets.
- `widget/widget.go`, `widget/paint.go`: dispatch Focus through existing geometry and rendering entry points.
- `internal/win/taskbar_windows.go`, `internal/win/taskbar_test.go`: native taskbar discovery and event-driven lifecycle.
- `taskbar.go`, `taskbar_test.go`: app docking transitions and geometry policy.
- `main.go`: settings serialization, input routing, and existing redraw integration only.
- `taskbar_settings_test.go`: settings compatibility and fallback.
- `DESIGN.md`, `docs/verification/taskbar-focus/verification.md`: shipped contract and revision-specific evidence.

## Task 1: Prove a safe taskbar placement mechanism

**Files:** Create the feasibility evidence and, only if necessary, an isolated diagnostic harness in the paths above. No installed application changes.

**Interfaces:** Consumes the approved spec and current Windows taskbar configuration. Produces a documented placement decision, measured geometry, supported arrangements, lifecycle evidence, and either PASS or FAIL. No product adapter API is presumed to work before this check.

- [x] Read the existing drag/window code and verify the current Windows build, taskbar rectangle, primary monitor work area, tray extent, and task-button extent. Record dimensions without collecting window titles or other unrelated personal content.
- [x] Verify candidate mechanisms against current Microsoft documentation. Assess notification icons, desktop appbars, and external taskbar placement against the requirement for space *inside the taskbar beside its indicators*. An adjacent desktop bar alone is not a passing substitute.
- [ ] Test a disposable readout at the required two-line size only if a candidate can identify genuinely unoccupied space without altering Explorer's private layout. Verify relayout, crowded taskbar, auto-hide, and Explorer-loss behavior. Do not restart Explorer automatically; reserve disruptive live checks for an appropriate user-controlled moment and leave them explicitly unverified until performed.
- [x] Write `feasibility.md` with the mechanism, documented APIs, screenshots/geometry, limitations, and outcome. PASS requires a defensible way to prevent overlap as taskbar contents change. A single empty-space screenshot is insufficient.
- [x] **Gate:** On FAIL, stop Tasks 2–5 and present the limitation plus a concrete revised option for review. Do not weaken the approved behavior or quietly ship an overlay. On PASS, add the proven adapter details to this plan before product implementation; a mechanism that changes the approved interaction requires user review.

## Task 2: Implement the pure Focus renderer

**Files:** Create `widget/focus.go`, `widget/focus_test.go`; modify `widget/widget.go`, `widget/paint.go`.

**Interfaces:** Add `Focus bool` to `widget.View`. Preserve `Render(View) *image.RGBA`, `Size(View) (int, int)`, `PillBounds(View) image.Rectangle`, `PanelBounds(View) image.Rectangle`, `Hit(View, image.Point) Target`, and existing control types. Focus dispatch takes precedence over Seam only when explicitly selected. Create private `focusLayout(View) focusGeometry` as the shared source for painting and hit targets.

- [ ] Add behavioral tests: `TestFocusSelectedAgent` selects each of the seven IDs and asserts the rail's `AgentControl` targets identify the correct ID; `TestFocusPoolStates` covers five-hour+week, one pool, zero, unlimited, stale, and failed states using real `Item` shapes.
- [ ] Add `TestFocusGeometry` at scales 1, 1.25, 1.5, and 2: rail width starts at 58 logical pixels, panel width starts at 468, all controls stay inside the image, and close/refresh/reset targets do not overlap agent targets. Add a long-detail fixture to prove content cannot paint over the rail or footer.
- [ ] Run `go test ./widget -run Focus -count=1`; confirm failures describe missing Focus behavior before implementation.
- [ ] Implement the vertical rail and selected detail workspace using existing authentic marks, text painters, pool selection, and threshold colours. Reuse provider detail rendering where suitable; preserve primary/reset consistency with the compact readout. Add explicit close and refresh controls; keep countdown/clock reset hit behavior.
- [ ] Run `go test ./widget -count=1`. Render fixtures through `widget.Render` for visual review; verify the readout preserves both rings and two text lines. Commit only this renderer change and its tests.

## Task 3: Implement docking state and native lifecycle

**Files:** Create `taskbar.go`, `taskbar_test.go`, `taskbar_settings_test.go`, `internal/win/taskbar_windows.go`, `internal/win/taskbar_test.go`; modify `main.go`, `internal/win/win.go`.

**Interfaces:** The proven mechanism from Task 1 supplies native placement. Define `win.TaskbarSnapshot` with `Bounds`, `WorkArea`, and `Slot` as `image.Rectangle`, plus `Available` and `Hidden` as bool. Expose `win.PrimaryTaskbar(readout image.Point) (TaskbarSnapshot, error)`; `Slot` must be empty unless placement is safe. Route taskbar lifecycle notifications through an optional `OnTaskbarChanged()` callback, following the existing optional callback pattern. An error or empty slot is never permission to overlap.

At app level add `Docked bool` to settings and transient `dockPreview bool` to `app`. Existing `X`, `Y`, and `HasPos` continue to store the floating anchor. Add `dockPlacement(snapshot win.TaskbarSnapshot, readout image.Point) (image.Rectangle, bool)` and `focusPanelPosition(readout, work image.Rectangle, panel image.Point) image.Point` as pure geometry helpers. Keep actual shell attachment confined to the adapter proven in Task 1.

- [ ] Add table tests for `dockPlacement`: exact fit passes; insufficient width/height, hidden taskbar, unavailable snapshot, or an empty slot fails. Add panel-position cases at both horizontal edges and each supported DPI, asserting full work-area containment.
- [ ] Add transition tests: floating → preview → docked on release; leaving the target before release cancels; docked → floating retains the pointer anchor; losing the taskbar invalidates the preview and falls back to the saved floating position. Corrupt/old settings default to floating; unavailable saved docking does not overwrite the saved floating coordinates.
- [ ] Run `go test . ./internal/win -run 'Taskbar|Dock|FocusPanel' -count=1` and confirm the new behavior fails before implementing it.
- [ ] Implement discovery and lifecycle using the Task 1 mechanism. Handle shell recreation, display/work-area/DPI changes, and visibility changes through native events. Do not introduce a recurring polling process or another quota timer. If event-driven safety cannot be maintained, fail closed to floating mode.
- [ ] Integrate drag targeting before work-area clamping, show the dock preview, commit state on release, and serialize `Docked` compatibly. Update `loadSettings` and the actual save payload together. Keep `LayoutVersion` migration independent of docking.
- [ ] Run the targeted tests and the existing drag/settings suite. Commit this task's reviewed changes without including unrelated edits.

## Task 4: Connect Focus to the live application

**Files:** Modify `main.go`, `taskbar.go`; extend `taskbar_test.go` and `widget/focus_test.go` where behavior crosses the boundary.

**Interfaces:** `app.buildView() widget.View` sets `Focus` only for active docked placement. `app.OnPointerClick`, `OnMove`, `OnDragEnd`, `OnDPIChanged`, and `OnTaskbarChanged` continue to own interaction. Renderer uses the same selected ID, formatted reset labels, and quota snapshot as the readout.

- [ ] Add tests proving selecting a rail item updates both selected detail and compact readout, clicking the readout toggles Focus, reset mode changes both surfaces, and detaching returns to Seam. Stale/failure transitions must retain agent selection and show the existing recovery information.
- [ ] Implement routing and native sizing. A panel may extend above the taskbar but must not cause the readout to jump. Keep taskbar placement separate from popup work-area clamping. Preserve focus, pin/close behavior, second-launch reveal, and manual refresh.
- [ ] Run `go test . ./widget -count=1`; inspect all seven selections once in rendered fixtures. Fix defects in one batch and confirm once. Commit the integration changes.

## Task 5: Verify and install the actual native result

**Files:** Update `DESIGN.md`; create `docs/verification/taskbar-focus/verification.md` and supporting captures.

- [ ] Run `gofmt -l .` (expect no output), `go vet ./...`, and `go test ./... -count=1`. Record commands and outcomes against the exact implementation revision.
- [ ] Build the executable and capture Focus closed/open, both quota rings, all seven selections, zero/missing pools, stale/failure states, and 100/125/150/200% render scales. Review one batched set, fix defects together, then confirm once.
- [ ] Exercise the real binary: drag-to-dock, cancel preview, undock, relaunch with saved docking, unsupported configuration fallback, focus retention while typing in another app, clock/tray/task-button usability, taskbar relayout, auto-hide, DPI change, and Explorer-loss/recreation recovery. Mark any unavailable live scenario unverified; do not claim completion without the required native evidence.
- [ ] Compare idle CPU/RAM with the existing app over the same sampling interval, separating app use from scheduled CLI subprocesses. Confirm local/API scheduling remains 30 seconds, CLI scheduling remains five minutes, and docking creates no extra reader invocations.
- [ ] Preserve the current installed executable as a rollback copy, replace it with the verified build, and verify its hash and running path. Preserve settings and stop/relaunch only Agent Quota. Confirm the installed Focus behavior visually.
- [ ] Update `DESIGN.md` with shipped behavior, supported taskbars, failure recovery, and genuine limitations. Commit only this task's documentation and any final reviewed fixes. Report the installed result and evidence, or the feasibility failure if Task 1 did not pass.

## Plan review

The user's project instructions already select main-agent implementation and verification. Use native execution here after plan approval; do not create implementation agents. The first deliverable is the feasibility verdict. Later tasks are conditional on its success, so this plan does not promise an unproven taskbar integration.
