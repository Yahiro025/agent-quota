# Bloom media queue: interface brief

Date: 2026-10-29. Amended after EGM review and user direction.

## Purpose and selected direction

Bennett should be able to see what plays next and immediately play an upcoming item without leaving Bloom's media panel. Quick recognition and reliable actions matter more than decoration.

The user selected an **attached drawer below the player**, keeping playback controls visible. Extend the existing Bloom media appearance; do not redesign its artwork, transport, progress, volume, audio output, or notch navigation. The [compact media component](../../../bloom/src/CompactMediaPlayer.tsx), its [styles](../../../bloom/src/App.css), and the [user-supplied media-panel screenshot](../../verification/media-queue-design/reference-media-panel.png) are the visual authority. The screenshot is an unchanged copy of the supplied reference, not a capture of this feature. The repository's native quota widget design is not a replacement style for the media player.

The attached drawer preserves access to playback while choosing what comes next. A replacement queue page would hide those controls; a separate floating surface would add window and dismissal behavior without helping this task.

## Layout scope

The first version adds queue access to the **compact player shown in the reference**. Preserve classic mode and its existing controls; adding queue access there is deferred. Switching to classic closes the drawer, and returning to compact starts with it closed.

Volume and queue share one expansion area: opening either closes the other. Keep transport and audio-output controls available, with transport centered despite the new queue button. Drawer height must update Bloom's native window and hit-test region as well as its visible content, clamped to the monitor's available work area. Moving from the player into the drawer must remain inside the notch's interaction region.

## Layout and interaction

- Add one queue-list icon immediately before the existing headphones/audio-output control. Give it an `Up next` accessible label and tooltip, an expanded state, and a restrained selected background while open. Preserve the centered transport group and all existing controls.
- Expand the same media surface downward. Separate the drawer with one quiet inset divider, rather than a floating window or another nested card. Its header reads `Up next`, with the active source named on the right, for example `Spotify` or `YouTube · Brave`. The source label is not a switcher.
- Use flat rows with queue order, title, artist/channel, and duration when supplied. The full row plays the item; a hover play affordance can replace its order number. No per-row action menu, repeated source badges, or extra artwork downloads in the first version. Long titles elide visually while retaining their complete accessible name.
- Show approximately four rows, then scroll the list. Grow only within the monitor's available work area; on smaller areas, reduce the visible row count. Keep the player controls and drawer header outside the scrolling list. Scrolling here must not also switch notch pages.
- Toggle the drawer with the same queue button; close it when the media panel closes. Moving the pointer into the drawer must not collapse the notch. Preserve Bloom's focus and dismissal behavior. Do not add window activation, Escape handling, or click-outside dismissal for this feature.
- On selection, show a pending state on that row and follow the confirmation rules below. Keep the drawer open. Update the current title and queue only when actual playback is confirmed; a successful dispatch alone is insufficient. If the active session changes, clear its former queue and pending feedback immediately.

## Session and selection contract

The displayed track, queue, and playback actions must refer to the same identified playback session. A source name such as `YouTube · Brave` is a label, not sufficient identity: separate tabs must remain distinct. If Bloom cannot reliably associate the queue with its displayed and controlled session, disable queue selection and show `Queue unavailable for this player`. Do not guess from title/artist alone or choose another tab silently. Preserve existing playback behavior for sources without queue support.

Selecting a later occurrence skips preceding upcoming entries rather than moving or re-adding them. For upcoming `[A, B, C, D]`, selecting `C` starts that occurrence immediately and leaves `D` next. It must not replace the underlying playlist/context or change shuffle/repeat settings. After the remaining entries, continuation follows the source's existing mode. For `[A, B, A, D]`, selecting the second `A` must select that occurrence, not the first matching title or URI. Supported shuffle/repeat modes must honor the actual reported upcoming order; modes that cannot meet this contract remain explicitly unavailable.

Before acting, revalidate the session identity, queue revision, and entry identity against the displayed snapshot. Reject a stale or removed entry and refresh the list; do not silently select whatever now occupies that row. Use one outstanding queue-selection request per session and disable all queue-row actions while it is pending. The bridge must enforce this rule as well as the UI.

Use a five-second confirmation deadline. Observe the selected occurrence playing in the intended session before declaring success; acknowledgment alone does not prove playback. On timeout or disconnect, end the pending indication and show that the result is uncertain. Recheck actual playback and obtain a fresh queue before enabling a retry; never automatically resend an uncertain selection. Match results to their request and session generation so late results cannot overwrite a newer session or request. Closing the drawer must not resend the command or assume an already dispatched command was canceled.

## States and motion

