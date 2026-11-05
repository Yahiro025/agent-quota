//! Authenticated, bounded connector transport inside Bloom's existing process.
use crate::media_queue::Source;
use serde::{Deserialize, Serialize};
use tokio_tungstenite::tungstenite::handshake::server::Request;

#[derive(Clone, Deserialize, Serialize)]
pub struct Settings {
    pub port: u16,
    pub spotify: String,
    pub youtube: String,
}
fn authenticate(request: &Request, settings: &Settings) -> Option<Source> {
    if request.headers().get("Host")?.to_str().ok()? != format!("127.0.0.1:{}", settings.port) {
        return None;
    }
    let origin = request.headers().get("Origin")?.to_str().ok()?;
    let (source, secret) = match request.uri().path() {
        "/spotify" if origin == "https://xpui.app.spotify.com" => {
            (Source::Spotify, &settings.spotify)
        }
        "/youtube"
            if origin
                .strip_prefix("chrome-extension://")
                .is_some_and(|id| {
                    id.len() == 32 && id.bytes().all(|b| (b'a'..=b'p').contains(&b))
                }) =>
        {
            (Source::Youtube, &settings.youtube)
        }
        _ => return None,
    };
    let protocols: Vec<_> = request
        .headers()
        .get("Sec-WebSocket-Protocol")?
        .to_str()
        .ok()?
        .split(',')
        .map(str::trim)
        .collect();
    if protocols.len() != 2
        || protocols[0] != "bloom.v1"
        || protocols[1].len() != 32
        || secret.len() != 32
    {
        return None;
    }
    let different = protocols[1]
        .bytes()
        .zip(secret.bytes())
        .fold(0u8, |sum, (a, b)| sum | (a ^ b));
    (different == 0).then_some(source)
}

use crate::media_queue::{validate, Broker, Entry, NativeHint, NoTarget, Snapshot, TabInfo, Target};
// The diagnostics log exists only in the app binary, not the lib test crate.
#[cfg(not(test))]
use crate::media_queue::{invalid_field, Rejected};
use serde_json::{json, Value};
use std::collections::HashMap;
use tokio::sync::{mpsc, oneshot};
struct Pending {
    generation: String,
    client: String,
    session: String,
    entry: Option<Entry>,
    expires: u64,
    reply: oneshot::Sender<String>,
    /// A tab command answers with its own status and no snapshot, so `finish`
    /// must not ask a player's confirmation rules to judge it.
    tab: bool,
}
#[derive(Default)]
struct State {
    broker: Broker,
    senders: HashMap<String, mpsc::Sender<Value>>,
    pending: HashMap<String, Pending>,
    uncertain: HashMap<String, u64>,
    // Windows-only players have no connector to publish them, so the media
    // thread hands them over here for `build` to append.
    native_cards: Vec<MediaCard>,
}
const PLAYER_OPS: [&str; 5] = ["select", "playPause", "next", "previous", "seek"];
const TAB_OPS: [&str; 3] = ["mute", "unmute", "show"];
impl State {
    fn prepare(
        &mut self,
        generation: &str,
        op: &str,
        revision: Option<&str>,
        entry: Option<&str>,
        position: Option<i64>,
        now: u64,
    ) -> Result<oneshot::Receiver<String>, String> {
        let tab = self.broker.tab_for(generation, now);
        let tab_op = TAB_OPS.contains(&op);
        // Tab and player generations are disjoint address spaces, so each op
        // family only reaches its own kind. Unknown ops fail both checks.
        if tab_op != tab.is_some() || (!tab_op && !PLAYER_OPS.contains(&op)) {
            return Err("Unsupported media action.".into());
        }
        if self.pending.len() >= 16
            || self.uncertain.contains_key(generation)
            || self.pending.values().any(|p| p.generation == generation)
        {
            return Err("Waiting for the previous action. Check playback before retrying.".into());
        }
        if let Some((client, tab)) = tab {
            let sender = self.senders.get(&client).ok_or("Player disconnected.")?;
            let id = uuid::Uuid::new_v4().simple().to_string();
            let (tx, rx) = oneshot::channel();
            let session = format!("tab:{}", tab.id);
            let command = json!({"op":op,"id":id,"session":session});
            sender
                .try_send(command)
                .map_err(|_| "Player is busy or disconnected.")?;
            self.pending.insert(
                id,
                Pending {
                    generation: generation.into(),
                    client,
                    session,
                    entry: None,
                    expires: now + 5000,
                    reply: tx,
                    tab: true,
                },
            );
            return Ok(rx);
        }
        if (op == "select" && entry.is_none()) || (op != "select" && entry.is_some()) {
            return Err("Unsupported media action.".into());
        }
        let (target, selected) = self
            .broker
            .resolve_action(generation, revision, entry, now)?;
        if op == "seek"
            && (!target.snapshot.seek_enabled
                || position.is_none_or(|p| p < 0 || p > target.snapshot.duration_ms))
        {
            return Err("Seeking is unavailable.".into());
        }
        let sender = self
            .senders
            .get(&target.client)
            .ok_or("Player disconnected.")?;
        let id = uuid::Uuid::new_v4().simple().to_string();
        let (tx, rx) = oneshot::channel();
        let command = json!({"id":id,"op":op,"session":target.snapshot.session,"revision":target.snapshot.revision,"entry":selected,"position_ms":position});
        sender
            .try_send(command)
            .map_err(|_| "Player is busy or disconnected.")?;
        self.pending.insert(
            id,
            Pending {
                generation: generation.into(),
                client: target.client,
                session: target.snapshot.session,
                entry: selected,
                expires: now + 5000,
                reply: tx,
                tab: false,
            },
        );
        Ok(rx)
    }
    fn finish(
        &mut self,
        client: &str,
        id: &str,
        status: &str,
        snapshot: Option<Snapshot>,
        now: u64,
    ) {
        self.reap(now);
        if !self.pending.get(id).is_some_and(|p| p.client == client) {
            return;
        }
        let p = self.pending.remove(id).unwrap();
        let mut observed = false;
        // A tab row is not a session: no snapshot can confirm it, and a
        // connector must not smuggle one in under a tab's session name.
        if !p.tab {
            if let Some(c) = self.broker.clients.get_mut(client) {
                if let Some(s) = snapshot.filter(|s| {
                    s.session == p.session
                        && validate(c.source, s)
                        && now.saturating_sub(s.observed_ms) < 6000
                        && s.observed_ms <= now + 1000
                }) {
                    if let Some(old) = c
                        .inventory
                        .iter_mut()
                        .find(|old| old.session == p.session && old.observed_ms <= s.observed_ms)
                    {
                        observed = p
                            .entry
                            .as_ref()
                            .is_none_or(|e| s.current_id == e.id && s.is_playing);
                        *old = s;
                    }
                }
            }
        }
        let result = if p.tab {
            // The connector's tab command answers with its own status; there
            // is no snapshot to confirm, so "confirmed" stays "confirmed".
            match status {
                "confirmed" | "dispatched" | "stale" | "unavailable" | "busy" => status,
                _ => "uncertain",
            }
        } else {
            match status {
                "confirmed" if observed => "confirmed",
                "dispatched" if p.entry.is_none() => "dispatched",
                "stale" => "stale",
                "busy" => "busy",
                "unavailable" => "unavailable",
                _ => "uncertain",
            }
        };
        if result == "uncertain" {
            self.uncertain.insert(p.generation, now);
        }
        let _ = p.reply.send(result.into());
    }
    fn reap(&mut self, now: u64) {
        let expired: Vec<_> = self
            .pending
            .iter()
            .filter(|(_, p)| p.expires <= now)
            .map(|(id, _)| id.clone())
            .collect();
        for id in expired {
            if let Some(p) = self.pending.remove(&id) {
                self.uncertain.insert(p.generation, now);
                let _ = p.reply.send("uncertain".into());
            }
        }
    }
    fn inventory(
        &mut self,
        client: &str,
        source: Source,
        snapshots: Vec<Snapshot>,
        tabs: Vec<TabInfo>,
        unknown: bool,
        now: u64,
    ) -> bool {
        // The broker's bool cannot say which bound refused the inventory, so
        // name the reason once per episode in the diagnostics log.
        #[cfg(not(test))]
        let (count, field) = (
            snapshots.len(),
            snapshots
                .iter()
                .find(|s| !validate(source, s))
                .map(|s| invalid_field(source, s)),
        );
        if let Err(_rejected) =
            self.broker
                .try_update_tabs(client, source, snapshots, tabs, unknown, now)
        {
            #[cfg(not(test))]
            record_rejection(
                source,
                _rejected,
                // Only a validation refusal has a field; other bounds do not.
                if _rejected == Rejected::InvalidSnapshot {
                    field.unwrap_or("-")
                } else {
                    "-"
                },
                count,
                unknown,
            );
            return false;
        }
        #[cfg(not(test))]
        clear_rejection(source);
        let c = &self.broker.clients[client];
        self.uncertain.retain(|generation, deadline| {
            // A tab generation names a row, not a session, so an accepted
            // inventory is the only observation the connector can give: it
            // clears that generation's uncertainty.
            if generation.starts_with(&format!("{client}:tab:")) {
                return false;
            }
            !generation.starts_with(&format!("{client}:"))
                || c.inventory.iter().any(|s| {
                    *generation == format!("{client}:{}", s.session) && s.observed_ms <= *deadline
                })
        });
        true
    }
    fn disconnect(&mut self, client: &str) {
        self.broker.clients.remove(client);
        self.senders.remove(client);
        let ids: Vec<_> = self
            .pending
            .iter()
            .filter(|(_, p)| p.client == client)
            .map(|(id, _)| id.clone())
            .collect();
        for id in ids {
            if let Some(p) = self.pending.remove(&id) {
                let _ = p.reply.send("disconnected".into());
            }
        }
        self.uncertain
            .retain(|generation, _| !generation.starts_with(&format!("{client}:")));
    }
}

