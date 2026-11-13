# Bloom quota companion bubble

Date: 2026-10-27  
Status: APPROVED DESIGN BASELINE (2026-10-27); implementation is a separate next step  
Mode: Builder

## Promise

On an ordinary hover of Bloom's collapsed status notch, a small companion surface shows the most-used reported agent and its five-hour and weekly quota windows. Each ring stays paired with its own reset. One click opens Agent Quotas on that same agent.

The design signature is a precise pair of live rings attached to Bloom's notch rhythm. The time and music stay in the notch. The existing urgent alert keeps its current job and behavior.

## Approved direction and invariants

- Use approach C, the companion bubble, centered below the collapsed notch.
- Show two labeled rings for one agent: `5h` and `Week`.
- Pick the agent with the highest numeric `used` value among its reported five-hour and weekly pools. A reported `0` is a real value. Missing periods stay missing and show a muted em dash.
- Do not substitute a daily pool for `5h`, or unlimited usage for a numeric limit.
- Clicking the bubble opens Agent Quotas with that agent selected. The urgent alert's current click behavior stays unchanged.
- Keep the existing time and music presentation. Keep the urgent alert strip intact.
- Waiting, stale, failed, and unsupported-window states stay recognizable.

## Information hierarchy

The glance has one job: answer who is using the most quota, how the two relevant windows look, and when each one resets.

```text
Collapsed status notch
  time | music | existing status content | urgent strip when active
                         |
                         | immediate hover reveal; 800 ms leave grace
                         v
Companion bubble, centered below the notch
  [agent mark] Agent name                         [Stale when needed]
  [5h ring + percent + reset] | [Week ring + percent + reset]
                         |
                         | click
                         v
Agent Quotas panel, same agent selected
```

Use one capsule with a compact identity row and two balanced ring cells. Give the rings equal visual weight. Keep each period label and reset directly associated with its ring. The ring arc encodes percent used; the number shows the reported percent. Keep the existing green, amber, and red usage thresholds. Keep an em dash inside an unavailable ring, never a zero.

The surface should feel like a small quota instrument. Use alignment, a clean split between the two windows, and Bloom's own material to give it character. Do not add nested cards, ornamental icons, a decorative glow, or repeated agent identity tiles.

## Selection and quota semantics

The helper's current `QuotaWindow` carries a label and numeric usage. Its detail rows carry reset times separately, so the bubble cannot currently prove that a reset belongs to the exact pool that won selection. The current draft's promise to pair each ring with its reset is therefore not implementable without a clearer association.

Add a small, additive local-helper data contract. Each quota window, including a group window, carries:

- a canonical `period` of `5h` or `week` when the reader knows that period;
- numeric `used` for a finite pool when the reader reports it; omit or mark unlimited pools as ineligible;
- optional `resetAt` or `resetText` for that exact window.

Keep the display `label` and the existing detail rows. Populate period and reset fields from structured source data in the helper. Do not parse a formatted value string or guess a weekly reset from an agent-level five-hour reset. Prefer `resetAt` for the local countdown. If a source only provides `resetText`, show that text with a `resets` prefix and do not parse it. A provider without either reset still contributes its usage; its reset caption is omitted. These are optional fields in Bloom's local helper protocol. No provider endpoint, credential, or polling change is needed.

Selection is deterministic:

1. Only finite, limited windows with a canonical `5h` or `week` period compete. A missing numeric value is absent; numeric zero is valid. Never encode an unlimited pool as zero percent.
2. For each agent and each period, use the highest `used` pool across its top-level and grouped windows. If two pools tie, keep the first source-order pool.
3. Score the agent by the higher of its available five-hour and weekly percentages. The highest score wins. Ties between agents keep the existing helper reading order.
4. `ok` readings compete. `stale` readings keep competing with their last-good values and resets. A `waiting` or `failed` reading without a last-good value contributes no candidate.
5. A pending reader does not erase a known candidate. While the bubble is visible, keep its agent identity fixed; update that agent's rings from new snapshots, but do not switch the button under the pointer. Recalculate on the next hover after the bubble closes.

