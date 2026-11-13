# Stage 9: Implementation-readiness assessment

Target: interface brief, lines 1–34. **DetailedRange: N/A.** No implementation plan exists. The user requested a consolidated review and a readiness assessment; reviewers assessed the available feature brief rather than inventing an implementation section. No actual DetailedRange audit or implementation validation passed.

## Implementation reviewer: queue_readiness

Verdict: not ready for real queue implementation; ready for a bounded connector feasibility spike.

- **readiness-1-1 — Blocker — lines 30–34:** Neither required connector has evidence of reading actual upcoming entries and selecting an occurrence while preserving the remaining queue. Record one live proof per source: upcoming order, selection of a later occurrence, playback confirmation, and remaining order. Include a repeated track and an unsupported browser page. Failed capability requires an explicit scope/behavior decision, not recommendation lists or destructive playback replacement. This blocks claiming the live feature works, not starting a bounded proof.
- **readiness-1-2 — Major — lines 14, 18, 24:** `MediaInfo` has no player/session identity. The service selects the first playing session, otherwise the first session with metadata. Specify source/session identity, distinct entry identity, queue revision, selection tied to revision, stale-selection rejection, and source-change invalidation.
- **readiness-1-3 — Major — lines 18, 24, 30:** Backend reuse does not explain connector communication, reconnects, or acknowledgments. Existing commands enqueue transport actions without results. Choose the communication route and access boundary; define success evidence, deadline, retry, and disconnect recovery. Dispatch alone is not confirmation.
- **readiness-1-4 — Major — lines 16–17, 26, 34:** Add a small file/change map and staged checks: connector proof, backend contract, drawer integration. Include queue/source races, duplicate suppression, selection failure/retry, constrained work-area clipping, pointer transitions, scroll isolation, and existing controls. A large replacement design is unnecessary.

Reads: complete implementation-readiness persona and standard; complete target; `CompactMediaPlayer.tsx:1–65`, `types.rs:20–45`, `commands.rs:2560–2630`, `services.rs:1200–1245,1340–1395`; targeted searches. No builds, tests, connector execution, live account checks, screenshots, or installed-binary validation.

## Codebase reviewer: queue_codebase_readiness

Verdict: ready for an illustrative UI prototype, not ready for live implementation under the zero-decision standard. The brief does not falsely claim existing integration. Absence of new-feature code is not itself a defect.

- **codebase-1-1 — Major — target 30–34; sources `services.rs:1219–1237,1348–1357`, `types.rs:24–45`, `Cargo.toml:21–87`:** Existing worker supplies Windows now-playing metadata/timeline and transport commands, not upcoming entries or selection. No source-specific queue connectors found. Internal Tauri IPC/events are reusable, but an external connector bridge is undefined. Perform bounded Spotify Free/Brave proofs for ordered entries, duplicate identities, later selection, and remaining order. Record APIs, versions, confirmation signals, supported pages, and unavailable outcomes. Select bridge transport/lifecycle and justify dependencies.
- **codebase-1-2 — Major — target 14,18,24,30; sources `services.rs:1158–1184,1223–1235,1359–1364`, `types.rs:24–37`, `App.tsx:2511–2519`:** Display selection takes the first playing enumerated session, otherwise first titled session; controls independently address `GetCurrentSession()`. Payloads lack identity and frontend equality ignores source changes. Specify a shared selection authority, browser-tab association, source-generation identity, and transport-routing policy. Verify concurrent Spotify/Brave and identical-metadata source switches.
- **codebase-1-3 — Major — target 18,24,34; sources `commands.rs:2593–2618`, `services.rs:1158–1184`, `App.tsx:2550–2557`:** Existing commands return no playback outcome, ignore channel/WinRT results, and optimistically flip play/pause UI. Define snapshot revision, entry identity, request correlation, revalidation timing, acknowledgment versus observed confirmation, timeout/disconnect/retry, and late-result handling. Preserve ordinary controls deliberately; their current success semantics cannot prove queue-selection success.
- **codebase-1-4 — Major — target 9,13–17,22,26; sources `CompactMediaPlayer.tsx:338–380`, `App.css:2093–2095,2222–2226`, `App.tsx:3032,3065–3071,3093–3098,3165,3270–3299,1768–1794`, `commands.rs:510–529`:** Classic/compact layouts, fixed music heights, volume expansion, space-between controls, and panel-changing wheel behavior need explicit integration choices. Define layout coverage, centered control grouping, combined expansion height, work-area/native-window sizing, close/reset behavior, and wheel containment. Reuse rectangle/region reporting at `App.tsx:796–830`. Check clipping and pointer continuity in supported layouts/constrained areas.

Reads: complete codebase persona and implementation-readiness standard; complete target; root/Bloom AGENTS and cited sources. No memory, transcript, installed app/config inspection, writes, builds, or agents. Frontend build and locked Rust check/clippy/tests are applicable later; not run here.

## Style reviewer: queue_readiness_style

Verdict: **Not applicable — implementation-plan style readiness.** No implementation plan or DetailedRange exists. Lines 32–34 distinguish the specified interface and illustrative concept from verified connector capability and integration. No misleading readiness wording or style blocker found.

Reads: complete style persona and target lines 1–34. No additional standard assigned; no application code, runtime behavior, or connector capability assessed. No files changed or tests run.