use futures_util::{SinkExt, StreamExt};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;
use tauri::{Emitter, Manager};
use tokio_tungstenite::{
    accept_hdr_async_with_config,
    tungstenite::{protocol::WebSocketConfig, Message},
};

fn shared() -> &'static Arc<Mutex<State>> {
    static STATE: OnceLock<Arc<Mutex<State>>> = OnceLock::new();
    STATE.get_or_init(|| Arc::new(Mutex::new(State::default())))
}
// The UI polls this command, so one line per distinct refusal episode is
// enough; repeating it every poll would bury the first occurrence.
// lib.rs compiles only the pure media modules for its test target, so these
// live only in the app binary, where the diagnostics log lives too.
#[cfg(not(test))]
fn refused_episode() -> &'static Mutex<Option<String>> {
    static LAST: OnceLock<Mutex<Option<String>>> = OnceLock::new();
    LAST.get_or_init(|| Mutex::new(None))
}
#[cfg(not(test))]
fn reset_refusal_episode() {
    if let Ok(mut last) = refused_episode().lock() {
        *last = None;
    }
}
// A refused inventory repeats every poll, so keep the episode keyed by the
// reason and log only when it changes; an accepted update clears it so the
// next refusal is a new episode.
#[cfg(not(test))]
type RejectionEpisode = (Source, Rejected, &'static str);
#[cfg(not(test))]
fn rejection_episode() -> &'static Mutex<Option<RejectionEpisode>> {
    static LAST: OnceLock<Mutex<Option<RejectionEpisode>>> = OnceLock::new();
    LAST.get_or_init(|| Mutex::new(None))
}
#[cfg(not(test))]
fn clear_rejection(source: Source) {
    if let Ok(mut last) = rejection_episode().lock() {
        if last.as_ref().is_some_and(|(episode, _, _)| *episode == source) {
            *last = None;
        }
    }
}
#[cfg(not(test))]
fn record_rejection(
    source: Source,
    rejected: Rejected,
    field: &'static str,
    snapshots: usize,
    unknown: bool,
) {
    if let Ok(mut last) = rejection_episode().lock() {
        let episode = (source, rejected, field);
        let changed = *last != Some(episode);
        *last = Some(episode);
        if changed {
            // Diagnostics stay content-free: no titles, artists, URLs or sessions.
            crate::diagnostics::record(
                "media-connector-rejected",
                &format!(
                    "source={} reason={rejected:?} field={field} snapshots={snapshots} unknown={unknown}",
                    card_source(source)
                ),
            );
        }
    }
}
// A connector that reconnects in a loop would otherwise write a line every
// few seconds, so a repeated reason waits for this interval.
const CLOSE_LOG_AFTER_MS: u128 = 30_000;
/// The last logged close for one source: its reason, when it was logged, and
/// how many closes were suppressed since.
#[cfg(not(test))]
type CloseEpisode = (String, std::time::Instant, u32);
#[cfg(not(test))]
fn close_episode() -> &'static Mutex<HashMap<&'static str, CloseEpisode>> {
    static LAST: OnceLock<Mutex<HashMap<&'static str, CloseEpisode>>> = OnceLock::new();
    LAST.get_or_init(|| Mutex::new(HashMap::new()))
}
/// A close line repeats only when its reason changed or the interval passed.
fn should_log_close(last: &Option<(String, u128)>, reason: &str, elapsed_ms: u128) -> bool {
    match last {
        Some((previous, _)) if previous.as_str() == reason => elapsed_ms >= CLOSE_LOG_AFTER_MS,
        _ => true,
    }
}
/// Content-free by construction: a source name, a reason label and counters.
fn close_detail(
    source: &str,
    reason: &str,
    lifetime_ms: u128,
    messages: u32,
    max_gap_ms: u128,
    suppressed: u32,
) -> String {
    let mut detail = format!(
        "source={source} reason={reason} lifetime_ms={lifetime_ms} messages={messages} max_gap_ms={max_gap_ms}"
    );
    if suppressed > 0 {
        detail.push_str(&format!(" suppressed={suppressed}"));
    }
    detail
}
/// `crate::diagnostics` exists only in the app binary, so the lib test target
/// builds this line and drops it.
fn record_close(
    source: &'static str,
    reason: &'static str,
    lifetime_ms: u128,
    messages: u32,
    max_gap_ms: u128,
) {
    #[cfg(not(test))]
    {
        let Ok(mut episodes) = close_episode().lock() else {
            return;
        };
        let now = std::time::Instant::now();
        let last = episodes.get(source).map(|(reason, at, _)| {
            (
                reason.clone(),
                now.saturating_duration_since(*at).as_millis(),
            )
        });
        let elapsed_ms = last.as_ref().map_or(0, |(_, elapsed)| *elapsed);
        if !should_log_close(&last, reason, elapsed_ms) {
            if let Some((_, _, suppressed)) = episodes.get_mut(source) {
                *suppressed = suppressed.saturating_add(1);
            }
            return;
        }
        let suppressed = episodes
            .get(source)
            .map_or(0, |(_, _, suppressed)| *suppressed);
        // Diagnostics stay content-free: no titles, artists, URLs or sessions.
        crate::diagnostics::record(
            "media-connector-closed",
            &close_detail(source, reason, lifetime_ms, messages, max_gap_ms, suppressed),
        );
        episodes.insert(source, (reason.to_string(), now, 0));
    }
    #[cfg(test)]
    let _ = (source, reason, lifetime_ms, messages, max_gap_ms);
}
static REFRESH: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);
pub fn take_refresh() -> bool {
    REFRESH.swap(false, std::sync::atomic::Ordering::Relaxed)
}
pub fn now() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64
}
pub fn active() -> Option<Target> {
    shared().lock().ok()?.broker.active(now())
}
pub fn native_update(hint: NativeHint, generation: &str) {
    if let Ok(mut s) = shared().lock() {
        s.broker.native = hint;
        s.broker.native_generation = generation.into();
    }
}
pub fn native_allowed(generation: &str) -> bool {
    shared().lock().is_ok_and(|s| {
        // The primary keeps the old rule; every other Windows player is
        // addressable while its published card is in the list.
        (s.broker.native_generation == generation && s.broker.active(now()).is_none())
            || s.native_cards.iter().any(|c| c.generation == generation)
    })
}
/// The media thread owns the Windows cards; publishing them here lets the
/// command surface and the list builder see one consistent set.
pub fn set_native_cards(cards: Vec<MediaCard>) {
    if let Ok(mut s) = shared().lock() {
        s.native_cards = cards;
    }
}
/// Source families whose connector currently shows at least one playing card.
pub fn playing_sources(broker: &Broker, now: u64) -> Vec<Source> {
    let mut sources = Vec::new();
    for target in broker.targets(now) {
        if !sources.contains(&target.source) {
            sources.push(target.source);
        }
    }
    // A fresh tab list represents the YouTube family too: Brave exposes one
    // SMTC session for all its tabs, and a tab row already names that audio.
    if !sources.contains(&Source::Youtube)
        && broker.clients.values().any(|c| {
            c.source == Source::Youtube
                && !c.tabs.is_empty()
                && now.saturating_sub(c.received) < 6000
        })
    {
        sources.push(Source::Youtube);
    }
    sources
}
/// The shared broker's playing sources; the media thread owns no broker handle.
pub fn playing_sources_now(now: u64) -> Vec<Source> {
    shared()
        .lock()
        .map(|s| playing_sources(&s.broker, now))
        .unwrap_or_default()
}
/// A Windows session is already represented when its app family's connector is
/// showing a playing card for that source. A live client that shows no card
/// cannot own the session: Brave exposes one SMTC session for every tab, so a
/// non-YouTube tab is unknown to the YouTube connector and would otherwise
/// lose its native card. A family without a playing card needs its own card.
pub fn connector_represented(family: Option<Source>, playing: &[Source]) -> bool {
    family.is_some_and(|f| playing.contains(&f))
}

