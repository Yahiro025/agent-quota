# Bloom visual repair requirements

Read this first. This file and audit.md are the task requirements. The approved Bloom and quota-space specifications resolve product intent. Do not restart design exploration.

## Goal

Repair the complete visual path through the combined notch. Preserve Bloom's original design and the complete standalone Agent Quota view inside the quota panel. Fix every actionable defect in the batched audit. Product code belongs to the delegated implementer, not the controller.

## Boundaries

- Own the required changes in bloom/src/quota/, quota integration and geometry in bloom/src/App.tsx, the corresponding styles and native sizing boundary, and regression tests. Change another file only when a traced cause requires it.
- Preserve unrelated dirty files, including the existing bloom/src-tauri/Cargo.toml modification. Never broad-stage or reset the checkout.
- Do not delete the old install, modify autostart, remove credentials, or publish. A duplicate standalone window may be stopped reversibly after its exact executable path is checked. Preserve rollback files and registry settings.
- Preserve all seven readers, actual values, error/recovery details, model groups, reset controls, refresh, pin, and close. Preserve focus-free passive interaction.
- Do not narrow the scope to a pretty screenshot. Bounds, scroll access, pointer hit regions, panel transitions, themes, and display scaling are part of the repair.

## Expected outcomes

- Every present agent remains legible, including Antigravity and Unavailable. Cells and marks have enough space without losing percentages or obscuring neighbors.
- The entire quota view, including header, detail, controls, divider and footer, remains on its painted notch surface and inside the native window.
- Choose width from the actual content and available display size. Do not force a wide child into a narrower fixed-width parent. Do not shrink text as an overflow workaround.
- Use a single coherent geometry contract. Account for parent padding, the status row, Bloom scale, Windows DPI, and the monitor height at their correct boundaries. Avoid competing measurements and resize feedback loops.
- Large content scrolls inside the available content area. Header/footer controls stay reachable. Short panels shrink back when leaving quotas, without stranded blank space, stale clipping, or an oversized input region.
- Empty, loading, failed, stale, busy, model-group, and long-content states have correct visual meaning and usable controls.
- Preserve Bloom's established material and theme behavior. Intentional quota usage colors and authentic agent marks are requirements, not decorative anti-patterns.

## Method and evidence

Read the Impeccable craft floor immediately before editing. The controller already ran Impeccable context once for this pass. Do not run it again. Read the TDD skill and writing-good-tests reference. Write behavior regressions and capture expected failures before product changes. Keep test-only fixtures outside production.

Read audit.md, then reproduce its root causes using existing native CDP and browser test harnesses. Native CDP is a documented workflow under .scratch/diag/. All before captures belong to one batched inspection. Make one coordinated repair batch, then one batched confirmation. Do not loop speculative visual polish.

Run the relevant full frontend test suite, browser regressions, build, and native checks when the native boundary changes. Keep full logs in this directory and report failures explicitly. Run the requested Impeccable detector once over changed frontend targets after the implementation. Do not repeat it for speculative polish.

Rebuild and relaunch the verified debug desktop app from this checkout with the existing quota helper beside it. Preserve settings and dock behavior. Do not leave a dev preview in place of the native result. Save desktop captures of resting, quota, long detail or groups, and a neighboring panel after switching. Capture only the app region when practical and label native versus mocked evidence.

Write implementation-report.md with defect-to-fix mapping, exact red and green commands/logs, changed files, native process path and build identity, confirmation screenshots, remaining limits, and deviations. Commit only owned changes on the existing feature branch. Return a short status, commit SHA, verification counts and report path. Never spawn subagents.
