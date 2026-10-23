# Final review — visual repair commit 819ddc5..f6a9bf0

Read-only review. Sources: implementation-brief.md, audit.md, implementation-report.md, the diff, and direct context in `bloom/src/App.tsx`, `bloom/src/quota/QuotaSpace.css`, `QuotaPanel.tsx`, `quotaView.ts`, `marks/index.tsx`, `App.css`, `src-tauri/src/commands.rs`. No suite/detector/native reruns per scope; no Playwright probes — source settles the named objections.

## Strengths

- Root cause is fixed correctly: `.qs { width: max-content; min-width: 386px }` makes the element itself report the ring row's need through `scrollWidth`, breaking the old measure/set feedback loop; deleting the reset bookkeeping is right because `width:max-content` re-sizes the element the observer already watches (App.tsx:1995-2008, QuotaSpace.css:17-27).
- The zoom/scrollWidth unit handling is correct: `scrollWidth` is already unzoomed CSS px while rects scale, so `/scale` on height and none on width are consistent (App.tsx:2000-2005). Verified by the report's RED at scale 1.25 and GREEN at 0.8/1/1.25.
- `grid-auto-columns: 1fr` gives equal cells matching the standalone's even split, while max-content still measures n×widest (QuotaSpace.css:76-84); `min-width:0` retained on cells and added to `.quota-viewport` prevents flex min-content blowout.
- Selected tile (`#1c1c20` + inset ring + drop shadow) is genuinely distinct from hover `#101010`, and the raised paint persists under hover (QuotaSpace.css:100-112).
- Footer clearance (`.qs` padding-bottom 8px), caption 10px, and the shrink/unmount path (`set_quota_panel_height` 0) are each covered by a regression test; the seven-agent fixture adds the overflow coverage the 5-agent suite lacked.
- React 19.3 honors callback-ref cleanup returns: `ro.disconnect()` runs on unmount and on `scale` dep changes; no observer leak, no stale-ref issue (App.tsx:1988-2011).
- New comments document real non-obvious platform invariants (zoom vs scroll extents, flex `min-width:0`), which comment-sicko keeps.
- `set_quota_panel_height` clamps to monitor height and re-pins at monitor top (commands.rs:514-529) — the height side has a real boundary.

## Issues

### Critical
None.

### Important

1. **Island width is unbounded — no cap against available display size.** `getRestingWidth` returns `Math.max(386, Math.ceil(quotaSpaceWidth))` (App.tsx:2962) and `quotaSpaceWidth = target.scrollWidth + 24` (App.tsx:2005). `.qs` is `width:max-content` with no `max-width` (QuotaSpace.css:20), and `max-content` intrinsic size ignores wrapping: every descendant contributes its *unwrapped* width. Repro: a snapshot whose `err`, `details[].label`, `help`, or `groups[].name` contains one long string (all flow into `.qs-line` / `.qs-detail-title` / `.qs-ctl` / `.qs-group-name`) — e.g. a reader error embedding a URL or a ~250-char line → `.qs` scrollWidth ≈ text width → island wider than the monitor; the native window is full monitor width (commands.rs:526), so the pill's outer cells clip offscreen. The brief requires width chosen from "actual content **and available display size**"; only the first half is implemented. Fix: cap the contract at the viewport, e.g. `.qs { max-width: calc(100vw - 24px) }` plus letting `.qs-rings`/cells shrink (`min-width:0` + existing ellipsis) rather than growing past it.

2. **`max-height:100vh` ignores the 36px `.main-row`, so the footer's bottom can render below the window edge.** `.quota-viewport` caps at `100vh` = full webview height (QuotaSpace.css:11), but it starts ~36px below `.bloom`'s top (`.main-row`, App.css:78, renders in quotas mode — the measurement comment itself says `.qs` "sits below the status row", App.tsx:1992). When `.qs` exceeds monitor height, Rust clamps the window to `m_size.height` (commands.rs:524) → the scrollport's bottom ~36px hang below the physical screen; content scrolled to the bottom still positions the footer in that offscreen band. The e2e check (`quota-space.spec.ts:204-214`) only proves `scroll > client` + DOM `isVisible` — the mocked harness has no native window clip, so it cannot see this. Repro: `longFixture` (60 detail rows ≈ 1200px) on a 1080p monitor. Fix: `max-height: calc(100vh - 36px)` or cap at the pill's remaining interior height.

### Minor

1. `quotaSpaceWidth` is never reset on unmount (App.tsx:1987); re-entering quotas applies the stale width until the observer's first tick. Self-corrects within a frame — harmless but worth a comment or `setQuotaSpaceWidth(0)` on ref-null.
2. `.qs` `min-width:386px` + the 24px padding means the island floor is effectively 410, making `Math.max(386, …)` (App.tsx:2962) dead arithmetic — cosmetic confusion, not a bug.
3. A few added comments narrate rather than explain a forced behavior (App.tsx:2012-2013 "no reset bookkeeping", App.tsx:3778-3780 "the island is the width contract", QuotaSpace.css:73-75). Borderline narration under comment-sicko; none rise to MUST KILL.
4. `.qs-group-name` (QuotaSpace.css:254-258) ellipsizes but has no `max-width`; fine once a cap exists, currently another unbounded contributor (folds into Important #1).

## Declined to judge

- `.qs-name` at 10.5px (audit F6): implementer declined, matching the standalone widget's name metrics; design parity, out of geometry scope — accepted.
- `.qs-mark` svg+img pairing (audit #7): confirmed intentional — `AgentMark`'s outer svg is the tile and `<image>`/nested svg is the glyph layer (marks/index.tsx:175-200). Not redundant.
- Audit's unverified items (click pass-through, DPI, transition shrink live, theme variants, lowercase Kiro basis labels): outside this commit's scope; overlays/screenshot contaminants handled by main.
- Impeccable detector rerun: explicitly excluded by scope (main ran it once → []).

## Assessment

**Ready to merge: with fixes.** The repair correctly eliminates the overflow/bleed/selected/footer/type defects with honest measured geometry and real regression coverage at three scales. Two Important gaps remain at the same boundary the brief names — the island's width never consults display size, and the height cap forgets the status row above the viewport — so oversized content can still push cells or the footer offscreen. Both fixes are small caps; land them before calling the repair complete.