#[derive(Serialize)]
pub struct QueueView {
    generation: String,
    available: bool,
    source: String,
    revision: String,
    entries: Vec<Entry>,
    reason: Option<String>,
    total_upcoming: Option<u32>,
    pending: bool,
    needs_check: bool,
}
#[tauri::command]
pub fn media_queue_get(generation: String) -> Result<QueueView, String> {
    let mut state = shared().lock().map_err(|_| "Media unavailable.")?;
    state.reap(now());
    // Any card owns its own queue, not just the primary.
    let refusal = match state.broker.target_for(&generation, now()) {
        Some(target) => {
            // A target resolves again, so the next refusal is a new episode.
            #[cfg(not(test))]
            reset_refusal_episode();
            return Ok(QueueView {
                generation,
                available: target.snapshot.queue_available,
                source: match target.source {
                    Source::Spotify => "Spotify",
                    Source::Youtube => "YouTube · Brave",
                }
                .into(),
                revision: target.snapshot.revision,
                entries: target.snapshot.entries,
                reason: target.snapshot.reason,
                total_upcoming: target.snapshot.total_upcoming,
                pending: state
                    .pending
                    .values()
                    .any(|p| p.generation == target.generation),
                needs_check: state.uncertain.contains_key(&target.generation),
            });
        }
        None => match state.broker.resolve(now()) {
            // The primary still resolves, so this generation is stale.
            Ok(_) => return Err("Media changed. Check the queue again.".into()),
            Err(refusal) => refusal,
        },
    };
    let native = &state.broker.native;
    let reason = match native.source {
        Some(Source::Spotify) if !state.broker.clients.values().any(|c|c.source==Source::Spotify && now().saturating_sub(c.received)<6000)=>
            "Spotify connector disconnected. Open Spotify and enable Bloom’s Spicetify extension, then check the queue.",
        Some(Source::Youtube) if !state.broker.clients.values().any(|c|c.source==Source::Youtube && now().saturating_sub(c.received)<6000)=>
            "YouTube connector disconnected. Open a YouTube playlist in Brave and enable Bloom Media Connector, then check the queue.",
        // Windows names a player Bloom has no connector for, so no queue exists.
        _ if native.source.is_none() && native.has_media =>
            "Queue works with Spotify and YouTube in Brave. This player isn’t supported.",
        _ => match refusal {
            NoTarget::UnknownAudible => "Another tab is playing audio, so Bloom can’t tell which queue is yours. Pause it, then check again.",
            NoTarget::AmbiguousPlaying => "More than one item is playing in this player. Pause the extras, then check again.",
            NoTarget::NativeMismatch | NoTarget::NoSession => "Waiting for the player to report. Check again in a moment.",
        },
    };
    #[cfg(not(test))]
    {
        let source = match native.source {
            Some(Source::Spotify) => "Spotify",
            Some(Source::Youtube) => "YouTube",
            None => "none",
        };
        // The episode key is the refusal tuple, so a poll that changes nothing
        // logs nothing.
        let episode = format!("{refusal:?}|{source}|{}", native.has_media);
        // Freshness flaps every poll, so the counts inform the detail but stay
        // out of the episode key; only the refusal tuple may re-log.
        let at = now();
        let count = |wanted: Source, fresh_only: bool| {
            state
                .broker
                .clients
                .values()
                .filter(|c| c.source == wanted && (!fresh_only || at.saturating_sub(c.received) < 6000))
                .count()
        };
        let (spotify_fresh, youtube_fresh) = (count(Source::Spotify, true), count(Source::Youtube, true));
        let (spotify_total, youtube_total) = (count(Source::Spotify, false), count(Source::Youtube, false));
        if let Ok(mut last) = refused_episode().lock() {
            let changed = last.as_deref() != Some(episode.as_str());
            *last = Some(episode);
            if changed {
                // Diagnostics stay content-free: no titles, artists, URLs or sessions.
                crate::diagnostics::record(
                    "media-queue-refused",
                    &format!(
                        "reason={refusal:?} source={source} clients={} spotify={spotify_fresh}/{spotify_total} youtube={youtube_fresh}/{youtube_total}",
                        state.broker.clients.len()
                    ),
                );
            }
        }
    }
    Ok(QueueView {
        generation,
        available: false,
        source: String::new(),
        revision: String::new(),
        entries: vec![],
        reason: Some(reason.into()),
        total_upcoming: None,
        pending: false,
        needs_check: false,
    })
}
#[derive(Serialize, Clone, PartialEq)]
pub struct MediaCard {
    pub generation: String,
    pub source: String,
    /// The player's name for the card chip: "Spotify"/"YouTube" for a
    /// connector, the SMTC app's display name for a Windows player.
    pub app: String,
    pub title: String,
    pub artist: String,
    pub is_playing: bool,
    pub artwork: Option<Vec<String>>,
    pub position_ms: i64,
    pub duration_ms: i64,
    pub seek_enabled: bool,
    pub position_updated_at: u64,
    pub queue: bool,
    pub primary: bool,
    /// "player" or "tab": the two card families render differently.
    pub kind: String,
    pub muted: bool,
    /// A Windows card's SMTC family. Routing state for `build` only: the UI
    /// cannot use it, so it never crosses the wire.
    #[serde(skip)]
    pub family: Option<Source>,
}
#[derive(Serialize, Clone, PartialEq)]
pub struct MediaList {
    pub cards: Vec<MediaCard>,
    pub other_audio: bool,
}
fn card_source(source: Source) -> &'static str {
    match source {
        Source::Spotify => "Spotify",
        Source::Youtube => "YouTube",
    }
}
/// A player's name for the card chip, from its SMTC AppUserModelId. Known
/// players get their brand spelling; anything else falls back to its exe stem
/// or package prefix, so the chip never says "Windows".
pub fn app_display_name(aumid: &str) -> String {
    let id = aumid.trim();
    if id.is_empty() {
        return "Media".into();
    }
    // AUMIDs arrive as a bare exe name, a full path, or a package id whose app
    // suffix follows "!". The stem is what the brand table matches.
    let segment = id.rsplit(['\\', '/']).next().unwrap_or(id);
    let before_app = segment.split('!').next().unwrap_or(segment);
    let stem = before_app
        .len()
        .checked_sub(4)
        .filter(|&i| {
            before_app
                .get(i..)
                .is_some_and(|suffix| suffix.eq_ignore_ascii_case(".exe"))
        })
        .map_or(before_app, |i| &before_app[..i]);
    let lower = stem.to_ascii_lowercase();
    let known = if lower == "spotify" || lower.starts_with("spotifyab.spotify") {
        Some("Spotify")
    } else if lower == "brave" || lower.starts_with("brave.") {
        Some("Brave")
    } else if lower.starts_with("msedge") || lower.starts_with("microsoft.microsoftedge") {
        Some("Edge")
    } else if lower.starts_with("chrome") {
        Some("Chrome")
    } else if lower.starts_with("firefox") || lower == "308046b0af4a39cb" {
        // Firefox's AUMID is a hash on some installs, not a name.
        Some("Firefox")
    } else if lower.starts_with("vlc") {
        Some("VLC")
    } else {
        None
    };
    match known {
        Some(name) => name.into(),
        // Only the first letter changes, so "SomeApp" keeps its inner casing.
        None => match stem.chars().next() {
            Some(first) => first.to_uppercase().chain(stem.chars().skip(1)).collect(),
            None => "Media".into(),
        },
    }
}
/// The one Spotify-family SMTC session, if exactly one exists: the Spotify
/// connector card may then show that player's cover. Matching is by app family
/// only; a cover is never associated with a card by track text.
pub fn spotify_art_session(families: &[Option<Source>]) -> Option<usize> {
    let mut spotify = families
        .iter()
        .enumerate()
        .filter(|(_, family)| **family == Some(Source::Spotify))
        .map(|(index, _)| index);
    match (spotify.next(), spotify.next()) {
        (Some(index), None) => Some(index),
        _ => None,
    }
}
fn build(broker: &Broker, native: &[MediaCard], now: u64) -> MediaList {
    let primary = broker.resolve(now).ok().map(|t| t.generation);
    let mut cards: Vec<MediaCard> = broker
        .targets(now)
        .into_iter()
        .map(|target| MediaCard {
            primary: primary.as_ref() == Some(&target.generation),
            source: card_source(target.source).into(),
            app: card_source(target.source).into(),
            generation: target.generation,
            title: target.snapshot.title,
            artist: target.snapshot.artist,
            is_playing: target.snapshot.is_playing,
            // Artwork stays primary-only; services.rs fills that one card.
            artwork: None,
            position_ms: target.snapshot.position_ms,
            duration_ms: target.snapshot.duration_ms,
            seek_enabled: target.snapshot.seek_enabled,
            position_updated_at: now,
            queue: true,
            kind: "player".into(),
            muted: false,
            family: None,
        })
        .collect();
    cards.extend(tab_cards(broker, now));
    // Native cards follow the connectors, ordered by their stable generation
    // so a map rebuilt every poll cannot shuffle them.
    let mut native = native.to_vec();
    native.sort_by(|a, b| a.generation.cmp(&b.generation));
    // Brave exposes one SMTC session for all its tabs, so a Brave-family
    // Windows card already shows the browser audio the YouTube connector could
    // not attribute. "Other audio playing" would name that audio twice.
    let brave_native = native
        .iter()
        .any(|card| card.family == Some(Source::Youtube));
    cards.extend(native);
    MediaList {
        cards,
        other_audio: broker.other_audio(now) && !brave_native,
    }
}
/// One card per tab the YouTube connector listed, ordered by window and then
/// id so the shelf keeps its place between polls. A tab's player is inside a
/// cross-origin iframe, so the card has no queue, no scrubber and no transport.
fn tab_cards(broker: &Broker, now: u64) -> Vec<MediaCard> {
    let mut tabs: Vec<_> = broker
        .clients
        .iter()
        .filter(|(_, c)| c.source == Source::Youtube && now.saturating_sub(c.received) < 6000)
        .flat_map(|(client, c)| c.tabs.iter().map(move |tab| (client, tab)))
        .collect();
    tabs.sort_by_key(|(client, tab)| (tab.window, tab.id, client.as_str()));
    tabs.into_iter()
        .map(|(client, tab)| MediaCard {
            generation: format!("{client}:tab:{}", tab.id),
            source: "Tab".into(),
            app: "Brave".into(),
            title: tab.title.clone(),
            artist: tab.site.clone(),
            is_playing: true,
            artwork: None,
            position_ms: 0,
            duration_ms: 0,
            seek_enabled: false,
            position_updated_at: now,
            queue: false,
            primary: false,
            kind: "tab".into(),
            muted: tab.muted,
            family: None,
        })
        .collect()
}
/// Change-only and content-free: the detail is the episode key, so an
/// unchanged list is silent. Counts and one bool only, never a title, site or
/// id. `crate::diagnostics` exists only in the app binary.
#[cfg(not(test))]
fn last_list_summary() -> &'static Mutex<Option<String>> {
    static LAST: OnceLock<Mutex<Option<String>>> = OnceLock::new();
    LAST.get_or_init(|| Mutex::new(None))
}
#[cfg(not(test))]
fn record_list_summary(list: &MediaList) {
    let count = |kind: &str, source: &str| {
        list.cards
            .iter()
            .filter(|c| c.kind == kind && c.source == source)
            .count()
    };
    let detail = format!(
        "players={}/{} tabs={} windows={} other_audio={}",
        count("player", "Spotify"),
        count("player", "YouTube"),
        list.cards.iter().filter(|c| c.kind == "tab").count(),
        list.cards.iter().filter(|c| c.source == "Windows").count(),
        list.other_audio
    );
    if let Ok(mut last) = last_list_summary().lock() {
        if last.as_deref() != Some(detail.as_str()) {
            *last = Some(detail.clone());
            crate::diagnostics::record("media-list-summary", &detail);
        }
    }
}
pub fn media_list(now: u64) -> MediaList {
    match shared().lock() {
        Ok(state) => {
            let list = build(&state.broker, &state.native_cards, now);
            #[cfg(not(test))]
            record_list_summary(&list);
            list
        }
        Err(_) => MediaList {
            cards: vec![],
            other_audio: false,
        },
    }
}
#[tauri::command]
pub fn media_list_get() -> MediaList {
    media_list(now())
}
pub fn target_for_generation(generation: &str) -> Option<Target> {
    shared()
        .lock()
        .ok()
        .and_then(|state| state.broker.target_for(generation, now()))
}
/// A fresh tab the generation names, so the command surface can tell a tab
/// address from a player's before it chooses a transport.
pub fn tab_for_generation(generation: &str) -> Option<TabInfo> {
    shared()
        .lock()
        .ok()
        .and_then(|state| state.broker.tab_for(generation, now()))
        .map(|(_, tab)| tab)
}
/// Position advances every poll, so compare it with the same 1 s tolerance
/// `media-update` uses. `position_updated_at` is the poll time and must never
/// count as a change, or every poll would re-emit the list.
pub fn same_list(a: &MediaList, b: &MediaList) -> bool {
    a.other_audio == b.other_audio
        && a.cards.len() == b.cards.len()
        && a.cards.iter().zip(&b.cards).all(|(x, y)| {
            x.generation == y.generation
                && x.source == y.source
                && x.app == y.app
                && x.title == y.title
                && x.artist == y.artist
                && x.is_playing == y.is_playing
                && x.artwork == y.artwork
                && x.duration_ms == y.duration_ms
                && x.seek_enabled == y.seek_enabled
                && x.queue == y.queue
                && x.primary == y.primary
                && x.kind == y.kind
                && x.muted == y.muted
                && (x.position_ms - y.position_ms).abs() <= 1000
        })
}
async fn request(
    generation: &str,
    op: &str,
    revision: Option<&str>,
    entry: Option<&str>,
    position: Option<i64>,
) -> Result<String, String> {
    let reply = {
        let mut state = shared().lock().map_err(|_| "Media unavailable.")?;
        state.reap(now());
        state.prepare(generation, op, revision, entry, position, now())?
    };
    match tokio::time::timeout(Duration::from_secs(5), reply).await {
        Ok(Ok(result)) => Ok(result),
        _ => {
            if let Ok(mut state) = shared().lock() {
                state.reap(now().saturating_add(1));
            }
            Ok("uncertain".into())
        }
    }
}
#[tauri::command]
pub async fn media_queue_select(
    generation: String,
    revision: String,
    entry_id: String,
) -> Result<String, String> {
    request(
        &generation,
        "select",
        Some(&revision),
        Some(&entry_id),
        None,
    )
    .await
}
pub async fn connector_action(
    generation: &str,
    op: &str,
    position: Option<i64>,
) -> Result<String, String> {
    request(generation, op, None, None, position).await
}