If the hover begins with no candidate, show `Checking` without locking an identity. Lock the first eligible candidate that arrives during that hover. If a newer successful reading for the locked agent has no eligible windows, keep the agent name, replace both values with dashes, and show `No 5h or weekly quota`. Do not keep old values styled as fresh and do not switch to another agent until the next hover. If the locked reading becomes unavailable, retain its name and show `Unavailable` until the bubble closes.

For a grouped pool, the selected window also owns the reset shown in that cell. Preserve exact source order for ties so a later snapshot cannot alternate between identical buckets.

## Visual treatment

- Use a single, horizontally balanced capsule. Target approximately 308 by 92 CSS pixels, with the complete surface and shadow measured after Bloom's scale is applied.
- In the identity row, show the authentic agent mark and the familiar agent name. Show a concise `Stale` treatment when using a last-good reading.
- In each ring cell, show the 30 to 34 pixel ring, its `5h` or `Week` label, the percentage, and the reset caption. Use tabular numerals so values do not shift the layout.
- Reuse Bloom's `--bloom-bg-expanded`, `--bloom-text`, and `--bloom-text-muted` theme variables, the ring treatment inside `QuotaPanel`, `AgentMark`, and the existing quota color thresholds. If the ring is private to `QuotaPanel`, extract a shared presentation component rather than duplicate its meaning. Do not force the standalone Agent Quota app's pure-black surface into Bloom's light or user-selected themes.
- Keep the UI typeface inherited from Bloom. Use readable contrast for every label. Keep the agent name and percentage at least 14 px, period and reset text at least 12 px, and reset text at a minimum 4.5:1 contrast against the actual surface.
- Use one subtle internal divider only if needed to separate the two window cells.
- For available widths below 284 CSS pixels, switch the two cells to compact stacked rows, preserving both rings, labels, values, and resets. Truncate only the visible agent name; keep its full name in the accessible label.
- Give the button an accessible name that includes the full agent name, each period's reported percentage or `not reported`, each available reset, and stale age when relevant. Expose that name to accessibility tools while the bubble is present. Bloom deliberately keeps the native overlay focus-free, and the collapsed notch has no keyboard tab stop. Keep this bubble pointer-first; do not promise keyboard focus entry or add a shortcut for this feature. Touch keeps the existing tap-to-open route. Preserve the Quotas panel's current keyboard behavior without claiming a new keyboard entry path.

## Appearance, placement, and dismissal

- Show only while the main notch is visible, collapsed, and in status mode. Do not show over an open panel, announcement, hidden notch, or a different activity view.
- Reveal immediately on the existing notch hover start; add no new dwell delay. Reuse the current 800 ms interaction grace for dismissal. The notch, bubble, and the small corridor between them act as one pointer region. Both DOM and native pointer-leave handlers must honor that union before releasing notch hover.
- Anchor to the measured visible notch rectangle, including any urgent-alert expansion. First avoid the notch and urgent strip, then the shelf and music bubbles, then status tooltips. Search downward from the centered position in small increments; clamp each candidate to the host monitor and test it again for collisions after clamping. Freeze the first clear position for the rest of that hover session; recalculate only after a new hover or a monitor/window-bounds change. If no collision-free position fits, suppress the bubble for that hover and leave the existing Quotas entry route available.
- Clamp the full painted and interactive bounds to the monitor hosting the notch, including the shadow. Preserve Bloom's existing top-edge relationship with MyDockFinder and the Windows taskbar. Do not hard-code the primary monitor when the notch is on another monitor.
- Include the bubble ref and its painted bounds in Bloom's existing `reportNotchRect`, native window region, and resize measurements. Pointer movement from notch to bubble must not pass through to the dock.
- Keep the urgent strip in its current position and preserve its contents and click behavior. It may mention a different urgent agent than the bubble. Clicking the bubble selects its displayed agent; clicking the alert retains the alert's existing selection behavior. Opening Agent Quotas closes the bubble.
- Keep passive hover focus-free. The bubble is one semantic button for pointer activation with the accessible name above. Do not autofocus or move focus on hover, and do not add a keyboard shortcut or entry path. Leave the existing Quotas panel keyboard behavior unchanged. Touch users keep the existing tap-to-open Quotas route.

## Motion

