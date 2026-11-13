# Multi-player media cards — design

Status: approved in chat by the user (2026-11-01). Not committed on purpose.

## Intent
Bloom's notch media panel shows exactly one player (one "displayed" source, one queue owner). The user wants to see every player that is currently playing, each with its own controls and its own "Up next". The single-slot design is also why one unattributable audible tab (e.g. AnimeSuge in Brave) used to veto the whole YouTube queue.

## Behavior (user-approved)
- The panel shows one card per **playing** source. Each card has its own controls and its own queue.
- A paused player's card disappears at once (user decision). With two playing, pausing one removes its card and the other stays.
- If fewer than 2 cards are playing, the panel shows exactly what it shows today (the single `media-update` card, including a paused last-displayed player). No visual change in the 0/1-player case.
- Card order is stable: the primary player first, then Spotify, then YouTube, then Windows-only players; ties by generation string. (Builder/orchestrator call, marked as such.)
- Windows-only players (no connector, e.g. Edge/Chrome/VLC) get a card with title, artist and play/pause/next/seek but no "Up next".
- Audio Bloom cannot attribute to a player (connector `unknown_playing`) is not a veto any more. It sets `other_audio`, shown as one small row "Other audio playing" under the cards (only when the stack is shown).
- Pill and notch are unchanged: they still show only the primary player from `media-update`.
- Invariants kept: a queue belongs to its own source+tab and is never inferred from track text; snapshots older than 6000 ms are ignored; one player's state can never disable another player's queue; the connector protocol and the `NoTarget` refusal messages already shipped stay as they are (they now apply per requested generation/primary).
- Non-primary cards have no artwork (placeholder icon). Artwork routing stays primary-only. (Orchestrator call: avoids associating art by text.)

## Contract

### Rust (src-tauri)
`media_queue.rs`, on `Broker`:
- `pub fn targets(&self, now: u64) -> Vec<Target>`: every fresh connector snapshot with `is_playing == true` from a fresh client, as a `Target` (generation `"{client}:{session}"`, same as today). `unknown_playing` does NOT filter anything here. Order: stable per the rule above; the caller marks primary.
- `pub fn target_for(&self, generation: &str, now: u64) -> Option<Target>`: look up by generation among `targets()` plus the primary from `resolve()` (so a paused primary stays addressable exactly like today).
- `pub fn other_audio(&self, now: u64) -> bool`: any fresh client with `unknown_playing`.
- `resolve_action` must use `target_for` instead of `active(now).filter(...)`.
- `resolve()/active()` unchanged (still the primary for pill, `native_allowed`, refusal messages).

`media_bridge.rs`:
- `media_queue_get(generation)` finds the target with `target_for`. If found: same `QueueView` as today. If not found: existing refusal path (NoTarget reasons based on `resolve()`), or "Media changed. Check the queue again." exactly as today when the primary exists but `generation` is unknown.
- New `#[tauri::command] pub fn media_list_get() -> MediaList` and a `pub fn media_list(now) -> MediaList` builder.
- `connector_action` unchanged except it now works for any generation that `target_for` knows.

Types (serde, camelCase NOT used; keep snake_case like `MediaInfo`):
```rust
#[derive(Serialize, Clone, PartialEq)]
pub struct MediaCard {
    pub generation: String,
    pub source: String,        // "Spotify" | "YouTube" | "Windows"
    pub title: String,
    pub artist: String,
    pub is_playing: bool,      // always true in the list
    pub artwork: Option<Vec<String>>, // primary only, else None
    pub position_ms: i64,
    pub duration_ms: i64,
    pub seek_enabled: bool,
    pub position_updated_at: u64,
    pub queue: bool,           // true when connector-backed (the card may ask media_queue_get)
    pub primary: bool,
}
#[derive(Serialize, Clone, PartialEq)]
pub struct MediaList { pub cards: Vec<MediaCard>, pub other_audio: bool }
```
Event `media-list` (payload `MediaList`) is emitted from the existing media thread in `services.rs` whenever the set of generations, any title/artist/is_playing, `other_audio`, or any position by >1000 ms changes (same debounce style as `media-update`). `media-update` is unchanged.

