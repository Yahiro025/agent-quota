# Bloom media queue: consolidated EGM review

Date: 2026-10-29. Target: [interface brief](../../superpowers/specs/2026-10-29-bloom-media-queue-design.md), lines 1–34.

## Recommendation

**Keep the attached drawer. Tighten its behavior contract, then prove the connectors before building the complete feature.** The largest risk is controlling the wrong playback session or changing its queue unexpectedly, not the drawer's appearance.

The selected interface is proportionate: one button, a flat scrolling list, and existing player controls. No independent reviewer found a reason for another window, service, source picker, artwork cache, or redesign. Nine fresh-context reviewer runs completed across five stages. No implementation plan was supplied: feature range is 1–34 and detailed range is N/A. The readiness stage is therefore a gap assessment, not an implementation-plan pass.

## Prioritized actions and traceability

| ID | Priority | Required outcome | Raw findings |
|---|---|---|---|
| Q1 | Blocker for live-feature acceptance | Prove real ordered entries, selection of a later occurrence, observed playback, and resulting queue order in Spotify Free and supported Brave YouTube contexts. Include duplicates and unsupported pages. This does not block starting the proof. | readiness-1-1; codebase-1-1 |
| Q2 | Major | Bind displayed metadata, queue, selection, and existing controls to one identified session/tab. Reject uncertain associations and invalidate former-session results. | design-1-1; readiness-1-2; codebase-1-2 |
| Q3 | Major | Give a before/after example for selecting C from upcoming A/B/C/D. Define earlier-entry treatment and preserve supported source-native continuation; do not silently replace the playlist. | design-1-2; structural-1-3; Q1 proof |
| Q4 | Major | One outstanding selection per session, finite pending deadline, explicit confirmation, stale-result rejection, and playback recheck before retrying uncertain outcomes. | design-1-3; readiness-1-2/3; codebase-1-3 |
| Q5 | Major | Name compact/classic coverage; define layout-switch and volume/drawer coexistence. Account for native height/region growth, work-area limits, centered controls, pointer continuity, and wheel isolation. | comprehension-1-2; structural-1-2; codebase-1-4 |
| Q6 | Major | Choose the smallest connector communication route, access boundary, lifecycle, and recovery. Add a compact file/change map and executable checks, without a general integration framework. | readiness-1-3/4; codebase-1-1/3/4 |
| Q7 | Documentation | Add durable screenshot/component references and briefly define the two connector roles. | comprehension-1-1/3 |
| Q8 | Nit | Replace “Operate surface” and “honest states” with plain, concrete wording. | style-1-1/2 |
| Q9 | Nit / contested severity | A sentence explaining the attached drawer's rationale is enough; a formal alternatives study or mandatory staged delivery is unnecessary. | design-1-4; structural-1-1; Stage 8 contest |

## Confirmed code facts

The existing player is a useful extension point, but it has no queue contract or source/session identity. The Windows worker selects display metadata from enumerated sessions, whereas transport controls call `GetCurrentSession()` independently. This is a concrete routing mismatch relevant to a new queue feature; the review did not demonstrate wrong-player playback live. Existing fire-and-forget commands and optimistic play/pause state cannot establish confirmed queue selection.

Bloom already has two player layouts, fixed media heights, a compact volume expansion, wheel navigation, and native region reporting. Adding rows only inside the compact component would not finish the feature's sizing and interaction work. Source anchors and qualifications are in the readiness report.

## Frugal next step

First resolve Q3 and Q5 in a short brief amendment. Then run bounded source-specific connector proofs for Q1/Q2 before committing to a bridge design. Use the results to write the minimal backend contract and drawer implementation checklist. Keep unsupported sources/modes explicitly unavailable; do not expand into generic media support or weaken the immediate-play promise. Build the full drawer against the proven contract, then validate source races, failures, layout sizing, and the existing controls.

## Review execution and limits

| Stage | Reviewer roles | Status | Artifact |
|---|---|---|---|
| 5 | Zoom out | Completed; no findings | [Initial review](zoom_out_report.md) |
| 6 | Comprehension | Completed; paraphrase retained | [Comprehension](comprehension_report.md) |
| 7 | Design, structural, style | All completed; one audit round | [Critique](critic_report.md) |
| 8 | Zoom out with prior findings | Completed; no new findings; alternatives severity contested | [Intermediate review](intermediate_zoom_out_report.md) |
| 9 | Readiness, codebase details, style | All completed; no DetailedRange; plan-style assessment N/A | [Readiness](implementation_readiness_report.md) |

All reviewer runs used isolated conversation contexts and read their personas and assigned standards. They returned findings read-only; root persisted reports. No reviewers failed or were counted as zero-findings after failure. This is a single consolidated review, not a claim of multi-round convergence. Findings were not remediated because the selected mode requested a report without intermediate resolution gates.

The isolated reviewers could not inspect the unnamed screenshot or interactive concept. Their interface opinion is based on the brief and source, not rendered visual acceptance. No builds, tests, source-connector execution, Spotify account checks, runtime canary, or installed-binary changes occurred. This review does not invalidate historical quota-reader evidence or retest it.

Target SHA-256 at review start: `62714682BCB5EFD1F5512DB979FFBAA20C300A91D4D212FF70502973CA343B15`. The brief remains unchanged. Only review artifacts were added by this review run.