- Reuse Bloom's reveal timing and spring character, with a restrained opacity fade and only a few pixels of vertical travel. Start near full size to avoid popping a large surface from a tiny scale.
- Respect `prefers-reduced-motion` by removing travel and ring interpolation.
- Animate a changed ring arc and its number together once per snapshot. Do not pulse, loop, or count every second.
- Refresh reset countdown captions once per minute. Use the current local countdown formatting. A past reset reads `now`; absent resets have no caption.

## State coverage

| State | What the user sees | Click behavior |
|---|---|---|
| Candidate available | Agent name and mark; both rings; a muted em dash for a missing period; each exact reset when supplied | Open Agent Quotas on that agent |
| One or more readers still pending, candidate available | The known highest reported candidate; no blanking or identity jump during this hover | Open the displayed agent |
| No candidate, readers pending | Same capsule footprint with two dashes and `Checking` | Open Agent Quotas using its existing default selection |
| Selected reading stale | Last-good ring values and resets, dimmed arc, visible `Stale` plus last-good age in the accessible label | Open the same agent |
| No candidate, all candidate-capable reads failed and none remain pending | Two dashes and `Unavailable`; the panel remains the place to see each failure reason | Open Agent Quotas on its existing default failed-agent selection |
| No candidate, healthy readings report only unsupported or unlimited pools | Two dashes and `No 5h or weekly quota` | Open Agent Quotas using its existing default selection |
| One period absent | Keep its label and show an em dash; retain the other ring and reset | Open the same agent |
| Fresh snapshot arrives while visible | Update the displayed agent's two values in place; keep the same agent identity until dismissal | Open the agent that remained visible |
| First candidate arrives during `Checking` | Fill the rings and lock that first eligible agent for the rest of this hover | Open the newly displayed agent |
| Locked agent's successful reading loses both eligible periods | Keep its name, show two dashes and `No 5h or weekly quota`; do not reuse old percentages or jump to another agent | Open the locked agent |
| Locked agent becomes unavailable | Keep its name and show `Unavailable` until dismissal | Open the locked agent |

If an agent has both a stale candidate and a newer failed attempt, keep the last-good values and reset and expose the existing stale/error details in the full panel. A failure with no last-good data never becomes a zero-percent ring.

## User journey and emotional arc

| Step | User does | User should feel | Design response |
|---|---|---|---|
| 1 | Briefly hovers the collapsed notch while working | Immediate orientation, with no interruption | Time and music remain in place; the bubble reveals at the existing hover start |
| 2 | Reads the agent name and paired rings | Confidence about the most-used reported agent | Period labels and resets stay attached to the exact values |
| 3 | Sees a missing or stale window | Informed rather than reassured by a fake number | Em dash or stale treatment keeps uncertainty visible |
| 4 | Moves from the notch to the capsule | The surface feels connected and easy to reach | Shared hover region and measured pointer corridor prevent flicker |
| 5 | Clicks the capsule | The control does exactly what its preview promised | Agent Quotas opens on the displayed agent |
| 6 | Returns to a later hover | The glance reflects the latest snapshot | Choose again at the next hover; preserve identity during the current one |

At five seconds, the user sees who is most used and both reset horizons. At five minutes, the click path and live updates remain predictable. Over long use, no missing value or reset is fabricated, which protects trust in the quota readings.

## Existing patterns and implementation boundaries

Bloom already provides the side activity bubble pattern in `bloom/src/App.tsx` and `bloom/src/App.css`, including theme variables, animation, `reportNotchRect`, and native hit-region measurement. The current geometry list tracks the shelf and music bubble refs, so the quota bubble must join that same path. The notch itself is not a keyboard tab stop and the native window stays `NOACTIVATE`; keep this feature pointer-first rather than describing a keyboard path the app does not provide.

The quota UI already provides `QuotaWindow`, `QuotaGroup`, `QuotaDetail`, `AgentMark`, the quota ring, ring thresholds, `quotaView.ts`, the Quotas panel, and urgent-alert selection. The full quota panel remains the detailed comparison surface.

An earlier concept review flagged a percentage and reset being paired across different periods as a P1 trust failure. This design carries that lesson into the bubble with an exact per-window reset association: [prior critique](../.impeccable/critique/2026-10-16T02-09-03Z__design-notch-overview.md).