`commands.rs::media_action(generation, op, position_ms)`: if `media_bridge::target_for_generation(&generation)` is `Some` -> `connector_action` (any card); otherwise the existing native path unchanged. (Phase 2 extends the native path to per-session dispatch.)

### Frontend (src)
- A hook `src/media/useMediaList.ts`: `invoke("media_list_get")` once on mount, then `listen("media-list")`. Returns `MediaList` (`{cards: MediaCard[], other_audio: boolean}`), default `{cards: [], other_audio: false}`.
- `src/media/MediaStack.tsx`: renders one `MediaCardView` per card plus the "Other audio playing" row. Each card has its own `QueueDrawer` + its own queue-state instance keyed by `generation` (reuse `src/media/queueState.ts`/`QueueDrawer.tsx`; do not fork them). Controls call `invoke("media_action", {generation: card.generation, op})`.
- The panel (where `CompactMediaPlayer` is rendered in `App.tsx`) shows `MediaStack` when `cards.length >= 2`, otherwise the existing `CompactMediaPlayer` unchanged. The panel height must grow/shrink with the stack the same way `onQueueHeightChange` already drives it for the single card.
- Use existing styling conventions (`mq-*` classes, framer-motion). No new dependencies.

## Non-goals
Card artwork for non-primary players, pause grace period, reordering by drag, per-card volume, changing the pill/notch, changing connector protocol or extensions.

## Phases
1. Connector-backed cards (Spotify + each YouTube tab), `other_audio`, per-generation queue/actions, frontend stack.
2. Windows-only cards (see below).

## Stack visual design: "Focus + shelf" (user-chosen 2026-11-01)
Replaces the equal-card stack. Live failures it fixes: the island-wide `notch-ambient-glow` (one album art) washed every card in the primary player's colour; a long title ran under the `WINDOWS` label; "WINDOWS" is jargon; Spotify showed a placeholder although Windows has its art; "Other audio playing" duplicated a player that already had a card; volume/output disappeared in stack mode; nested card boxes.

Layout (stack mode, `cards.length >= 2`; the 0/1 case stays the existing `CompactMediaPlayer`, untouched):
- **Focus**: exactly one card is focused (default the `primary` card, else the first). It is drawn directly on the island (no card box, no nested surface): row 1 artwork 46px (radius 10px) + title (13.5px/600, single line, marquee on overflow like `cmp-marquee`) and artist (11.5px, muted) in a `flex:1; min-width:0` column + app chip at the right (app glyph 12px + app name, 10.5px/500, sentence case, muted, `max-width: 38%`, ellipsis; never overlaps the title). Row 2 scrubber: elapsed and remaining times (10.5px, `font-variant-numeric: tabular-nums`, muted) either side of a 3px track (5px on hover), fill in text colour; click/drag seeks via `media_action {op:"seek", positionMs}` only when `seek_enabled`, otherwise not interactive (no hover growth). Row 3 transport centred (previous / play-pause 36px / next) with "Up next" toggle at the right only when `queue`; the drawer opens under the focus only.
- **Shelf**: every other card is a 40px row below a 1px inset hairline (`rgba(255,255,255,.07)`): artwork 26px (radius 6px) or app-glyph tile, title (12px/500, ellipsis) then " · " + app (muted), a 28px play/pause button at the right (hit area >= 30px), and a 2px progress hairline along the row's bottom edge (track 8% text, fill 45% text). The whole row is a button "Show <title>" that promotes it to focus; hover = 6% text background, visible `:focus-visible` ring.
- **Promotion**: framer-motion `LayoutGroup`; artwork shares `layoutId={"art-"+generation}` so it morphs 26->46px; rows reflow with `layout`; 0.32s, ease [0.16,1,0.3,1]. Reduced motion: crossfade only. A focused card that disappears (its player paused) hands focus to the primary/first card in the same frame.
- **Footer** (shared): one 28px row: volume icon + slider (the existing system volume, same commands as `CompactMediaPlayer`) and, at the left, a quiet "Other audio playing" note (speaker-wave icon) only when `other_audio`.
- **Glow**: the island's `notch-ambient-glow` uses the FOCUS card's artwork (none if it has none), and in stack mode the `notch-ambient-dim` is always active with `--dim-from` = the focus block's measured height, so the shelf and footer sit on calm black.
- Copy: app names, never "Windows". Titles keep full text in `title` attributes.