#[derive(Deserialize)]
#[serde(tag = "op", rename_all = "snake_case")]
enum Incoming {
    Inventory {
        snapshots: Vec<Snapshot>,
        /// Audible browser tabs. Only the YouTube connector may send them.
        #[serde(default)]
        tabs: Vec<TabInfo>,
        #[serde(default)]
        unknown_playing: bool,
        /// Opt-in: the client asks the server to drive its publish cadence,
        /// because Chromium throttles the client's own timer chain.
        #[serde(default)]
        pull: bool,
    },
    Result {
        id: String,
        status: String,
        #[serde(default)]
        snapshot: Option<Snapshot>,
    },
}

/// Why a connector socket's loop stopped. Every arm breaks exactly where the
/// loop broke before; only the diagnostics line is new.
#[derive(Clone, Copy)]
enum CloseReason {
    ClientClosed,
    StreamEnded,
    /// The read failed; the payload names the error family only, because
    /// tungstenite's Display text can echo frame contents.
    ReadError(&'static str),
    /// The frame did not parse as `Incoming`; the payload is serde's category.
    BadJson(&'static str),
    Invalid,
    Idle,
    SendFailed,
    CommandChannelClosed,
    /// The shared state mutex was poisoned.
    StateLockFailed,
}
impl CloseReason {
    fn label(self) -> &'static str {
        match self {
            CloseReason::ClientClosed => "ClientClosed",
            CloseReason::StreamEnded => "StreamEnded",
            CloseReason::ReadError(kind) => match kind {
                "message_too_big" => "ReadError:message_too_big",
                "protocol" => "ReadError:protocol",
                "closed" => "ReadError:closed",
                "io" => "ReadError:io",
                _ => "ReadError:other",
            },
            CloseReason::BadJson(kind) => match kind {
                "syntax" => "BadJson:syntax",
                "data" => "BadJson:data",
                "eof" => "BadJson:eof",
                _ => "BadJson:io",
            },
            CloseReason::Invalid => "Invalid",
            CloseReason::Idle => "Idle",
            CloseReason::SendFailed => "SendFailed",
            CloseReason::CommandChannelClosed => "CommandChannelClosed",
            CloseReason::StateLockFailed => "StateLockFailed",
        }
    }
}
fn read_error_kind(error: &tokio_tungstenite::tungstenite::Error) -> &'static str {
    use tokio_tungstenite::tungstenite::Error;
    match error {
        Error::Capacity(_) => "message_too_big",
        Error::Protocol(_) | Error::Utf8(_) | Error::AttackAttempt => "protocol",
        Error::ConnectionClosed | Error::AlreadyClosed => "closed",
        Error::Io(_) => "io",
        _ => "other",
    }
}
fn json_error_kind(error: &serde_json::Error) -> &'static str {
    match error.classify() {
        serde_json::error::Category::Syntax => "syntax",
        serde_json::error::Category::Data => "data",
        serde_json::error::Category::Eof => "eof",
        serde_json::error::Category::Io => "io",
    }
}
/// Only an opted-in client is polled, and never faster than once per 2000 ms.
fn poll_due(pull: bool, since_last_poll_ms: u128) -> bool {
    pull && since_last_poll_ms >= 2000
}

