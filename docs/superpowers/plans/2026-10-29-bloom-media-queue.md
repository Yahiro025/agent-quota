# Bloom Media Queue Implementation Plan

> **For agentic workers:** Use executing-plans inline. The main agent implements; bounded research and the final review run separately.

**Goal:** Show the real upcoming queue in Bloom's compact media player and play the selected occurrence without replacing its source context.

**Architecture:** Prove source capabilities before choosing the production bridge. Reuse Bloom's backend and native notch sizing; keep source connectors small. A temporary, loopback-only proof harness is disposable instrumentation, not a second production service.

**Tech stack:** Existing React/TypeScript, Rust/Tauri, Bun test runner for JavaScript connectors, installed Spicetify, Brave extension.

**Spec:** `docs/superpowers/specs/2026-10-29-bloom-media-queue-design.md`.

## Global Constraints

- Original dirty checkout is read-only. This isolated branch starts at buildable `303d3e1`; do not silently lose installed clock-click navigation when later integrating the app.
- Do not merge, push, permanently deploy, restart Bloom/helper, or launch the floating quota widget.
- Credentials remain unread and unlogged. Back up any connector configuration before reversible installation; retain a verifiable restore route.
- One identified session/tab owns track, queue, and controls. Uncertain association disables queue selection.
- Selecting C in upcoming A/B/C/D starts C then D; earlier entries are skipped, context and shuffle/repeat settings preserved. Duplicate URIs require occurrence identities.
- One pending selection per session; five-second observed-playback deadline; no automatic retry after uncertainty; reject stale queue revisions and results.
- Compact player only; classic unchanged; queue and volume expansions exclusive. Preserve native geometry, work-area limits, focus/dismissal behavior and all existing integrations.
- Complete the source capability gate before building the production bridge/drawer. Failed capability requires an explicit scope decision, not a silent fallback.

## Review Focus

- Wrong player/tab with equal metadata: identity, not title matching, controls routing.
- Duplicate track occurrence and shuffle/repeat: preserve actual upcoming order and context.
- Uncertain timeout/disconnect: no blind retry or stale success feedback.
- Native work-area/DPI clipping and pointer transition into a scrolled drawer.
- Installed feature preservation: source baseline must include previously deployed navigation before canary.

## Task 1: Prove Spotify occurrence selection

**Files:** New `bloom/connectors/proof/spotify-snapshot.js`, `spotify-snapshot.test.js`, `spotify-probe.js`, `serve-probe.js`; evidence under `docs/verification/media-queue-proof/`.

**Interfaces:** Consumes the installed `Spicetify` object. Produces bounded, credential-free capability/queue observations and a recorded selection experiment. Exact selection API must come from installed code/runtime evidence, not assumptions about online versions.

- [ ] Write failing Bun tests for a bounded snapshot: no account/token serialization, absent API, duplicate occurrences retained, oversized metadata/queues bounded, throwing getters contained.
- [ ] Run `bun test bloom/connectors/proof/spotify-snapshot.test.js`. Expected: failing snapshot behavior before implementation.
- [ ] Implement the smallest snapshot/probe and run the same command. Expected: all tests pass.
- [ ] Run a temporary loopback harness, back up config/affected extension artifacts, install a read-only Spicetify probe, and record real runtime capabilities. Keep Spotify data/credentials untouched; no Bloom restart.
- [ ] Add a test first for the exact occurrence selection boundary found in runtime. Watch it fail; implement minimal stale/pending/confirmation logic and watch it pass. Do not use URI-only play as a queue-preserving substitute.
- [ ] Perform a controlled live selection and verify actual remaining order, duplicate occurrence, and supported modes. Restore temporary probe configuration. Record exact versions and failures honestly.
- [ ] Commit only scoped proof code/tests/evidence once checked. Task is not complete until live capability evidence meets the spec.

## Task 2: Prove Brave YouTube queue selection

**Files:** New `bloom/connectors/youtube/` source/manifest/tests and proof evidence. Reuse temporary harness instrumentation if useful.

**Interfaces:** Consumes a specific supported YouTube page's playlist/explicit queue and stable tab identity; produces snapshot revisions, distinct occurrence identities, and observed playback confirmation.

- [ ] Write and run failing adapter tests for playlist order, duplicate video occurrences, absent queue, stale revisions, and removed items.
- [ ] Implement the smallest page adapter and run its Bun tests. Expected: pass without inferring autoplay recommendations as a queue.
- [ ] Install the companion extension using a supported method, preserving existing browser profile/settings. Prove playlist and explicit-queue selection, duplicate videos, multiple tabs, unsupported page, and timeout/disconnect. Record evidence; no unsupported browser policy or binary patching.
- [ ] Commit scoped connector/tests/evidence. Failed proof gates downstream work.

## Task 3: Connect proven sources to one playback session

**Files:** Shared media types, commands and worker routing; a focused connector module; source connectors; source-specific tests.

**Interfaces:** Consumes verified source APIs from Tasks 1/2; publishes bounded snapshots and correlated selection results to the frontend. Before execution, record the concrete transport/schema and access restrictions in the task brief using proof evidence.

- [ ] Write/run failing tests for source-generation changes, concurrent sources, revision rejection, pending exclusion, five-second uncertainty, and late results.
- [ ] Implement the minimal bridge inside Bloom's backend, with consistent display/control association and bounded input/state. No new standalone production service.
- [ ] Run locked Rust check/clippy/tests and connector tests. Expected: all applicable checks pass; dispatch acknowledgment cannot impersonate playback confirmation.
- [ ] Commit scoped changes after evidence review.

## Task 4: Integrate the compact Up next drawer

**Files:** Compact player, focused queue UI, media event/state integration and sizing, existing media CSS, focused UI tests.

**Interfaces:** Consumes Task 3's proven snapshots/results; preserves current media transport and native geometry ownership.

- [ ] Reconcile the isolated source with previously deployed clock navigation using a separate scoped compatibility change; do not import the entire original dirty tree.
- [ ] Write/run failing interaction tests for drawer/volume exclusivity, layout switches, source changes, pending/error/empty states, scroll isolation, and long/missing metadata.
- [ ] Implement the approved attached drawer; run tests and frontend build. Expected: pass, with existing media/quota/navigation tests preserved.
- [ ] Verify native work-area/DPI sizing and hit regions, not just DOM height. Commit scoped implementation.

## Task 5: Final acceptance and independent review

- [ ] Run frontend build, focused UI/regression suites, connector tests, locked Rust check/clippy/tests. Record actual failures and warnings.
- [ ] Obtain one fresh-context whole-branch review on a capable Codex model, using the spec, plan, ledger and complete diff. Fix important findings with failing tests first; record all rulings/minors.
- [ ] Only with matched build/rollback readiness, run a human-assisted canary. No permanent deployment, merge or push. Keep the previous installation available.
- [ ] Report live source behavior separately from mocks, native UI acceptance and backend tests. Never mark a gated task complete without its evidence.