Backend support (Rust):
- `MediaCard` gains `app: String`: "Spotify" / "YouTube" for connector cards; for Windows cards a display name from the SMTC AppUserModelId (spotify->Spotify, brave->Brave, msedge->Edge, chrome->Chrome, firefox->Firefox, vlc->VLC, else the exe stem / id prefix title-cased). The frontend falls back to `source` when `app` is missing.
- Per-card artwork: every Windows card carries its own SMTC thumbnail (read through `native_art_cache`, so each distinct title/artist is read once); the Spotify connector card takes the artwork of the single Spotify-family SMTC session when exactly one exists (family match, never text match). YouTube connector cards keep `None` for now.
- `other_audio` is false whenever a Windows card exists for the Brave/YouTube family (that card already is the unattributed audio).

## Browser tab rows (user-chosen 2026-11-01: titled rows, add `tabs` permission)
Why: Windows exposes ONE Brave media session for all tabs (live: it reported AnimeSuge "Paused" while it played), and the extension could see only that some non-YouTube tab was audible. The user accepted Brave's "Read your browsing history" warning to get titled rows. Play/pause stays impossible for such tabs (their players live in cross-origin iframes); rows get Mute/Unmute and Show tab.

Extension (connectors/runtime, Brave service worker `youtube-worker.js`, manifest via `build.js`):
- Manifest `permissions` adds `"tabs"`; version `0.3.0`; description mentions it lists audible tabs' titles to the local Bloom only.
- Each `inventory` message adds `tabs: [{ id, title, site, muted, window }]` for AUDIBLE tabs that are NOT YouTube watch tabs already represented by snapshots: `id` = Chrome tab id (integer), `title` = tab title (trimmed, ≤512 chars; fallback = site), `site` = hostname of `tab.url` (≤253 chars, "" if unavailable), `muted` = `tab.mutedInfo.muted`, `window` = `tab.windowId`. At most 8 (first 8 by window then index). Only audible tabs are ever sent; nothing else about browsing is read or sent. `unknown_playing` stays true only for an audible tab that cannot be described (no title and no URL) or a YouTube page that cannot be bound (existing rule).
- New command ops `mute`, `unmute`, `show` with `session: "tab:<id>"` (client.js action allowlist grows by exactly these three; unknown ops still close 1008). The worker re-checks the tab still exists and is not a YouTube watch tab, then `chrome.tabs.update(id, {muted})` or `chrome.tabs.update(id, {active:true})` + `chrome.windows.update(windowId, {focused:true})`; replies `confirmed`, or `stale` when the tab is gone. The Spotify connector replies `unavailable` to these ops.
- Deployed to a durable folder `%LOCALAPPDATA%\com.sehaz.bloom\connectors\youtube` (the previously registered unpacked folder was deleted; Brave runs it from memory). The user loads it once via brave://extensions > Load unpacked and removes the stale entry.

Rust bridge:
- `Incoming::Inventory` gains `#[serde(default)] tabs: Vec<TabInfo>`; accepted only from YouTube-source clients (ignored for Spotify). More than 8 tabs, or a tab with empty title AND empty site, or out-of-range fields -> the inventory is rejected with a new `Rejected::InvalidTab` (logged by the existing rejection logging). Stored on the client with the same freshness (6000 ms).
- `MediaCard` gains `kind: String` ("player" | "tab") and `muted: bool`. Tab cards: generation `"{client}:tab:{id}"`, `source:"Tab"`, `app:"Brave"`, `title`, `artist` = site, `is_playing:true`, `duration_ms:0`, `seek_enabled:false`, `queue:false`, `artwork:None`, `primary:false`. Order: connector player cards, then tab cards (by window then id), then Windows cards. `same_list` compares the new fields.
- The Brave/YouTube family counts as represented when the YouTube connector shows a playing YouTube card OR any tab card (so the single Brave SMTC session never duplicates a tab row).
- `media_action(generation, op)`: ops gain `mute`, `unmute`, `show`, valid only for a fresh tab generation; they are sent to that client as a command `{op, id, session:"tab:<id>"}` and the reply status is returned (5 s deadline like other connector actions). Player ops on a tab generation and tab ops on a player generation return an error.
- Diagnostics: one content-free change-only line `media-list-summary players=<spotify>/<youtube> tabs=<n> windows=<n> other_audio=<bool>` (no titles, sites or ids).