Opening uses one short expansion, approximately 160–200 ms; reduced-motion preference uses an immediate reveal. Avoid continuous decorative animation or animated queue reordering.

Distinguish `Loading queue…`, `Nothing up next`, `Queue unavailable for this player`, a disconnected connector with a specific recovery action, and a failed play request. Empty and unavailable are different states. Preserve playback controls in every queue state. Missing durations stay absent. A changed queue must be revalidated before executing a selection; repeated tracks require distinct queue-entry identities.

Maintain readable contrast, visible focus when the host permits keyboard interaction, proper button semantics and expanded/control relationships, and usable pointer targets. Row selection must not depend on hover. Verify long video titles, duplicate entries, missing metadata, and large queues.

## Frugal scope and acceptance

First-version sources are the existing Spicetify-enabled Spotify desktop installation and supported YouTube playlists/explicit queues in Brave through a companion extension. Use the active player's actual upcoming entries, not recommendations. Queue editing, reordering, searching, source switching, and other services are deferred. Reuse Bloom's backend; avoid another standalone background service and unbounded queue/artwork caches.

Spicetify is the existing Spotify customization host; a small new Spicetify extension will supply Spotify queue access. A new Brave companion extension will supply supported YouTube page queues and selection. Neither connector is implemented or validated by this document.

Live Spotify Free-account queue selection and preservation of remaining playback order still require testing. Do not silently substitute playback behavior that discards the source queue. Browser playback restrictions and unsupported page types must produce the defined unavailable or failed-request states.

## Delivery sequence and connector-test gate

1. **Prove the connectors before building the complete feature.** For Spotify Free and supported YouTube playlists/explicit queues in Brave, capture the real upcoming order, select a later occurrence, observe playback confirmation, and compare the remaining order with the selection contract above. Include repeated tracks, source-supported shuffle/repeat, concurrent Spotify/Brave playback, multiple YouTube tabs, and an unsupported browser page. Exercise a timeout/disconnect without blind retry. Record the APIs, installed versions, supported contexts, and observed results. A failed proof requires an explicit scope/behavior decision; it must not silently weaken the contract.
2. **Choose the smallest communication boundary using that evidence.** Reuse Bloom's backend and Tauri events/commands; select the external connector route, access restrictions, reconnect lifecycle, bounded queue transfer, and request/result messages. Carry session identity, queue revision, distinct entry identity, request identity, and confirmation outcome. Avoid a general integration framework or another always-running service. Establish consistent display/control routing before exposing queue actions. No bridge transport or dependency is assumed proven yet.
3. **Integrate the compact drawer against the proven contract.** Keep queue rendering separate from the existing transport controls, and reuse the main notch's sizing and region ownership. Expected change areas are the compact player and styles, media events/state and geometry in the main app, shared Rust media types/commands/worker routing, and the two small source connectors. Keep unrelated quota, dock, settings, and classic-player work outside scope.
4. **Verify the complete feature.** Run frontend build/type checks and focused UI tests, locked Rust check/clippy/tests, connector integration tests, then a human-assisted Windows canary with a verified rollback. Preserve settings and existing integrations. Publishing, merging, pushing, or permanently deploying requires separate approval.

## Acceptance checks

- Each supported source passes the connector-test gate, including exact occurrence selection and remaining-order parity. An unsupported context cannot display a recommendation list as an actual queue.
- Concurrent sources, multiple tabs, and identical titles cannot misroute a command. Source changes, changed revisions, removed rows, and late results cannot apply stale selections or feedback.
- Rapid clicks across different rows produce at most one outstanding request per session. Acknowledgment without playback is not success; a timed-out result is rechecked before retry.
- Compact/classic switches and notch closure reset drawer visibility. Queue and volume never expand together. Native region/height changes preserve pointer continuity and fit the available work area, with the list scrolling independently of notch navigation.
- Check long titles, duplicate entries, missing durations, large queues, loading/empty/unavailable/disconnected/error states, readable contrast, accessible names, and reduced motion. Queue transfer and retained state remain bounded; visible-row count alone is not a memory bound.
- Existing play/pause, skip, seek, volume, audio output, classic layout, settings, notch navigation, quota integrations, and taskbar restoration continue working. Compare actual playback and queue behavior with the source app, not just mocked backend responses.

The interactive design concept uses illustrative data and is not evidence of live Spotify or browser integration. This amendment changes documentation and preserves a reference image only; application code and installed binaries remain unchanged. The [EGM review](../../verification/media-queue-review/consolidated_review_report.md) describes the previous revision and remains unchanged as historical evidence. These amendments have not received a new independent review or runtime validation.