The root `DESIGN.md` documents the standalone Agent Quota Seam renderer. Its usage colors and identity rules apply to the quota data. Its pure-black material and native geometry do not replace Bloom's own theme or overlay behavior.

The quota protocol currently transmits resets in detail rows and an agent-level five-hour reset. It does not attach a reset or canonical period to each structured window. The bubble requires the additive per-window contract above so each reset can be proven to belong to the ring it labels.

## NOT in scope

- Replacing, consolidating, or changing the urgent alert strip.
- Adding Bloom's dock or changing the installed dock, MyDockFinder, or Windows taskbar behavior.
- Adding quota providers, provider API calls, credentials, polling intervals, or background work.
- Replacing the full Agent Quotas panel or changing its current selection, refresh, or reset-display settings.
- Substituting daily, monthly, count-based, or unlimited readings for the requested five-hour and weekly windows.
- Adding a setting to disable the default hover glance in this first version.
- Showing the bubble in open panels, announcement mode, or another collapsed activity mode.

## Success criteria

- Ordinary hover in collapsed status mode reveals one companion capsule while time and music remain readable.
- The representative is the highest numeric `used` value over recognized five-hour and weekly windows, including grouped windows, with deterministic pool and agent tie-breaks.
- A missing pool stays labeled and visibly missing. A reset is shown only when attached to the exact selected pool.
- Waiting, stale, failed, partial, and unsupported-window states match the state table.
- The representative identity never changes while the bubble is visible. The next hover uses the newest snapshot.
- Clicking opens Agent Quotas with the visible agent selected. The urgent alert still behaves as it does now.
- The bubble stays within the notch's monitor, avoids visible activity bubbles and tooltips, and remains in the same native hit region as the notch.
- Verify light and dark themes, 100/125/150/200% DPI, narrow-monitor layout, reduced motion, the accessible name, existing Quotas keyboard behavior, shelf/music coexistence, and urgent-alert coexistence.

## Implementation tasks

Synthesized from this review. These are design tasks only; no application code changed in this review.

- [ ] **T1 (P1, human: ~3h / CC+gstack: ~25min)** - Quota protocol - Add canonical period and exact reset fields to each top-level and grouped window; omit unlimited pools from numeric candidates.
  - Surfaced by: Selection semantics - the current protocol cannot guarantee that a ring's reset belongs to its winning pool.
  - Files: `quota/quota.go`, provider readers under `quota/`, `quota/service_protocol.go`, `quota/service.go`, `bloom/src/quota/protocol.ts`.
  - Verify: fixtures prove finite 0% differs from unlimited, top-level and grouped resets remain paired, and old snapshots without optional fields still parse.
- [ ] **T2 (P1, human: ~3h / CC+gstack: ~20min)** - Quota selection - Add a pure representative and ring view model with deterministic ranking, stale handling, empty states, and hover-session identity locking.
  - Surfaced by: Pass 1 and Pass 2 - the draft lacked a precise ranking and state contract.
  - Files: `bloom/src/quota/quotaView.ts`, related quota view tests, `bloom/src/App.tsx`.
  - Verify: numeric zero, absent periods, grouped buckets, ties, unsupported periods, stale/failed readings, pending snapshots, and identity stability.
- [ ] **T3 (P1, human: ~4h / CC+gstack: ~30min)** - Companion capsule - Render the paired rings, matched reset captions, states, theme, reduced-motion behavior, and accessible label.
  - Surfaced by: Pass 1, Pass 3, Pass 4, Pass 5, and Pass 6 - the visual hierarchy and behavior needed concrete rules.
  - Files: `bloom/src/App.tsx`, `bloom/src/App.css`, `bloom/src/quota/QuotaPeek.tsx`, `bloom/src/quota/QuotaPeek.css`, `bloom/src/quota/quotaView.ts`.
  - Verify: visual states in light/dark themes; labels, stale, dashes, contrast, accessible name, and reduced motion.
