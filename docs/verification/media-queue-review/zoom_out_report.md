# Initial Zoom Out Review

Target: `docs/superpowers/specs/2026-10-29-bloom-media-queue-design.md`, lines 1–34.
Stage: EGM Stage 5. Reviewer: fresh-context `queue_initial_review`.
Disposition: Pass as an interface brief. No actionable Blocker, Major, or Nit findings. No finding IDs assigned.

## Assessment

The objective is sufficiently clear: inspect upcoming media and play an entry without leaving Bloom (line 7). The selected attached drawer directly serves that objective while preserving the current player (line 9). It fits Bloom's documented role as a Windows notch with media information and playback controls.

The scope remains appropriately small. Lines 13–18 describe one entry point, one drawer, and one primary row action. Lines 15 and 30 explicitly defer artwork downloads, action menus, queue editing, search, reordering, source switching, and additional services. There is no speculative abstraction, generalized integration framework, independent service, or premature caching scheme to remove.

The document stays at the right altitude. Motion duration, row content, pending/error behavior, accessibility, and scrolling boundaries are appropriate interface requirements. It does not prescribe helper functions, transport protocols, or internal state machinery. Connector implementation details should remain in a subsequent technical design.

The brief also preserves the root behavior rather than accepting a superficial substitute. Lines 18 and 24 require confirmed results, duplicate prevention, distinct entry identities, and queue revalidation. Lines 30–34 require actual upcoming entries and order parity, explicitly distinguish illustrative data from integration evidence, and prohibit silently discarding the remaining source queue. Spotify Free-account selection and browser capability remain acknowledged uncertainties, rather than asserted capabilities. Those uncertainties do not invalidate an explicitly scoped interface brief.

## Reuse verification

`CompactMediaPlayer.tsx` already supplies the artwork, track information, progress, centered transport, audio-output button, and expanding volume section. Its existing appearance and `App.css` styles provide the intended extension point; the brief does not propose replacing them.

`App.tsx` already owns player mounting, panel dimensions, media events, and wheel navigation. Existing wheel exclusions demonstrate an established pattern for keeping scrolling content from changing notch pages. Existing expansion sizing and dismissal ownership are also available to extend. No separate drawer window is justified or proposed.

The backend already exposes transport commands and publishes media updates through its Windows media-session worker. Searches across `bloom` found no existing Spicetify connector, companion browser extension, or queue-selection implementation to reuse. The current worker reports playback metadata, not upcoming entries. Reusing Bloom's backend therefore means extending its existing integration boundary; it is not evidence that queue capability already exists. The brief correctly retains this distinction.

## Confirmed reads

- EGM `agents/zoom_out.md` and `resources/document_standards/zoom-out-standard.md`, completely.
- Target brief, all lines 1–34, decoded as UTF-8.
- Repository `AGENTS.md`, `bloom/AGENTS.md`, and `bloom/README.md`, completely.
- `bloom/src/CompactMediaPlayer.tsx`, lines 1–383.
- `bloom/src/App.css`, lines 1120–1180 and 2065–2408.
- `bloom/src/App.tsx`, lines 1755–1805, 2495–2580, 2990–3102, and 3260–3430.
- `bloom/src-tauri/src/commands.rs`, lines 2581–2645.
- `bloom/src-tauri/src/services.rs`, lines 1219–1393.
- Repository searches for queue implementations, Spotify/Spicetify, companion extensions, and relevant reference artifacts.

## Limitations

The supplied screenshot and interactive concept were unavailable in this isolated context. Their fidelity, layout fit, and visual quality were not assessed. Root `README.md` and `context.md` were absent; Bloom's README supplied project context. No live connectors, runtime behavior, or installed binaries were tested. No files were modified by the reviewer; root persisted this report.
