# Media queue canary, 2026-10-30

Status: canary run and rolled back. Nothing is merged, pushed or permanently deployed. Each
restore put the original Bloom and helper back (hashes verified). After a later reboot, a second
Bloom install on autostart replaced the helper with its own; see the end of this document.

## Result

The queue drawer works end to end in a real Bloom notch against real Brave and YouTube, driven
with real mouse input. The headline case passes: with the same video queued twice, clicking the
second occurrence plays the second occurrence.

| Check | Result | Evidence |
| --- | --- | --- |
| Bloom starts and talks to the matched helper | Pass | helper `ready` in diagnostics; helper hash unchanged `AC775B52...` |
| Notch shows connector-owned media with source label | Pass | "YouTube - Brave", title/artist from the page |
| Queue lists upcoming entries incl. a duplicate | Pass | `canary-queue-drawer-duplicates.png` (items 1 and 3 are the same video) |
| Click second duplicate selects that occurrence | Pass | URL went to `v=jNQXAC9IVRw&list=<same list>&index=4`; notch "Me at the zoo / jawed"; drawer "Playing selected item." (`canary-second-duplicate-selected.png`) |
| Single video (no playlist) fails closed | Pass | "Queue unavailable for this YouTube page." |
| Two playing YouTube tabs fail closed | Pass (by design) | backend returned the generic "Pause other active players" reason until one tab was paused |
| Bloom play/pause on connector media | Pass | Windows session went Playing -> Paused -> Playing |
| Codex shown in the notch matches the standalone reader | Pass | UI 5h 100% resets 1h 30m, week 73% resets 5d 12h; `--print` 5h 100%, week 73%, reset 04:10 |
| Other providers | Pass | Kiro 3, Cursor 0, Copilot 13, Devin 92, Antigravity 94 matched `--print`; Claude refreshed on its own 35 (stale) -> 43 -> 51 |
| Manual refresh | Pass | real click: "Checking..." then "Just checked" in about 4 s |
| Automatic refresh | Pass | Claude values advanced with no click |
| Settings window | Pass | opened, rendered, closed with a real click |
| Helper killed | Pass | respawned about 1 s later, values repopulated, no orphans |
| Clean shutdown | Pass | WM_CLOSE: taskbar restored, helper and all webview children gone |
| Resource growth | No growth seen over ~20 min | Bloom 31 MB commit / 27 MB private WS; helper 57-61 MB commit / 11-14 MB private WS |

## Defects found

1. **Candidate packaging (not a source defect).** The first candidate (sha `513282A9...`) was built
   with plain `cargo build --release`, which does not enable `tauri/custom-protocol`. Its webviews
   navigated to `http://localhost:1420` where nothing listens, so the notch window never showed.
   Hash, payload, vite e2e and cargo checks could not catch this because none launched the exe.
   Rolled back immediately. The rebuilt candidate (`7AEB9943...`) embeds the frontend; its
   webviews load `http://tauri.localhost/`.
2. **Connector went permanently dead (fixed in the working tree).** After a failed first
   connection while Bloom was not listening, Brave suspended the service worker. Nothing woke it,
   so every Bloom restart left the connector "Inactive" until the extension was reloaded by hand
   (seen live, with an Errors entry). Fix: a repeating `alarms` event wakes the worker, and
   loading it reconnects. Manifest permissions are now `scripting` and `alarms` (the build test
   that pinned `["scripting"]` was changed first and seen failing). Live check: Bloom off 75 s,
   restarted, connector reconnected in 11 s with no reload. `bun test connectors src`: 112 pass.
   This adds a permission, so it needs your decision.
3. **Stale cover art (fixed in the working tree, verified live in v3).** For a few seconds after
   the playing tab changed, the notch showed the previous video's cover next to the new title.
   Cause: browsers publish a new title before its thumbnail, and the art cache stored the old
   image under the new title. See "Follow-up" below.

## Follow-up: cover art and drawer polish (candidates v3-v5)

Same procedure: each candidate ran from the artifact folder, never over `bloom.exe`, and the
installed hashes were checked before every swap and at the restore.

**Cover art** (`media_artwork.rs`, `services.rs`). After a track change, an image identical to the
previous track's is withheld until a different one arrives, unless the two tracks can share a
cover: same title and artist (a replay or a duplicate queue entry) or the same non-empty album and
album artist. Windows reports album and album artist for Spotify ("711" / TONEEJAY on this
machine) and none for YouTube in Brave. An unexplained identical image appears after 10 s but stays
provisional and uncached, so a thumbnail arriving even later still replaces it. Only accepted art
is cached. It applies to connector and native-only playback. 13 artwork unit tests; the
duplicate-entry test was seen failing against the earlier 6 s gate.