- [ ] **T4 (P1, human: ~3h / CC+gstack: ~25min)** - Placement and navigation - Measure the bubble with the notch, preserve a continuous hit region, avoid side activities and tooltips, clamp to the host monitor, and open the same selected agent.
  - Surfaced by: Pass 6 and existing geometry review - the bubble must stay clickable and visible under real pointer movement.
  - Files: `bloom/src/App.tsx`, `bloom/src-tauri/src/window_region.rs` or the existing geometry owner, targeted end-to-end specs.
  - Verify: native pointer sweep and click at top-edge coordinates, music/shelf/urgent overlap, narrow monitor, and 100/125/150/200% DPI.

## NOT in scope for this review

- Application implementation, packaging, installation, or live desktop changes. This pass updates the design document only.
- Generating mockups. The gstack design binary is unavailable in this environment; no hand-built mockup was substituted.
- Rewriting the root `DESIGN.md`, which documents the standalone native Agent Quota widget rather than Bloom's overlay design system.

## Design review scorecard

| Pass | Initial | After review | Finding and resolution |
|---|---:|---:|---|
| 1. Information architecture | 6/10 | 9/10 | Added a hierarchy diagram, ring anatomy, and click path. |
| 2. Interaction state coverage | 5/10 | 9/10 | Added explicit loading, partial, stale, failed, unsupported, update, and click behavior. |
| 3. User journey and emotional arc | 6/10 | 9/10 | Added a six-step journey and five-second, five-minute, long-use goals. |
| 4. AI slop risk | 8/10 | 9/10 | One necessary interactive capsule; specific data hierarchy and motion; no generic dashboard patterns. No mockup pixels were available to inspect. |
| 5. Design system alignment | 6/10 | 9/10 | Bound the surface to Bloom theme variables and reused quota marks, ring, thresholds, and button styling. |
| 6. Responsive and accessibility | 5/10 | 8/10 | Added narrow layout, monitor/DPI bounds, an explicit pointer-first boundary, accessible naming, contrast, and reduced-motion behavior. |
| 7. Unresolved decisions | Not scored | 0 open | Design choices are resolved in this draft. Final user approval remains the Office Hours gate before implementation. |

Overall design score uses the lowest of passes 1 through 6: **5/10 initial to 8/10 after review**.

Pass 4 evidence: classifier `OPERATE`. Hard-rejection patterns: none in the written direction. The review is text-only because the gstack designer is unavailable.

| Litmus | Result | Evidence |
|---|---|---|
| Bloom and quota are recognizable at first glance | Yes | The notch remains the anchor; an authentic agent mark, name, and two explicit quota periods identify the payload. |
| One strong visual anchor | Yes | The two live quota rings carry the glance. |
| Understandable by scanning | Yes | The name, `5h`, `Week`, percentages, and reset captions are specified in that order. |
| Each section has one job | Yes | The capsule reports one representative; the full panel remains the comparison surface. |
| The capsule surface earns its place | Yes | The whole surface is one clickable target that opens the same agent. |
| Motion improves hierarchy | Yes | One reveal and one value update are specified; no looping motion. |
| Premium without decorative shadows | Yes | Theme material, alignment, and exact data pairing carry the treatment. |

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|---|---|---|---:|---|---|
| CEO Review | not run | Product scope was already selected | 0 | not run | No new product-direction gap found |
| Outside Review | OpenAI `gpt-6-astra` (`astra-advisor`) | Independent critical review of this draft | 1 | completed | 4 findings resolved: keyboard reachability, hover-session data transitions, actual leave timing, and collision fallback |
| Eng Review | not run | This was a design-document review; no code changed | 0 | not run | Validate protocol and native interaction during implementation |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | clean | Score: 5/10 to 8/10; 4 design decisions added |
| DX Review | not run | Builder experience is unchanged | 0 | not run | No developer workflow scope |

**OUTSIDE COVERAGE:** OpenAI `gpt-6-astra` (`astra-advisor`) completed a read-only critical review of this draft; all four findings were incorporated. The earlier bounded quota-bubble audit was also used as prior context. No cross-provider review was run.

**VERDICT:** design direction is ready for user approval. The local helper's structured-window contract must be extended before a reset can be guaranteed to match each ring. Implementation review and visual QA remain future work.

NO UNRESOLVED DECISIONS