Panel (frontend):
- `kind:"tab"` cards: art tile = Globe glyph on the 8% tile; title, `artist` (site) as secondary; app chip "Brave". Shelf row's right button is Mute/Unmute (VolumeX when muted) with `aria-label` "Mute <title>"/"Unmute <title>"; muted rows show the title at 60% opacity. As focus: no scrubber, no transport; one row of two labelled buttons centred: [Mute|Unmute] and [Show tab] (icon + text, 12px/500). No glow (no artwork).

## Connector pull protocol (found live 2026-11-01)
Cause: connectors poll with a chained `setTimeout` (client.js `poll`, 2 s). When Spotify's window is hidden for 5+ minutes Chromium's intensive timer throttling cuts a chain of ~5 timers to one wake-up per minute, the bridge's 8 s idle rule (`media_bridge.rs` `socket`) drops the socket, the client reconnects and the cycle repeats. Live log: `media-connector-closed source=Spotify reason=Idle lifetime_ms≈18000 messages=5 max_gap_ms≈3000`. WebSocket message events are not throttled, so the server drives the cadence.
- Opt-in, backward compatible in both directions: a client that supports it adds `"pull": true` to its `inventory` messages. `Incoming::Inventory` gets `#[serde(default)] pull: bool`. Old servers ignore the unknown field; old clients never send it.
- For a connection that has sent `pull:true`, the server sends `{"op":"poll"}` about every 2000 ms (driven by the existing 250 ms tick, timeout/failure handling like other sends). Clients that never opted in receive no `poll`.
- Client: on `{"op":"poll"}` (accepted before the action validation, which still closes on unknown ops), publish the inventory immediately. Once a poll has been seen, stop re-arming the `setTimeout` chain (keep it as the fallback for an old server).
- No change to the 8 s idle rule, the 6 s freshness window or any bound.

## Phase 2 — Windows-only cards (Rust only; no frontend change, `queue:false` and `source:"Windows"` are already in the contract)
- The media thread in `services.rs` currently stops at the first playing SMTC session (`break`). Collect ALL SMTC sessions that have a title instead, while keeping the choice of the displayed/primary session (`best_session`) identical to today (first playing, else first with a title). Do NOT regress cost: thumbnail/artwork reading stays only for the session that becomes the displayed one (skip it for later sessions, reuse `native_art_cache` as today).
- A playing SMTC session becomes a native card only if it is NOT already represented by a connector: a session whose app family is Spotify or Brave (existing `family` mapping) is represented when a fresh connector client of that source exists; otherwise it gets a card. (Brave exposes one SMTC session for all tabs, so Brave tabs only ever appear via the connector.)
- Each native card has a stable generation: a uuid per SMTC session identity (the existing `IUnknown` raw pointer `identity`), kept in a map across polls and pruned when the identity disappears. The displayed session keeps using the existing `native_generation`, so the pill/primary path is unchanged.
- Card fields: `source:"Windows"`, `queue:false`, `artwork` only for the displayed session, `primary` true only for the displayed session when no connector target is the primary. Title/artist/position from the SMTC info. Sort after connector cards, by generation.
- Shared state (`media_bridge.rs`): add a way for the media thread to publish the current native cards (`set_native_cards(Vec<MediaCard>)`), appended by `media_list`/`build` after connector cards. `native_allowed(generation)` additionally accepts any published native card generation. (The existing `generation == native_generation && active().is_none()` rule stays for the primary.)
- Actions: the `MediaTarget{generation,op,position_ms}` handler in `services.rs` must dispatch to the SMTC session that owns `generation` (map generation -> session kept in the media thread), falling back to `displayed_session` for `native_generation` exactly as today. A generation that is not currently known is ignored.
- `other_audio` stays connector-only.