async fn socket(
    stream: tokio::net::TcpStream,
    settings: Settings,
    state: Arc<Mutex<State>>,
    notify: Arc<dyn Fn() + Send + Sync>,
) {
    let mut source = None;
    let callback =
        |request: &Request,
         mut response: tokio_tungstenite::tungstenite::handshake::server::Response| {
            source = authenticate(request, &settings);
            if source.is_none() {
                return Err(tokio_tungstenite::tungstenite::http::Response::builder()
                    .status(401)
                    .body(None)
                    .unwrap());
            }
            response
                .headers_mut()
                .insert("Sec-WebSocket-Protocol", "bloom.v1".parse().unwrap());
            Ok(response)
        };
    // Tungstenite 0.30 also bounds pre-authentication handshake bytes (64 KiB)
    // and packets. An explicit deadline prevents idle handshakes occupying slots.
    let config = WebSocketConfig::default()
        .read_buffer_size(4096)
        .write_buffer_size(4096)
        .max_write_buffer_size(2097152)
        .max_message_size(Some(1048576))
        .max_frame_size(Some(1048576));
    let Ok(Ok(mut socket)) = tokio::time::timeout(
        Duration::from_secs(2),
        accept_hdr_async_with_config(stream, callback, Some(config)),
    )
    .await
    else {
        return;
    };
    let Some(source) = source else {
        return;
    };
    let client = uuid::Uuid::new_v4().simple().to_string();
    let (tx, mut rx) = mpsc::channel::<Value>(16);
    {
        let Ok(mut state) = state.lock() else {
            return;
        };
        if state.senders.len() >= 4 {
            return;
        }
        state.senders.insert(client.clone(), tx);
    }
    let connected_at = std::time::Instant::now();
    let mut last_message = connected_at;
    // Accepted messages drive the gap, so an idle close shows how long the
    // player had been quiet before the socket went.
    let mut last_accepted = connected_at;
    let mut messages: u32 = 0;
    let mut max_gap_ms: u128 = 0;
    // An opted-in client is polled from the tick; the connect instant starts
    // the first interval so a poll never races the handshake.
    let mut pull = false;
    let mut last_poll = connected_at;
    let mut tick = tokio::time::interval(Duration::from_millis(250));
    let reason = loop {
        tokio::select! {
            incoming=socket.next()=>{
                let message=match incoming {
                    Some(Ok(message))=>message,
                    Some(Err(error))=>break CloseReason::ReadError(read_error_kind(&error)),
                    None=>break CloseReason::StreamEnded,
                };
                if message.is_close(){break CloseReason::ClientClosed;}
                if let Message::Text(text)=message {
                    let message=match serde_json::from_str::<Incoming>(&text) {
                        Ok(message)=>message,
                        Err(error)=>break CloseReason::BadJson(json_error_kind(&error)),
                    };
                    let Ok(mut state)=state.lock()else{break CloseReason::StateLockFailed;};
                    let mut wants_pull=false;
                    let valid=match message {
                        Incoming::Inventory{snapshots,tabs,unknown_playing,pull}=>{wants_pull=pull;state.inventory(&client,source,snapshots,tabs,unknown_playing,now())},
                        Incoming::Result{id,status,snapshot}=>{if id.len()>128 || status.len()>32{false}else{state.finish(&client,&id,&status,snapshot,now());true}},
                    };
                    if !valid{break CloseReason::Invalid;}
                    // The opt-in latches only once the inventory was accepted.
                    if wants_pull{pull=true;}
                    let accepted=std::time::Instant::now();
                    max_gap_ms=max_gap_ms.max(accepted.saturating_duration_since(last_accepted).as_millis());
                    last_accepted=accepted;last_message=accepted;messages=messages.saturating_add(1);
                    drop(state);notify();
                }
            },
            command=rx.recv()=>{
                let Some(command)=command else{break CloseReason::CommandChannelClosed;};
                if !matches!(tokio::time::timeout(Duration::from_secs(2),socket.send(Message::Text(command.to_string().into()))).await,Ok(Ok(()))){break CloseReason::SendFailed;}
            },
            _=tick.tick()=>{
                if last_message.elapsed()>Duration::from_secs(8){break CloseReason::Idle;}
                // The idle rule and reap keep running whether or not a poll is due.
                if poll_due(pull,last_poll.elapsed().as_millis()){
                    if !matches!(tokio::time::timeout(Duration::from_secs(2),socket.send(Message::Text(r#"{"op":"poll"}"#.into()))).await,Ok(Ok(()))){break CloseReason::SendFailed;}
                    last_poll=std::time::Instant::now();
                }
                if let Ok(mut state)=state.lock(){state.reap(now());}
            }
        }
    };
    record_close(
        card_source(source),
        reason.label(),
        connected_at.elapsed().as_millis(),
        messages,
        max_gap_ms,
    );
    if let Ok(mut state) = state.lock() {
        state.disconnect(&client);
    }
    notify();
}

pub fn start(app: &tauri::AppHandle) -> Result<(), String> {
    use std::io::Read;
    let directory = app
        .path()
        .app_local_data_dir()
        .map_err(|_| "Media connector folder unavailable.")?;
    std::fs::create_dir_all(&directory).map_err(|_| "Media connector folder unavailable.")?;
    let path = directory.join("media-connectors.json");
    let existing = path.exists();
    let mut settings = if existing {
        let mut text = String::new();
        std::fs::File::open(&path)
            .map_err(|_| "Media connector configuration unavailable.")?
            .take(4097)
            .read_to_string(&mut text)
            .map_err(|_| "Media connector configuration unavailable.")?;
        if text.len() > 4096 {
            return Err("Media connector configuration is invalid.".into());
        }
        serde_json::from_str::<Settings>(&text)
            .map_err(|_| "Media connector configuration is invalid.")?
    } else {
        Settings {
            port: 0,
            spotify: uuid::Uuid::new_v4().simple().to_string(),
            youtube: uuid::Uuid::new_v4().simple().to_string(),
        }
    };
    if settings.spotify.len() != 32
        || settings.youtube.len() != 32
        || settings.spotify == settings.youtube
    {
        return Err("Media connector configuration is invalid.".into());
    }
    let listener = std::net::TcpListener::bind((std::net::Ipv4Addr::LOCALHOST, settings.port))
        .map_err(|_| "Media connector port unavailable.")?;
    settings.port = listener
        .local_addr()
        .map_err(|_| "Media connector address unavailable.")?
        .port();
    listener
        .set_nonblocking(true)
        .map_err(|_| "Media connector listener unavailable.")?;
    if !existing {
        let temp = directory.join("media-connectors.json.tmp");
        std::fs::write(
            &temp,
            serde_json::to_vec(&settings)
                .map_err(|_| "Media connector configuration unavailable.")?,
        )
        .map_err(|_| "Media connector configuration unavailable.")?;
        std::fs::rename(temp, path).map_err(|_| "Media connector configuration unavailable.")?;
    }
    let handle = app.clone();
    let notify: Arc<dyn Fn() + Send + Sync> = Arc::new(move || {
        REFRESH.store(true, std::sync::atomic::Ordering::Relaxed);
        let _ = handle.emit("media-queue-update", ());
    });
    let state = Arc::clone(shared());
    tauri::async_runtime::spawn(async move {
        let Ok(listener) = tokio::net::TcpListener::from_std(listener) else {
            return;
        };
        let slots = Arc::new(tokio::sync::Semaphore::new(4));
        while let Ok((stream, _)) = listener.accept().await {
            let Ok(slot) = Arc::clone(&slots).try_acquire_owned() else {
                drop(stream);
                continue;
            };
            let (settings, state, notify) =
                (settings.clone(), Arc::clone(&state), Arc::clone(&notify));
            tauri::async_runtime::spawn(async move {
                let _slot = slot;
                socket(stream, settings, state, notify).await;
            });
        }
    });
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn a_result_after_the_deadline_is_uncertain_even_before_the_timer_tick() {
        let (mut s, mut rx) = state();
        let mut reply = s
            .prepare(
                "conn:tab/document/player",
                "select",
                Some("1"),
                Some("1:BBBBBBBBBBB"),
                None,
                101,
            )
            .unwrap();
        let cmd = rx.try_recv().unwrap();
        let mut observed = s.broker.clients["conn"].inventory[0].clone();
        observed.current_id = "1:BBBBBBBBBBB".into();
        observed.observed_ms = 5102;
        s.finish(
            "conn",
            cmd["id"].as_str().unwrap(),
            "confirmed",
            Some(observed),
            5102,
        );
        assert_eq!(reply.try_recv().unwrap(), "uncertain");
    }
    #[test]
    fn uncertainty_does_not_retain_sessions_removed_from_a_connected_source() {
        let (mut s, _rx) = state();
        s.uncertain.insert("conn:removed-session".into(), 100);
        let mut snapshot = s.broker.clients["conn"].inventory[0].clone();
        snapshot.observed_ms = 102;
        assert!(s.inventory(
            "conn",
            Source::Youtube,
            vec![snapshot],
            vec![],
            false,
            102
        ));
        assert!(s.uncertain.is_empty());
    }
    #[test]
    fn handshake_requires_source_secret_origin_and_loopback_host() {
        let settings = Settings {
            port: 39641,
            spotify: "s".repeat(32),
            youtube: "y".repeat(32),
        };
        let request = |path: &str, host: &str, origin: &str, secret: &str| {
            Request::builder()
                .uri(path)
                .header("Host", host)
                .header("Origin", origin)
                .header("Sec-WebSocket-Protocol", format!("bloom.v1, {secret}"))
                .body(())
                .unwrap()
        };
        assert_eq!(
            authenticate(
                &request(
                    "/spotify",
                    "127.0.0.1:39641",
                    "https://xpui.app.spotify.com",
                    &settings.spotify
                ),
                &settings
            ),
            Some(Source::Spotify)
        );
        assert!(authenticate(
            &request(
                "/spotify",
                "evil.example",
                "https://xpui.app.spotify.com",
                &settings.spotify
            ),
            &settings
        )
        .is_none());
        assert!(authenticate(
            &request(
                "/spotify",
                "127.0.0.1:39641",
                "https://www.youtube.com",
                &settings.spotify
            ),
            &settings
        )
        .is_none());
        assert!(authenticate(
            &request(
                "/youtube",
                "127.0.0.1:39641",
                "https://xpui.app.spotify.com",
                &settings.youtube
            ),
            &settings
        )
        .is_none());
        assert_eq!(
            authenticate(
                &request(
                    "/youtube",
                    "127.0.0.1:39641",
                    &format!("chrome-extension://{}", "a".repeat(32)),
                    &settings.youtube
                ),
                &settings
            ),
            Some(Source::Youtube)
        );
        assert!(authenticate(
            &request(
                "/youtube",
                "127.0.0.1:39641",
                &format!("chrome-extension://{}", "a".repeat(32)),
                &settings.spotify
            ),
            &settings
        )
        .is_none());
    }
    fn state() -> (State, mpsc::Receiver<Value>) {
        let mut state = State::default();
        let (tx, rx) = mpsc::channel(16);
        let snapshot = Snapshot {
            session: "tab/document/player".into(),
            revision: "1".into(),
            current_id: "0:AAAAAAAAAAA".into(),
            title: "Current".into(),
            is_playing: true,
            queue_available: true,
            entries: vec![Entry {
                id: "1:BBBBBBBBBBB".into(),
                title: "Next".into(),
                index: Some(1),
                video_id: Some("BBBBBBBBBBB".into()),
                ..Default::default()
            }],
            ..Default::default()
        };
        state
            .broker
            .update("conn", Source::Youtube, vec![snapshot], false, 100);
        state.senders.insert("conn".into(), tx);
        (state, rx)
    }
    #[test]
    fn dispatch_uses_stored_entry_and_excludes_conflicting_clicks() {
        let (mut state, mut rx) = state();
        let _reply = state
            .prepare(
                "conn:tab/document/player",
                "select",
                Some("1"),
                Some("1:BBBBBBBBBBB"),
                None,
                101,
            )
            .unwrap();
        let cmd = rx.try_recv().unwrap();
        assert_eq!(cmd["entry"]["video_id"], "BBBBBBBBBBB");
        assert_eq!(cmd["session"], "tab/document/player");
        assert!(state
            .prepare("conn:tab/document/player", "next", None, None, None, 102)
            .is_err());
        assert!(rx.try_recv().is_err());
    }
    #[test]
    fn rejects_invalid_actions_and_stale_rows_without_dispatch() {
        let (mut state, mut rx) = state();
        assert!(state
            .prepare("conn:tab/document/player", "shell", None, None, None, 101)
            .is_err());
        assert!(state
            .prepare(
                "conn:tab/document/player",
                "select",
                Some("0"),
                Some("1:BBBBBBBBBBB"),
                None,
                101
            )
            .is_err());
        assert!(rx.try_recv().is_err());
    }
    #[test]
    fn confirmed_selection_requires_observed_occurrence_playing() {
        let (mut s, mut rx) = state();
        let mut reply = s
            .prepare(
                "conn:tab/document/player",
                "select",
                Some("1"),
                Some("1:BBBBBBBBBBB"),
                None,
                101,
            )
            .unwrap();
        let cmd = rx.try_recv().unwrap();
        let wrong = s.broker.clients["conn"].inventory[0].clone();
        s.finish(
            "conn",
            cmd["id"].as_str().unwrap(),
            "confirmed",
            Some(wrong),
            102,
        );
        assert_eq!(reply.try_recv().unwrap(), "uncertain");
    }
    #[test]
    fn timeout_is_uncertain_and_does_not_dispatch_a_retry() {
        let (mut s, mut rx) = state();
        let mut reply = s
            .prepare(
                "conn:tab/document/player",
                "select",
                Some("1"),
                Some("1:BBBBBBBBBBB"),
                None,
                101,
            )
            .unwrap();
        rx.try_recv().unwrap();
        s.reap(5102);
        assert_eq!(reply.try_recv().unwrap(), "uncertain");
        assert!(s
            .prepare("conn:tab/document/player", "next", None, None, None, 5103)
            .is_err());
        assert!(rx.try_recv().is_err());
    }
    #[test]
    fn unknown_or_late_results_cannot_replace_the_current_snapshot() {
        let (mut s, _rx) = state();
        let mut old = s.broker.clients["conn"].inventory[0].clone();
        old.title = "Old result".into();
        s.finish("conn", "unknown", "confirmed", Some(old), 102);
        assert_eq!(s.broker.clients["conn"].inventory[0].title, "Current");
    }
    #[test]
    fn real_loopback_socket_routes_an_occurrence_and_cleans_up_on_disconnect() {
        tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap().block_on(async{
            use tokio_tungstenite::tungstenite::client::IntoClientRequest;
            let listener=tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
            let port=listener.local_addr().unwrap().port();let config=Settings{port,spotify:"s".repeat(32),youtube:"y".repeat(32)};
            let state=Arc::new(Mutex::new(State::default()));let event=Arc::new(tokio::sync::Notify::new());
            let worker={let state=Arc::clone(&state);let event=Arc::clone(&event);let config=config.clone();tokio::spawn(async move{
                let(stream,_)=listener.accept().await.unwrap();socket(stream,config,state,Arc::new(move||event.notify_one())).await;
            })};
            let mut request=format!("ws://127.0.0.1:{port}/spotify").into_client_request().unwrap();
            request.headers_mut().insert("Origin","https://xpui.app.spotify.com".parse().unwrap());
            request.headers_mut().insert("Sec-WebSocket-Protocol",format!("bloom.v1, {}",config.spotify).parse().unwrap());
            let stream=tokio::net::TcpStream::connect((std::net::Ipv4Addr::LOCALHOST,port)).await.unwrap();
            let(mut client,_)=tokio_tungstenite::client_async(request,stream).await.unwrap();
            let mut snapshot=Snapshot{session:"spotify-instance".into(),revision:"r1".into(),title:"Before".into(),current_id:"old".into(),is_playing:true,queue_available:true,observed_ms:now(),
                entries:vec![Entry{id:"q2".into(),title:"Target".into(),uid:Some("q2".into()),uri:Some("spotify:track:a".into()),..Default::default()}],..Default::default()};
            client.send(Message::Text(json!({"op":"inventory","snapshots":[snapshot]}).to_string().into())).await.unwrap();
            tokio::time::timeout(Duration::from_secs(2),event.notified()).await.unwrap();
            let generation=state.lock().unwrap().broker.active(now()).unwrap().generation;
            let reply=state.lock().unwrap().prepare(&generation,"select",Some("r1"),Some("q2"),None,now()).unwrap();
            let command=tokio::time::timeout(Duration::from_secs(2),client.next()).await.unwrap().unwrap().unwrap();
            let command:Value=serde_json::from_str(command.to_text().unwrap()).unwrap();assert_eq!(command["entry"]["uid"],"q2");
            snapshot.current_id="q2".into();snapshot.title="Target".into();snapshot.entries.clear();snapshot.revision="r2".into();snapshot.observed_ms=now();
            client.send(Message::Text(json!({"op":"result","id":command["id"],"status":"confirmed","snapshot":snapshot}).to_string().into())).await.unwrap();
            assert_eq!(tokio::time::timeout(Duration::from_secs(2),reply).await.unwrap().unwrap(),"confirmed");
            client.close(None).await.unwrap();tokio::time::timeout(Duration::from_secs(2),worker).await.unwrap().unwrap();
            assert!(state.lock().unwrap().broker.clients.is_empty());assert!(state.lock().unwrap().pending.is_empty());
        });
    }
    async fn bridge() -> (Settings, Arc<Mutex<State>>, tokio::task::JoinHandle<()>) {
        let listener=tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port=listener.local_addr().unwrap().port();
        let config=Settings{port,spotify:"s".repeat(32),youtube:"y".repeat(32)};
        let state=Arc::new(Mutex::new(State::default()));let event=Arc::new(tokio::sync::Notify::new());
        let worker={let state=Arc::clone(&state);let event=Arc::clone(&event);let config=config.clone();tokio::spawn(async move{
            let(stream,_)=listener.accept().await.unwrap();socket(stream,config,state,Arc::new(move||event.notify_one())).await;
        })};
        (config,state,worker)
    }
    async fn bridge_client(config: &Settings) -> tokio_tungstenite::WebSocketStream<tokio::net::TcpStream> {
        use tokio_tungstenite::tungstenite::client::IntoClientRequest;
        let port=config.port;
        let mut request=format!("ws://127.0.0.1:{port}/spotify").into_client_request().unwrap();
        request.headers_mut().insert("Origin","https://xpui.app.spotify.com".parse().unwrap());
        request.headers_mut().insert("Sec-WebSocket-Protocol",format!("bloom.v1, {}",config.spotify).parse().unwrap());
        let stream=tokio::net::TcpStream::connect((std::net::Ipv4Addr::LOCALHOST,port)).await.unwrap();
        tokio_tungstenite::client_async(request,stream).await.unwrap().0
    }
    fn inventory(pull: Option<bool>) -> String {
        let snapshot=Snapshot{session:"spotify-instance".into(),revision:"r1".into(),title:"Song".into(),current_id:"old".into(),is_playing:true,queue_available:true,observed_ms:now(),..Default::default()};
        let mut message=json!({"op":"inventory","snapshots":[snapshot]});
        if let Some(pull)=pull{message["pull"]=json!(pull);}
        message.to_string()
    }
    async fn next_poll(client: &mut tokio_tungstenite::WebSocketStream<tokio::net::TcpStream>) -> Value {
        let message=tokio::time::timeout(Duration::from_secs(3),client.next()).await.unwrap().unwrap().unwrap();
        serde_json::from_str(message.to_text().unwrap()).unwrap()
    }
    #[test]
    fn an_opted_in_client_is_polled_about_every_two_seconds() {
        tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap().block_on(async{
            let (config,state,worker)=bridge().await;
            let mut client=bridge_client(&config).await;
            client.send(Message::Text(inventory(Some(true)).into())).await.unwrap();
            assert_eq!(next_poll(&mut client).await,json!({"op":"poll"}));
            let polled=std::time::Instant::now();
            // The reply to a poll is another inventory; the next poll follows it.
            client.send(Message::Text(inventory(Some(true)).into())).await.unwrap();
            assert_eq!(next_poll(&mut client).await,json!({"op":"poll"}));
            assert!(polled.elapsed()>=Duration::from_millis(1900));
            assert_eq!(state.lock().unwrap().broker.clients.len(),1);
            client.close(None).await.unwrap();tokio::time::timeout(Duration::from_secs(2),worker).await.unwrap().unwrap();
        });
    }
    #[test]
    fn a_client_that_never_opts_in_receives_no_poll_and_stays_connected() {
        tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap().block_on(async{
            let (config,state,worker)=bridge().await;
            let mut client=bridge_client(&config).await;
            client.send(Message::Text(inventory(None).into())).await.unwrap();
            assert!(tokio::time::timeout(Duration::from_secs(3),client.next()).await.is_err());
            assert_eq!(state.lock().unwrap().broker.clients.len(),1);
            client.close(None).await.unwrap();tokio::time::timeout(Duration::from_secs(2),worker).await.unwrap().unwrap();
        });
    }
    #[test]
    fn polling_needs_the_opt_in_and_two_seconds_since_the_last_poll() {
        assert!(!poll_due(false,0));
        assert!(!poll_due(false,10_000));
        assert!(!poll_due(true,0));
        assert!(!poll_due(true,1999));
        assert!(poll_due(true,2000));
        assert!(poll_due(true,2001));
    }
    #[test]
    fn card_list_marks_the_resolved_primary() {
        let (state, _rx) = state();
        let list = build(&state.broker, &[], 101);
        assert_eq!(list.cards.len(), 1);
        assert_eq!(list.cards[0].generation, "conn:tab/document/player");
        assert_eq!(list.cards[0].source, "YouTube");
        assert!(list.cards[0].primary && list.cards[0].queue);
        assert!(list.cards[0].artwork.is_none() && !list.other_audio);
    }
    fn native(generation: &str) -> MediaCard {
        MediaCard {
            generation: generation.into(),
            source: "Windows".into(),
            app: "Edge".into(),
            title: "Native".into(),
            artist: "Artist".into(),
            is_playing: true,
            artwork: None,
            position_ms: 1,
            duration_ms: 2,
            seek_enabled: true,
            position_updated_at: 1,
            queue: false,
            primary: false,
            kind: "player".into(),
            muted: false,
            family: None,
        }
    }
    #[test]
    fn native_cards_follow_the_connector_cards_in_generation_order() {
        let (state, _rx) = state();
        let mut first = native("a");
        first.artwork = Some(vec!["data:image/png;base64,AA==".into()]);
        let list = build(&state.broker, &[first, native("b")], 101);
        assert_eq!(list.cards.len(), 3);
        assert_eq!(list.cards[0].generation, "conn:tab/document/player");
        assert_eq!(list.cards[1].generation, "a");
        assert_eq!(list.cards[2].generation, "b");
        assert_eq!(list.cards[1].source, "Windows");
        assert!(!list.cards[1].queue && list.cards[1].is_playing);
        assert_eq!(
            list.cards[1].artwork,
            Some(vec!["data:image/png;base64,AA==".into()])
        );
        // A published order the thread did not sort must not move cards.
        let reversed = build(&state.broker, &[native("b"), native("a")], 101);
        assert_eq!(reversed.cards[1].generation, "a");
        assert_eq!(reversed.cards[2].generation, "b");
    }
    #[test]
    fn published_native_generations_are_addressable_and_unknown_ones_are_not() {
        native_update(NativeHint::default(), "primary-generation");
        set_native_cards(vec![]);
        assert!(playing_sources_now(now()).is_empty());
        assert!(native_allowed("primary-generation"));
        assert!(!native_allowed("unknown"));
        set_native_cards(vec![native("card-generation")]);
        assert!(native_allowed("card-generation"));
        assert!(native_allowed("primary-generation"));
        assert!(!native_allowed("unknown"));
        set_native_cards(vec![]);
    }
    #[test]
    fn only_a_playing_connector_family_represents_a_windows_session() {
        // A playing connector card for the family's source owns the session.
        assert!(connector_represented(
            Some(Source::Spotify),
            &[Source::Spotify]
        ));
        assert!(connector_represented(
            Some(Source::Youtube),
            &[Source::Youtube]
        ));
        // A stale connector shows no playing card, so the session stays native.
        assert!(!connector_represented(Some(Source::Spotify), &[]));
        assert!(!connector_represented(Some(Source::Youtube), &[]));
        // A live client with no card of its own (a non-YouTube Brave tab) is not
        // a playing target either, so the other family's session is native.
        assert!(!connector_represented(
            Some(Source::Youtube),
            &[Source::Spotify]
        ));
        assert!(!connector_represented(
            Some(Source::Spotify),
            &[Source::Youtube]
        ));
        // No connector family can own Edge, Chrome or VLC sessions.
        assert!(!connector_represented(
            None,
            &[Source::Spotify, Source::Youtube]
        ));
    }
    #[test]
    fn a_fresh_connector_without_a_playing_card_does_not_represent_its_family() {
        let mut broker = Broker::default();
        let spotify = Snapshot {
            session: "spotify:track".into(),
            revision: "1".into(),
            current_id: "0:AAAAAAAAAAA".into(),
            title: "Song".into(),
            is_playing: true,
            observed_ms: 100,
            ..Default::default()
        };
        assert!(broker.update("sp", Source::Spotify, vec![spotify], false, 100));
        // The YouTube connector is fresh but heard a tab it cannot attribute.
        assert!(broker.update("yt", Source::Youtube, vec![], true, 100));
        let playing = playing_sources(&broker, 101);
        assert_eq!(playing, vec![Source::Spotify]);
        assert!(connector_represented(Some(Source::Spotify), &playing));
        assert!(!connector_represented(Some(Source::Youtube), &playing));
    }
    #[test]
    fn list_equality_treats_a_retitled_native_card_as_a_change() {
        let list = MediaList {
            cards: vec![native("card")],
            other_audio: false,
        };
        assert!(same_list(&list, &list.clone()));
        let mut retitled = list.clone();
        retitled.cards[0].title = "Other".into();
        assert!(!same_list(&list, &retitled));
    }
    #[test]
    fn list_equality_ignores_only_small_position_drift() {
        let (state, _rx) = state();
        let list = build(&state.broker, &[], 101);
        let mut same = list.clone();
        same.cards[0].position_ms += 1000;
        same.cards[0].position_updated_at = 9000;
        assert!(same_list(&list, &same));
        same.cards[0].position_ms += 1;
        assert!(!same_list(&list, &same));
        let mut retitled = list.clone();
        retitled.cards[0].title = "Other".into();
        assert!(!same_list(&list, &retitled));
    }
    #[test]
    fn close_logging_repeats_only_after_the_interval_or_a_new_reason() {
        // The first close for a source always logs.
        assert!(should_log_close(&None, "Idle", 0));
        let last = Some(("Idle".to_string(), 1_000));
        // The same reason inside the interval is suppressed.
        assert!(!should_log_close(&last, "Idle", 0));
        assert!(!should_log_close(&last, "Idle", 29_999));
        // At the interval the line returns, carrying the suppressed count.
        assert!(should_log_close(&last, "Idle", 30_000));
        // A changed reason always logs, however soon it follows.
        assert!(should_log_close(&last, "ReadError:io", 0));
    }
    #[test]
    fn close_detail_is_content_free_and_names_the_source_and_reason() {
        let detail = close_detail("Spotify", "ReadError:io", 18_750, 4, 8_100, 0);
        assert_eq!(
            detail,
            "source=Spotify reason=ReadError:io lifetime_ms=18750 messages=4 max_gap_ms=8100"
        );
        // A suppressed run appends its count; nothing else may appear.
        let suppressed = close_detail("YouTube", "Idle", 30_000, 0, 0, 7);
        assert_eq!(
            suppressed,
            "source=YouTube reason=Idle lifetime_ms=30000 messages=0 max_gap_ms=0 suppressed=7"
        );
        for detail in [&detail, &suppressed] {
            assert!(detail.split(' ').all(|field| {
                let (key, _) = field.split_once('=').unwrap();
                ["source", "reason", "lifetime_ms", "messages", "max_gap_ms", "suppressed"]
                    .contains(&key)
            }));
        }
    }
    #[test]
    fn json_failures_are_classified_without_the_error_text() {
        let kind = |text: &str| {
            json_error_kind(&serde_json::from_str::<Incoming>(text).err().unwrap())
        };
        assert_eq!(kind(r#"{"op":}"#), "syntax");
        assert_eq!(kind(r#"{"op":"nope"}"#), "data");
        assert_eq!(kind(""), "eof");
        assert_eq!(kind("{"), "eof");
    }
    #[test]
    fn read_failures_are_classified_without_the_error_text() {
        use tokio_tungstenite::tungstenite::error::CapacityError;
        use tokio_tungstenite::tungstenite::Error;
        let too_big = Error::Capacity(CapacityError::MessageTooLong {
            size: 2,
            max_size: 1,
        });
        assert_eq!(read_error_kind(&too_big), "message_too_big");
        assert_eq!(
            read_error_kind(&Error::Io(std::io::Error::from(
                std::io::ErrorKind::ConnectionReset
            ))),
            "io"
        );
        assert_eq!(read_error_kind(&Error::ConnectionClosed), "closed");
        assert_eq!(read_error_kind(&Error::AttackAttempt), "protocol");
        assert_eq!(
            read_error_kind(&Error::WriteBufferFull(Box::new(Message::text("")))),
            "other"
        );
    }
    #[test]
    fn app_names_come_from_the_smtc_app_id_not_from_the_source() {
        for (aumid, expected) in [
            ("spotify.exe", "Spotify"),
            ("Spotify", "Spotify"),
            ("SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify", "Spotify"),
            ("brave.exe", "Brave"),
            ("Brave", "Brave"),
            ("Brave.5UPU3OQA2NIDBH5RNJ7GS34UL4", "Brave"),
            ("MSEdge", "Edge"),
            ("msedge.exe", "Edge"),
            (
                "Microsoft.MicrosoftEdge_8wekyb3d8bbwe!MicrosoftEdge",
                "Edge",
            ),
            ("Chrome", "Chrome"),
            ("chrome.exe", "Chrome"),
            ("308046B0AF4A39CB", "Firefox"),
            ("firefox.exe", "Firefox"),
            (r"C:\Program Files\VideoLAN\VLC\vlc.exe", "VLC"),
            ("SomeApp.exe", "SomeApp"),
            ("", "Media"),
        ] {
            assert_eq!(app_display_name(aumid), expected, "aumid={aumid:?}");
        }
    }
    #[test]
    fn a_connector_card_names_its_app() {
        let (state, _rx) = state();
        let list = build(&state.broker, &[], 101);
        assert_eq!(list.cards[0].app, "YouTube");
        let mut broker = Broker::default();
        broker.update(
            "sp",
            Source::Spotify,
            vec![Snapshot {
                session: "spotify:track".into(),
                revision: "1".into(),
                title: "Song".into(),
                is_playing: true,
                observed_ms: 100,
                ..Default::default()
            }],
            false,
            100,
        );
        assert_eq!(build(&broker, &[], 101).cards[0].app, "Spotify");
    }
    #[test]
    fn a_brave_windows_card_suppresses_the_other_audio_note() {
        let (mut state, _rx) = state();
        // A fresh client that heard audio it could not attribute.
        assert!(state.broker.update("yt", Source::Youtube, vec![], true, 100));
        assert!(build(&state.broker, &[], 101).other_audio);
        let mut brave = native("brave");
        brave.family = Some(Source::Youtube);
        assert!(!build(&state.broker, &[brave], 101).other_audio);
        // A Windows player from another family leaves the note alone.
        let mut vlc = native("vlc");
        vlc.family = None;
        assert!(build(&state.broker, &[vlc], 101).other_audio);
    }
    #[test]
    fn list_equality_treats_a_renamed_app_as_a_change() {
        let (state, _rx) = state();
        let list = build(&state.broker, &[], 101);
        let mut renamed = list.clone();
        renamed.cards[0].app = "Brave".into();
        assert!(!same_list(&list, &renamed));
    }
    #[test]
    fn spotify_art_comes_only_from_one_spotify_session() {
        assert_eq!(spotify_art_session(&[]), None);
        assert_eq!(spotify_art_session(&[None, Some(Source::Youtube)]), None);
        assert_eq!(spotify_art_session(&[None, Some(Source::Spotify)]), Some(1));
        // Two Spotify players cannot say whose cover belongs on the card.
        assert_eq!(
            spotify_art_session(&[Some(Source::Spotify), Some(Source::Spotify)]),
            None
        );
        assert_eq!(
            spotify_art_session(&[Some(Source::Youtube), Some(Source::Spotify), None]),
            Some(1)
        );
    }
    fn playing_youtube() -> Snapshot {
        Snapshot {
            session: "video".into(),
            revision: "1".into(),
            current_id: "0:AAAAAAAAAAA".into(),
            title: "Video".into(),
            is_playing: true,
            queue_available: true,
            observed_ms: 100,
            ..Default::default()
        }
    }
    fn tab(id: i64, window: i64, muted: bool) -> TabInfo {
        TabInfo {
            id,
            title: format!("Tab {id}"),
            site: "example.com".into(),
            muted,
            window,
        }
    }
    #[test]
    fn tab_cards_follow_players_and_precede_windows_cards() {
        let mut broker = Broker::default();
        assert!(broker
            .try_update_tabs(
                "yt",
                Source::Youtube,
                vec![playing_youtube()],
                vec![tab(9, 2, true), tab(3, 1, false), tab(5, 1, false)],
                false,
                100
            )
            .is_ok());
        let list = build(&broker, &[native("native-1")], 101);
        let order: Vec<_> = list
            .cards
            .iter()
            .map(|c| (c.kind.as_str(), c.generation.as_str()))
            .collect();
        assert_eq!(
            order,
            [
                ("player", "yt:video"),
                ("tab", "yt:tab:3"),
                ("tab", "yt:tab:5"),
                ("tab", "yt:tab:9"),
                ("player", "native-1"),
            ]
        );
        let tab = &list.cards[1];
        assert_eq!(tab.source, "Tab");
        assert_eq!(tab.app, "Brave");
        assert_eq!(tab.title, "Tab 3");
        assert_eq!(tab.artist, "example.com");
        assert!(tab.is_playing && !tab.seek_enabled && !tab.queue && !tab.primary);
        assert!(tab.artwork.is_none() && tab.duration_ms == 0 && tab.position_ms == 0);
        assert!(!tab.muted);
        assert!(list.cards[3].muted);
        // Player cards keep their old shape: a player, never muted.
        assert_eq!(list.cards[0].kind, "player");
        assert!(!list.cards[0].muted);
        assert!(list.cards[0].queue);
    }
    #[test]
    fn stale_tabs_leave_the_card_list() {
        let mut broker = Broker::default();
        assert!(broker
            .try_update_tabs("yt", Source::Youtube, vec![], vec![tab(1, 1, false)], false, 100)
            .is_ok());
        assert_eq!(tab_cards(&broker, 6099).len(), 1);
        assert!(tab_cards(&broker, 6100).is_empty());
        assert!(build(&broker, &[], 6100).cards.is_empty());
    }
    #[test]
    fn a_fresh_tab_list_represents_the_youtube_family() {
        let mut broker = Broker::default();
        assert!(broker
            .try_update_tabs("yt", Source::Youtube, vec![], vec![tab(1, 1, false)], false, 100)
            .is_ok());
        let playing = playing_sources(&broker, 101);
        assert_eq!(playing, vec![Source::Youtube]);
        assert!(connector_represented(Some(Source::Youtube), &playing));
        // Spotify keeps its old rule: no playing card, no representation.
        assert!(!connector_represented(Some(Source::Spotify), &playing));
        // A stale tab list stops representing the family.
        assert!(playing_sources(&broker, 6100).is_empty());
    }
    #[test]
    fn list_equality_treats_a_muted_flip_as_a_change() {
        let mut broker = Broker::default();
        assert!(broker
            .try_update_tabs("yt", Source::Youtube, vec![], vec![tab(1, 1, false)], false, 100)
            .is_ok());
        let list = build(&broker, &[], 101);
        assert!(same_list(&list, &list.clone()));
        let mut muted = list.clone();
        muted.cards[0].muted = true;
        assert!(!same_list(&list, &muted));
    }
    #[test]
    fn a_player_operation_never_reaches_a_tab_generation() {
        let mut state = State::default();
        assert!(state
            .broker
            .try_update_tabs("yt", Source::Youtube, vec![], vec![tab(4, 1, false)], false, 100)
            .is_ok());
        assert_eq!(
            state
                .prepare("yt:tab:4", "playPause", None, None, None, 101)
                .err()
                .as_deref(),
            Some("Unsupported media action.")
        );
        // The tab's own operations are the only ones it accepts.
        let (tx, _rx) = mpsc::channel(16);
        state.senders.insert("yt".into(), tx);
        assert!(state
            .prepare("yt:tab:4", "mute", None, None, None, 101)
            .is_ok());
    }
    #[test]
    fn a_tab_operation_never_reaches_a_player_generation() {
        let (mut state, mut rx) = state();
        assert_eq!(
            state
                .prepare("conn:tab/document/player", "mute", None, None, None, 101)
                .err()
                .as_deref(),
            Some("Unsupported media action.")
        );
        assert!(rx.try_recv().is_err());
    }
    #[test]
    fn real_loopback_tab_mute_reaches_the_connector() {
        tokio::runtime::Builder::new_current_thread().enable_all().build().unwrap().block_on(async{
            let (config,state,worker)=bridge().await;
            let mut client=youtube_client(&config).await;
            client.send(Message::Text(json!({"op":"inventory","snapshots":[],"tabs":[{"id":7,"title":"Clip","site":"youtube.com","muted":false,"window":1}]}).to_string().into())).await.unwrap();
            // The client id is minted by the server, so read it back once the
            // inventory is stored instead of guessing the generation.
            let id=tokio::time::timeout(Duration::from_secs(2),async{
                loop{
                    if let Some(id)=state.lock().unwrap().broker.clients.keys().next().cloned(){break id;}
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }).await.unwrap();
            let reply=state.lock().unwrap().prepare(&format!("{id}:tab:7"),"mute",None,None,None,now()).unwrap();
            let command=tokio::time::timeout(Duration::from_secs(2),client.next()).await.unwrap().unwrap().unwrap();
            let command:Value=serde_json::from_str(command.to_text().unwrap()).unwrap();
            assert_eq!(command["op"],"mute");
            assert_eq!(command["session"],"tab:7");
            client.send(Message::Text(json!({"op":"result","id":command["id"],"status":"confirmed"}).to_string().into())).await.unwrap();
            assert_eq!(tokio::time::timeout(Duration::from_secs(2),reply).await.unwrap().unwrap(),"confirmed");
            client.close(None).await.unwrap();tokio::time::timeout(Duration::from_secs(2),worker).await.unwrap().unwrap();
        });
    }
    async fn youtube_client(config: &Settings) -> tokio_tungstenite::WebSocketStream<tokio::net::TcpStream> {
        use tokio_tungstenite::tungstenite::client::IntoClientRequest;
        let port=config.port;
        let mut request=format!("ws://127.0.0.1:{port}/youtube").into_client_request().unwrap();
        request.headers_mut().insert("Origin",format!("chrome-extension://{}","a".repeat(32)).parse().unwrap());
        request.headers_mut().insert("Sec-WebSocket-Protocol",format!("bloom.v1, {}",config.youtube).parse().unwrap());
        let stream=tokio::net::TcpStream::connect((std::net::Ipv4Addr::LOCALHOST,port)).await.unwrap();
        tokio_tungstenite::client_async(request,stream).await.unwrap().0
    }
}