Live (v3, every `media-update` the notch received was recorded with a cover fingerprint):

| Transition | Result |
| --- | --- |
| Dev Live -> Me at the zoo (selected in the drawer) | new cover arrived with the new title; no old cover in between |
| Me at the zoo -> the second Me at the zoo (duplicate selected) | cover stayed; no placeholder gap |
| Playlist end -> YouTube autoplay video | matching cover at each step |

Each recorded cover matched its own video (`live-v3-covers-received.png`).

**Drawer and player** (`QueueDrawer.tsx/.css`, `queueState.ts`, `CompactMediaPlayer.tsx`,
`App.tsx/.css`):

- The island fits the drawer's content (max 236 px for the drawer) instead of a fixed 212 px slot,
  so empty and unavailable queues no longer leave a blank band. The drawer reports the whole island
  height it needs, measured from the rendered player. v4 found that an assumed 132 px player was
  really 132.8 px plus 3 px padding, which gave short lists a needless scrollbar. Live in v4: 3 rows,
  content 132 px in a 132 px box.
- A shrinking island never leaves the pointer outside it. v4 showed that choosing the last item
  emptied the list, shrank the island from 313 to 178 px under the pointer, and closed the notch.
  v5 holds the island just below the pointer until the pointer moves above the new edge. Live:
  306 px held with the drawer open, then 211 px once the pointer moved up. An e2e test fails
  without the hold.
- The list shows 3.5 rows with a bottom fade, so it reads as scrollable. It uses Bloom's 4 px
  themed scrollbar without arrow buttons (WebView2 drew arrows with `scrollbar-width`).
- The cover's ambient glow fades out behind the list. On a bright cover, white muted text measured
  about 1.7:1 contrast on the glow; the list now sits on the island colour.
- Type follows the player: 13 px titles and 11 px secondary text (was 12 and 10 px).
- Rows dim while a choice is pending or must be re-checked. The play glyph also shows on keyboard
  focus. Player and drawer buttons have visible focus rings.
- "Playback is uncertain" now carries its recovery inline ("Check again"). "Playing selected item."
  clears after 4 s. The drawer shows "Loading queue…" instead of flashing "unavailable" before its
  first reply.
- Design states at real DPR: `drawer-polish-states.png` (list, empty, uncertain, light theme).

Candidates: v3 `2DD8F7B0…` (art live test), v4 `F2FF1F2A…` (height fix), v5 `112998EB…` (pointer
hold). None installed. The success-note timer fix came after v5 (unit-tested only).

Gates on the final code: `cargo test` 35 + 123 (1 ignored), `cargo clippy --all-targets` exit 0
with no warnings in changed code, `bun test connectors src` 113, `bun run build` ok, full
Playwright 37/37.

Not verified live: a Spotify same-album transition, and the 10 s provisional path.

## Not verified

- Spotify and the Spicetify bridge (needs changes to Spotify; out of scope for this run).
- The overlay (OSD) window, autostart at login, multi-monitor, and DPI other than 125%.
- Selection while a command is pending or the connector drops mid-action (unit-tested only).
- Soak longer than about 20 minutes.
- The candidate was run from its artifact folder, not installed over `bloom.exe`.

## Candidate and state at the end

- Candidate (not installed): `bloom-candidate-v2.exe`, sha256
  `7AEB9943B60B2547B8C8399F93FCC81A468A0115B963CBA1645720B679366A5E`, in the canary artifact
  folder. Embedded helper is byte-identical to the installed optimized helper.
- Installed `bloom.exe` `74B4729E...` and helper `AC775B52...` match `rollback-manifest.json`.
- The connector config added for the test was removed. The unpacked "Bloom Media Connector"
  extension you loaded stays in Brave and will log failed connection attempts until a candidate
  runs; remove or disable it if you do not want that.
- Your jazz playback was paused for under two minutes through Bloom's own button and resumed.
- Follow-up runs: your jazz tab and Spotify were already paused and were left paused. The test
  playlist tabs YouTube autoplayed into were paused, but stay open in Brave (three YouTube tabs:
  two "Tokyo Lo-fi Hiphop" and one "90s Japanese Lofi").
- After the follow-up the machine rebooted (09:13). Autostart launches a second install,
  `%LOCALAPPDATA%\Programs\Bloom\bloom.exe` (28 Oct, `76255E8C…`), not the
  `%LOCALAPPDATA%\bloom\bloom.exe` this canary restored (still `74B4729E…`). At start it rewrote
  `agentquota.exe` with its own embedded helper (`76183F4A…`), replacing the optimized `AC775B52…`.
  The backup of the optimized helper is intact in the artifact folder. Nothing was changed.
