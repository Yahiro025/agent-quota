//! Source-bound media state. Never associate browser queues by track text.
use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Source {
    Spotify,
    Youtube,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
pub struct Entry {
    pub id: String,
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub duration: Option<String>,
    #[serde(default)]
    pub uri: Option<String>,
    #[serde(default)]
    pub uid: Option<String>,
    #[serde(default)]
    pub index: Option<u32>,
    #[serde(default)]
    pub video_id: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
pub struct Snapshot {
    pub session: String,
    pub revision: String,
    pub current_id: String,
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub is_playing: bool,
    #[serde(default)]
    pub position_ms: i64,
    #[serde(default)]
    pub duration_ms: i64,
    #[serde(default)]
    pub seek_enabled: bool,
    #[serde(default)]
    pub queue_available: bool,
    #[serde(default)]
    pub reason: Option<String>,
    #[serde(default)]
    pub observed_ms: u64,
    #[serde(default)]
    pub entries: Vec<Entry>,
    #[serde(default)]
    pub total_upcoming: Option<u32>,
}

/// One audible browser tab the YouTube connector lists. A tab's player lives
/// in a cross-origin iframe, so the row carries no queue and no transport:
/// only the tab-level mute and show commands act on it.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize, Serialize)]
pub struct TabInfo {
    pub id: i64,
    pub title: String,
    pub site: String,
    #[serde(default)]
    pub muted: bool,
    pub window: i64,
}

#[derive(Default, Clone)]
pub struct NativeHint {
    pub source: Option<Source>,
    pub playing: bool,
    pub has_media: bool,
}
#[derive(Clone)]
pub struct Target {
    pub generation: String,
    pub client: String,
    pub source: Source,
    pub snapshot: Snapshot,
}
pub struct Client {
    pub source: Source,
    pub inventory: Vec<Snapshot>,
    /// Browser tabs share the client's `received` freshness, because they
    /// arrive in the same inventory message.
    pub tabs: Vec<TabInfo>,
    pub received: u64,
    pub unknown_playing: bool,
}
#[derive(Default)]
pub struct Broker {
    pub clients: std::collections::HashMap<String, Client>,
    pub native: NativeHint,
    pub native_generation: String,
}
/// Why no queue can be owned right now. The reason decides which guidance the
/// UI can honestly give, so a bare `None` would collapse distinct fixes into
/// one misleading message.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NoTarget {
    /// The displayed source heard audio it could not attribute to one queue.
    UnknownAudible,
    /// More than one fresh session of the displayed source could own the queue.
    AmbiguousPlaying,
    /// The connector's play state disagrees with the Windows session.
    NativeMismatch,
    /// No fresh session exists in a supported, displayed source.
    NoSession,
}
/// Why a connector's inventory was refused. `update` collapses every bound
/// into one `false`, so a live connector that never registers cannot be told
/// apart from a stale one; the variant is the only evidence the log can show.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Rejected {
    /// An empty or over-long client id cannot key the client map.
    BadClientId,
    /// One update carries more sessions than the broker stores at once.
    TooManySnapshots,
    /// A new client would exceed the connector slots the broker serves.
    TooManyClients,
    /// Reusing a client id for another source would cross-bind its queue.
    SourceMismatch,
    /// The inventory across all clients would exceed the total bound.
    InventoryTooLarge,
    /// A snapshot failed the content and identity checks, so it is unusable.
    InvalidSnapshot,
    /// Two snapshots in one update claim the same session.
    DuplicateSession,
    /// The snapshot predates the freshness window the resolver uses.
    StaleSnapshot,
    /// The snapshot claims a clock ahead of ours, so its age cannot be trusted.
    FutureSnapshot,
    /// The tab list exceeds its bound or one tab's identity or text is unusable.
    InvalidTab,
}

impl Broker {
    pub fn resolve_action(
        &self,
        generation: &str,
        revision: Option<&str>,
        entry: Option<&str>,
        now: u64,
    ) -> Result<(Target, Option<Entry>), String> {
        let target = self
            .target_for(generation, now)
            .ok_or("Media changed. Check the queue again.")?;
        let selected = if let Some(id) = entry {
            if !target.snapshot.queue_available
                || revision != Some(target.snapshot.revision.as_str())
            {
                return Err("Queue changed. Check it again.".into());
            }
            Some(
                target
                    .snapshot
                    .entries
                    .iter()
                    .find(|e| e.id == id)
                    .cloned()
                    .ok_or("That entry is no longer queued.")?,
            )
        } else {
            None
        };
        Ok((target, selected))
    }
    /// `update`'s body, but it names the bound that refused the inventory.
    /// The checks keep their original order, so what is accepted is unchanged.
    pub fn try_update(
        &mut self,
        client: &str,
        source: Source,
        snapshots: Vec<Snapshot>,
        unknown: bool,
        now: u64,
    ) -> Result<(), Rejected> {
        self.try_update_tabs(client, source, snapshots, Vec::new(), unknown, now)
    }
    /// `try_update` for an inventory that may list audible browser tabs.
    /// Only the YouTube connector can see tabs; Spotify's field is ignored
    /// rather than rejected, so an older or stray client cannot be refused.
    pub fn try_update_tabs(
        &mut self,
        client: &str,
        source: Source,
        mut snapshots: Vec<Snapshot>,
        tabs: Vec<TabInfo>,
        unknown: bool,
        now: u64,
    ) -> Result<(), Rejected> {
        let tabs = if source == Source::Youtube {
            tabs
        } else {
            Vec::new()
        };
        if client.is_empty() || client.len() > 128 {
            return Err(Rejected::BadClientId);
        }
        if snapshots.len() > 16 {
            return Err(Rejected::TooManySnapshots);
        }
        if !self.clients.contains_key(client) && self.clients.len() >= 4 {
            return Err(Rejected::TooManyClients);
        }
        if self.clients.get(client).is_some_and(|c| c.source != source) {
            return Err(Rejected::SourceMismatch);
        }
        if self
            .clients
            .iter()
            .filter(|(id, _)| id.as_str() != client)
            .map(|(_, c)| c.inventory.len())
            .sum::<usize>()
            + snapshots.len()
            > 16
        {
            return Err(Rejected::InventoryTooLarge);
        }
        // A tab row is display-only, so its bounds are looser than a snapshot's
        // but still bounded: a blank row names nothing the user could choose.
        if tabs.len() > 8
            || tabs.iter().any(|tab| {
                (tab.title.is_empty() && tab.site.is_empty())
                    || tab.title.chars().count() > 512
                    || tab.site.chars().count() > 253
                    || tab.id < 0
                    || tab.window < 0
            })
        {
            return Err(Rejected::InvalidTab);
        }
        let mut ids = std::collections::HashSet::new();
        for snapshot in &snapshots {
            if !validate(source, snapshot) {
                return Err(Rejected::InvalidSnapshot);
            }
            if !ids.insert(snapshot.session.as_str()) {
                return Err(Rejected::DuplicateSession);
            }
            if now.saturating_sub(snapshot.observed_ms) > 6000 {
                return Err(Rejected::StaleSnapshot);
            }
            if snapshot.observed_ms > now.saturating_add(1000) {
                return Err(Rejected::FutureSnapshot);
            }
        }
        // A periodic read can finish after an action's newer observed result.
        // Keep that result, including timestamp ties, instead of rolling back
        // the displayed occurrence. Removed sessions are still removed.
        if let Some(previous) = self.clients.get(client) {
            for incoming in &mut snapshots {
                if let Some(newer) = previous.inventory.iter().find(|old| {
                    old.session == incoming.session && old.observed_ms >= incoming.observed_ms
                }) {
                    *incoming = newer.clone();
                }
            }
        }
        self.clients.insert(
            client.into(),
            Client {
                source,
                inventory: snapshots,
                tabs,
                received: now,
                unknown_playing: unknown,
            },
        );
        Ok(())
    }
    // The bool wrapper stays for callers and tests that only need the verdict;
    // the app path uses `try_update` to name the refusal.
    #[allow(dead_code)]
    pub fn update(
        &mut self,
        client: &str,
        source: Source,
        snapshots: Vec<Snapshot>,
        unknown: bool,
        now: u64,
    ) -> bool {
        self.try_update(client, source, snapshots, unknown, now).is_ok()
    }
    pub fn resolve(&self, now: u64) -> Result<Target, NoTarget> {
        let clients: Vec<_> = self
            .clients
            .iter()
            .filter(|(_, c)| now.saturating_sub(c.received) < 6000)
            .collect();
        // The Windows session says which source Bloom is displaying. Scope
        // ambiguity to that source so another player cannot disable its queue.
        let native_scoped = self.native.has_media && self.native.source.is_some();
        let scoped: Vec<_> = clients
            .iter()
            .filter(|(_, c)| !native_scoped || Some(c.source) == self.native.source)
            .map(|(id, c)| (*id, *c))
            .collect();
        if scoped.iter().any(|(_, c)| c.unknown_playing) {
            return Err(NoTarget::UnknownAudible);
        }
        let candidates: Vec<_> = scoped
            .iter()
            .flat_map(|(id, c)| {
                c.inventory
                    .iter()
                    .filter(move |s| now.saturating_sub(s.observed_ms) < 6000)
                    .map(move |s| (*id, *c, s))
            })
            .collect();
        // An empty candidate set cannot be a play-state conflict; say so, so the
        // UI does not ask the user to pause a player that reported nothing.
        if candidates.is_empty() {
            return Err(NoTarget::NoSession);
        }
        let playing: Vec<_> = candidates.iter().filter(|(_, _, s)| s.is_playing).collect();
        let paused: Vec<_> = candidates
            .iter()
            .filter(|(_, _, s)| !s.is_playing)
            .collect();
        let choice = if native_scoped {
            // The connector must agree with the native play state; otherwise the
            // snapshot is stale and cannot own the displayed queue.
            let expected = if self.native.playing {
                &playing
            } else {
                &paused
            };
            if expected.len() != 1 {
                return Err(if expected.len() > 1 {
                    NoTarget::AmbiguousPlaying
                } else {
                    NoTarget::NativeMismatch
                });
            }
            *expected[0]
        } else if playing.len() == 1 {
            let chosen = playing[0];
            if self.native.playing && self.native.source != Some(chosen.1.source) {
                return Err(NoTarget::NativeMismatch);
            }
            *chosen
        } else if playing.is_empty() && !self.native.playing {
            let paused: Vec<_> = candidates
                .iter()
                .filter(|(_, c, _)| !self.native.has_media || self.native.source == Some(c.source))
                .collect();
            if paused.len() != 1 {
                return Err(if paused.len() > 1 {
                    NoTarget::AmbiguousPlaying
                } else {
                    NoTarget::NoSession
                });
            }
            *paused[0]
        } else {
            return Err(if playing.len() > 1 {
                NoTarget::AmbiguousPlaying
            } else {
                NoTarget::NoSession
            });
        };
        Ok(Target {
            generation: format!("{}:{}", choice.0, choice.2.session),
            client: choice.0.clone(),
            source: choice.1.source,
            snapshot: choice.2.clone(),
        })
    }
    pub fn active(&self, now: u64) -> Option<Target> {
        self.resolve(now).ok()
    }
    /// Every playing connector snapshot, for the per-player cards. Unknown
    /// audible tabs are reported by `other_audio` instead of filtering here:
    /// one unattributable tab must not hide another player's card or queue.
    pub fn targets(&self, now: u64) -> Vec<Target> {
        let mut targets: Vec<_> = self
            .clients
            .iter()
            .filter(|(_, c)| now.saturating_sub(c.received) < 6000)
            .flat_map(|(id, c)| {
                c.inventory
                    .iter()
                    .filter(move |s| s.is_playing && now.saturating_sub(s.observed_ms) < 6000)
                    .map(move |s| Target {
                        generation: format!("{id}:{}", s.session),
                        client: id.clone(),
                        source: c.source,
                        snapshot: s.clone(),
                    })
            })
            .collect();
        // The client map has no order, so sort: Spotify before YouTube, then by
        // generation, so the cards keep their place between polls.
        targets.sort_by_key(|t| (t.source != Source::Spotify, t.generation.clone()));
        targets
    }
    /// A generation is addressable while it is a playing card or the primary,
    /// so a paused primary keeps owning its queue exactly as before.
    pub fn target_for(&self, generation: &str, now: u64) -> Option<Target> {
        self.targets(now)
            .into_iter()
            .find(|t| t.generation == generation)
            .or_else(|| self.active(now).filter(|t| t.generation == generation))
    }
    /// Audio the connectors heard but could not attribute to a player.
    pub fn other_audio(&self, now: u64) -> bool {
        self.clients
            .values()
            .any(|c| c.unknown_playing && now.saturating_sub(c.received) < 6000)
    }
    /// The tab a `"{client}:tab:{id}"` generation names, while that client is
    /// fresh and still lists the tab. A generation that already resolves as a
    /// player is never a tab: the player's meaning wins.
    pub fn tab_for(&self, generation: &str, now: u64) -> Option<(String, TabInfo)> {
        if self.target_for(generation, now).is_some() {
            return None;
        }
        let (client, id) = generation.rsplit_once(":tab:")?;
        let id: i64 = id.parse().ok()?;
        let c = self.clients.get(client)?;
        if c.source != Source::Youtube || now.saturating_sub(c.received) >= 6000 {
            return None;
        }
        c.tabs
            .iter()
            .find(|tab| tab.id == id)
            .cloned()
            .map(|tab| (client.into(), tab))
    }
}

pub fn validate(source: Source, snapshot: &Snapshot) -> bool {
    let bounded = |s: &str, max| s.chars().count() <= max;
    if snapshot.session.is_empty()
        || !bounded(&snapshot.session, 256)
        || snapshot.revision.is_empty()
        || !bounded(&snapshot.revision, 128)
        || !bounded(&snapshot.current_id, 256)
        || snapshot.title.is_empty()
        || !bounded(&snapshot.title, 512)
        || !bounded(&snapshot.artist, 512)
        || snapshot.entries.len() > 64
        || snapshot.position_ms < 0
        || snapshot.duration_ms < 0
        || snapshot.position_ms > 31_536_000_000
        || snapshot.duration_ms > 31_536_000_000
        || snapshot.reason.as_ref().is_some_and(|s| !bounded(s, 512))
        || (!snapshot.queue_available && !snapshot.entries.is_empty())
    {
        return false;
    }
    let mut seen = std::collections::HashSet::new();
    snapshot.entries.iter().all(|e| {
        if e.id.is_empty()
            || !bounded(&e.id, 256)
            || !seen.insert(&e.id)
            || !bounded(&e.title, 512)
            || !bounded(&e.artist, 512)
            || e.duration.as_ref().is_some_and(|s| !bounded(s, 32))
        {
            return false;
        }
        match source {
            Source::Spotify => {
                e.uid.as_ref().is_some_and(|s| s == &e.id)
                    && e.uri.as_ref().is_some_and(|s| {
                        bounded(s, 256)
                            && (s.starts_with("spotify:track:")
                                || s.starts_with("spotify:episode:"))
                    })
            }
            Source::Youtube => {
                e.index.is_some_and(|index| index < 5000)
                    && e.video_id.as_ref().is_some_and(|id| {
                        id.len() == 11
                            && id
                                .bytes()
                                .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
                            && e.id == format!("{}:{id}", e.index.unwrap_or(5000))
                    })
            }
        }
    })
}

/// Which `validate` check a snapshot fails, or `""` when it passes. The bool
/// cannot say whether a connector broke a title or an id, and the log must not
/// carry the content itself.
pub fn invalid_field(source: Source, snapshot: &Snapshot) -> &'static str {
    let bounded = |s: &str, max| s.chars().count() <= max;
    if snapshot.session.is_empty() || !bounded(&snapshot.session, 256) {
        return "session";
    }
    if snapshot.revision.is_empty() || !bounded(&snapshot.revision, 128) {
        return "revision";
    }
    if !bounded(&snapshot.current_id, 256) {
        return "current_id";
    }
    if snapshot.title.is_empty() {
        return "title_empty";
    }
    if !bounded(&snapshot.title, 512) {
        return "title_len";
    }
    if !bounded(&snapshot.artist, 512) {
        return "artist";
    }
    if snapshot.entries.len() > 64 {
        return "entries_over_64";
    }
    if snapshot.position_ms < 0 || snapshot.position_ms > 31_536_000_000 {
        return "position";
    }
    if snapshot.duration_ms < 0 || snapshot.duration_ms > 31_536_000_000 {
        return "duration";
    }
    if snapshot.reason.as_ref().is_some_and(|s| !bounded(s, 512)) {
        return "reason_len";
    }
    if !snapshot.queue_available && !snapshot.entries.is_empty() {
        return "entries_when_unavailable";
    }
    let mut seen = std::collections::HashSet::new();
    for entry in &snapshot.entries {
        if entry.id.is_empty()
            || !bounded(&entry.id, 256)
            || !seen.insert(&entry.id)
            || !bounded(&entry.title, 512)
            || !bounded(&entry.artist, 512)
            || entry.duration.as_ref().is_some_and(|s| !bounded(s, 32))
        {
            return "entry";
        }
        let bound = match source {
            Source::Spotify => {
                entry.uid.as_ref().is_some_and(|s| s == &entry.id)
                    && entry.uri.as_ref().is_some_and(|s| {
                        bounded(s, 256)
                            && (s.starts_with("spotify:track:")
                                || s.starts_with("spotify:episode:"))
                    })
            }
            Source::Youtube => {
                entry.index.is_some_and(|index| index < 5000)
                    && entry.video_id.as_ref().is_some_and(|id| {
                        id.len() == 11
                            && id
                                .bytes()
                                .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
                            && entry.id == format!("{}:{id}", entry.index.unwrap_or(5000))
                    })
            }
        };
        if !bound {
            return "entry";
        }
    }
    ""
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn delayed_inventory_never_replaces_a_newer_observed_result() {
        let mut b = Broker::default();
        let mut fresh = spotify();
        fresh.observed_ms = 200;
        fresh.title = "Confirmed current".into();
        b.update("conn", Source::Spotify, vec![fresh.clone()], false, 200);
        for timestamp in [199, 200] {
            let mut delayed = fresh.clone();
            delayed.observed_ms = timestamp;
            delayed.title = "Old inventory".into();
            assert!(b.update("conn", Source::Spotify, vec![delayed], false, 201));
            assert_eq!(b.clients["conn"].inventory[0].title, "Confirmed current");
        }
    }
    fn spotify() -> Snapshot {
        Snapshot {
            session: "instance".into(),
            revision: "123".into(),
            current_id: "old".into(),
            title: "Song".into(),
            queue_available: true,
            entries: vec![Entry {
                id: "q1".into(),
                title: "Next".into(),
                uri: Some("spotify:track:a".into()),
                uid: Some("q1".into()),
                ..Default::default()
            }],
            ..Default::default()
        }
    }
    fn youtube() -> Snapshot {
        Snapshot {
            session: "tab/document/player".into(),
            revision: "1".into(),
            current_id: "0:AAAAAAAAAAA".into(),
            title: "Video".into(),
            queue_available: true,
            entries: vec![Entry {
                id: "1:BBBBBBBBBBB".into(),
                title: "Next".into(),
                index: Some(1),
                video_id: Some("BBBBBBBBBBB".into()),
                ..Default::default()
            }],
            ..Default::default()
        }
    }
    #[test]
    fn accepts_bound_occurrences_not_just_titles() {
        assert!(validate(Source::Spotify, &spotify()));
    }
    #[test]
    fn rejects_duplicate_occurrence_identity() {
        let mut s = spotify();
        s.entries.push(s.entries[0].clone());
        assert!(!validate(Source::Spotify, &s));
    }
    #[test]
    fn rejects_oversized_state_without_silently_dropping_rows() {
        let mut s = spotify();
        s.entries = (0..65)
            .map(|i| Entry {
                id: format!("q{i}"),
                uid: Some(format!("q{i}")),
                ..s.entries[0].clone()
            })
            .collect();
        assert!(!validate(Source::Spotify, &s));
        s = spotify();
        s.title = "x".repeat(513);
        assert!(!validate(Source::Spotify, &s));
    }
    #[test]
    fn rejects_cross_source_actions() {
        let s = spotify();
        assert!(!validate(Source::Youtube, &s));
    }
    #[test]
    fn empty_queue_can_be_available() {
        let mut s = spotify();
        s.entries.clear();
        assert!(validate(Source::Spotify, &s));
    }
    #[test]
    fn unavailable_state_cannot_smuggle_actionable_entries() {
        let mut s = spotify();
        s.queue_available = false;
        assert!(!validate(Source::Spotify, &s));
    }
    #[test]
    fn unique_playing_connector_owns_its_complete_target() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        assert!(b.update("conn", Source::Spotify, vec![s], false, 100));
        let t = b.active(101).unwrap();
        assert_eq!(t.generation, "conn:instance");
        assert_eq!(t.snapshot.title, "Song");
    }
    #[test]
    fn ambiguous_or_unknown_active_media_never_infers_a_queue() {
        let mut b = Broker::default();
        let mut a = spotify();
        a.is_playing = true;
        b.update("one", Source::Spotify, vec![a.clone()], false, 100);
        a.session = "two".into();
        b.update("two", Source::Spotify, vec![a], false, 100);
        assert!(b.active(101).is_none());
        b.clients.remove("two");
        b.native = NativeHint {
            source: None,
            playing: true,
            has_media: true,
        };
        assert!(b.active(101).is_none());
    }
    #[test]
    fn native_source_selects_the_displayed_source_when_both_are_playing() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        let mut youtube = youtube();
        youtube.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        let target = b.active(101).expect("the displayed source owns the queue");
        assert_eq!(target.source, Source::Spotify);
        assert_eq!(target.client, "spotify");
    }
    #[test]
    fn native_source_ignores_another_sources_unknown_playing() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![], true, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        assert_eq!(b.active(101).unwrap().source, Source::Spotify);
    }
    #[test]
    fn native_source_still_rejects_multiple_playing_sessions_of_its_own_source() {
        let mut b = Broker::default();
        let mut one = spotify();
        one.is_playing = true;
        let mut two = spotify();
        two.session = "second".into();
        two.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![one, two], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        assert!(b.active(101).is_none());
    }
    #[test]
    fn native_youtube_source_owns_the_unique_browser_session() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        let mut youtube = youtube();
        youtube.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        b.native = NativeHint {
            source: Some(Source::Youtube),
            playing: true,
            has_media: true,
        };
        let target = b.active(101).expect("YouTube owns the displayed queue");
        assert_eq!(target.source, Source::Youtube);
        assert_eq!(target.client, "youtube");
    }
    #[test]
    fn native_source_owns_its_queue_when_the_other_player_is_paused_but_fresh() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        let mut youtube = youtube();
        youtube.is_playing = false;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        let target = b.active(101).expect("the displayed source owns the queue");
        assert_eq!(target.source, Source::Spotify);
        assert_eq!(target.client, "spotify");
    }
    #[test]
    fn unavailable_snapshot_stays_active_so_its_reason_reaches_the_ui() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        s.queue_available = false;
        s.reason = Some("Queue unavailable for this Spotify context.".into());
        s.entries.clear();
        assert!(b.update("spotify", Source::Spotify, vec![s], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        let target = b
            .active(101)
            .expect("an unavailable queue still identifies the displayed source");
        assert!(!target.snapshot.queue_available);
        assert_eq!(
            target.snapshot.reason.as_deref(),
            Some("Queue unavailable for this Spotify context.")
        );
    }
    #[test]
    fn native_paused_source_owns_its_paused_session_even_when_another_source_plays() {
        let mut b = Broker::default();
        let spotify = spotify();
        let mut youtube = youtube();
        youtube.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: false,
            has_media: true,
        };
        assert_eq!(b.active(101).unwrap().source, Source::Spotify);
    }
    #[test]
    fn stale_inventory_and_unknown_browser_players_disable_queue_routing() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        b.update("conn", Source::Spotify, vec![s.clone()], false, 100);
        assert!(b.active(6101).is_none());
        b.update("conn", Source::Spotify, vec![s], true, 6200);
        assert!(b.active(6201).is_none());
    }
    #[test]
    fn reconnect_changes_target_generation_even_for_identical_metadata() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        b.update("old", Source::Spotify, vec![s.clone()], false, 100);
        let old = b.active(101).unwrap().generation;
        b.clients.clear();
        b.update("new", Source::Spotify, vec![s], false, 101);
        assert_ne!(old, b.active(102).unwrap().generation);
    }
    #[test]
    fn invalid_inventory_cannot_replace_good_state() {
        let mut b = Broker::default();
        let s = spotify();
        b.update("conn", Source::Spotify, vec![s.clone()], false, 100);
        let mut bad = s;
        bad.entries.push(bad.entries[0].clone());
        assert!(!b.update("conn", Source::Spotify, vec![bad], false, 101));
        assert_eq!(b.clients["conn"].received, 100);
    }
    #[test]
    fn resolves_only_the_displayed_generation_revision_and_stored_occurrence() {
        let mut b = Broker::default();
        b.update("conn", Source::Spotify, vec![spotify()], false, 100);
        let (target, entry) = b
            .resolve_action("conn:instance", Some("123"), Some("q1"), 101)
            .unwrap();
        assert_eq!(target.client, "conn");
        assert_eq!(entry.unwrap().uid.as_deref(), Some("q1"));
        assert!(b
            .resolve_action("old:instance", Some("123"), Some("q1"), 101)
            .is_err());
        assert!(b
            .resolve_action("conn:instance", Some("122"), Some("q1"), 101)
            .is_err());
        assert!(b
            .resolve_action("conn:instance", Some("123"), Some("forged"), 101)
            .is_err());
    }
    #[test]
    fn unknown_audible_tab_reports_its_own_reason() {
        let mut b = Broker::default();
        assert!(b.update("youtube", Source::Youtube, vec![], true, 100));
        b.native = NativeHint {
            source: Some(Source::Youtube),
            playing: true,
            has_media: true,
        };
        assert_eq!(b.resolve(101).err(), Some(NoTarget::UnknownAudible));
        assert!(b.active(101).is_none());
        let mut playing = youtube();
        playing.is_playing = true;
        assert!(b.update("youtube", Source::Youtube, vec![playing], false, 102));
        assert!(b.resolve(103).is_ok());
        assert!(b.active(103).is_some());
    }
    #[test]
    fn two_playing_sessions_of_the_displayed_source_are_ambiguous() {
        let mut b = Broker::default();
        let mut one = spotify();
        one.is_playing = true;
        let mut two = spotify();
        two.session = "second".into();
        two.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![one, two], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        assert_eq!(b.resolve(101).err(), Some(NoTarget::AmbiguousPlaying));
        assert!(b.active(101).is_none());
        let mut one = spotify();
        one.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![one], false, 102));
        assert!(b.resolve(103).is_ok());
        assert!(b.active(103).is_some());
    }
    #[test]
    fn connector_play_state_disagreeing_with_windows_is_a_native_mismatch() {
        let mut b = Broker::default();
        assert!(b.update("spotify", Source::Spotify, vec![spotify()], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        assert_eq!(b.resolve(101).err(), Some(NoTarget::NativeMismatch));
        assert!(b.active(101).is_none());
        b.native.playing = false;
        assert!(b.resolve(102).is_ok());
        assert!(b.active(102).is_some());
    }
    #[test]
    fn no_fresh_session_reports_no_session() {
        let mut b = Broker::default();
        assert_eq!(b.resolve(101).err(), Some(NoTarget::NoSession));
        assert!(b.active(101).is_none());
        assert!(b.update("conn", Source::Spotify, vec![spotify()], false, 102));
        assert!(b.resolve(103).is_ok());
        assert!(b.active(103).is_some());
    }
    #[test]
    fn resolve_and_active_agree_on_the_target() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        assert!(b.update("conn", Source::Spotify, vec![s], false, 100));
        let resolved = b.resolve(101).expect("a unique playing session resolves");
        assert_eq!(
            b.active(101).map(|target| target.generation),
            Some(resolved.generation)
        );
    }
    #[test]
    fn targets_list_every_playing_connector_in_a_stable_order() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        let mut youtube = youtube();
        youtube.is_playing = true;
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        b.native = NativeHint {
            source: Some(Source::Youtube),
            playing: true,
            has_media: true,
        };
        // Cards are ordered for display, not by the primary: Spotify first.
        let targets = b.targets(101);
        assert_eq!(
            targets
                .iter()
                .map(|t| t.generation.as_str())
                .collect::<Vec<_>>(),
            ["spotify:instance", "youtube:tab/document/player"]
        );
        // The primary still comes from resolve(), unchanged.
        assert_eq!(
            b.resolve(101).unwrap().generation,
            "youtube:tab/document/player"
        );
    }
    #[test]
    fn unknown_audio_is_reported_but_never_hides_a_playing_card() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![s], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![], true, 100));
        assert_eq!(b.targets(101).len(), 1);
        assert!(b.other_audio(101));
        b.native = NativeHint {
            source: Some(Source::Youtube),
            playing: true,
            has_media: true,
        };
        // The displayed source is still refused; only the card list ignores it.
        assert_eq!(b.resolve(101).err(), Some(NoTarget::UnknownAudible));
        assert_eq!(b.targets(101).len(), 1);
        assert!(!b.other_audio(6101));
    }
    #[test]
    fn a_paused_primary_is_addressable_without_becoming_a_card() {
        let mut b = Broker::default();
        assert!(b.update("spotify", Source::Spotify, vec![spotify()], false, 100));
        assert!(b.targets(101).is_empty());
        assert_eq!(b.resolve(101).unwrap().generation, "spotify:instance");
        assert!(b.target_for("spotify:instance", 101).is_some());
        assert!(b.target_for("gone:instance", 101).is_none());
    }
    #[test]
    fn actions_reach_a_non_primary_playing_target() {
        let mut b = Broker::default();
        let mut spotify = spotify();
        spotify.is_playing = true;
        let mut youtube = youtube();
        youtube.is_playing = true;
        assert!(b.update("spotify", Source::Spotify, vec![spotify], false, 100));
        assert!(b.update("youtube", Source::Youtube, vec![youtube], false, 100));
        b.native = NativeHint {
            source: Some(Source::Spotify),
            playing: true,
            has_media: true,
        };
        assert!(b
            .resolve_action(
                "youtube:tab/document/player",
                Some("1"),
                Some("1:BBBBBBBBBBB"),
                101
            )
            .is_ok());
        assert_eq!(
            b.resolve_action("youtube:gone", None, None, 101)
                .err()
                .as_deref(),
            Some("Media changed. Check the queue again.")
        );
    }
    #[test]
    fn stale_snapshots_leave_the_card_list() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.is_playing = true;
        s.observed_ms = 100;
        assert!(b.update("conn", Source::Spotify, vec![s], false, 100));
        assert_eq!(b.targets(6099).len(), 1);
        // 6000 ms old is outside the window resolve() uses, so it is not a card.
        assert!(b.targets(6100).is_empty());
        assert!(b.target_for("conn:instance", 6100).is_none());
    }
    fn fresh(index: usize) -> Snapshot {
        let mut s = spotify();
        s.session = format!("s{index}");
        s
    }
    #[test]
    fn rejects_an_empty_or_oversized_client_id() {
        let mut b = Broker::default();
        assert_eq!(
            b.try_update("", Source::Spotify, vec![spotify()], false, 100),
            Err(Rejected::BadClientId)
        );
        let long = "c".repeat(129);
        assert_eq!(
            b.try_update(&long, Source::Spotify, vec![spotify()], false, 100),
            Err(Rejected::BadClientId)
        );
    }
    #[test]
    fn rejects_more_snapshots_than_one_update_may_carry() {
        let mut b = Broker::default();
        let many = (0..17).map(fresh).collect();
        assert_eq!(
            b.try_update("conn", Source::Spotify, many, false, 100),
            Err(Rejected::TooManySnapshots)
        );
    }
    #[test]
    fn rejects_a_fifth_client_but_still_accepts_a_known_one() {
        let mut b = Broker::default();
        for id in ["a", "b", "c", "d"] {
            assert!(b.update(id, Source::Spotify, vec![spotify()], false, 100));
        }
        assert_eq!(
            b.try_update("e", Source::Spotify, vec![spotify()], false, 100),
            Err(Rejected::TooManyClients)
        );
        assert!(b
            .try_update("a", Source::Spotify, vec![spotify()], false, 100)
            .is_ok());
    }
    #[test]
    fn rejects_reusing_a_client_id_for_another_source() {
        let mut b = Broker::default();
        assert!(b.update("conn", Source::Spotify, vec![spotify()], false, 100));
        assert_eq!(
            b.try_update("conn", Source::Youtube, vec![youtube()], false, 100),
            Err(Rejected::SourceMismatch)
        );
    }
    #[test]
    fn rejects_inventory_over_the_total_bound() {
        let mut b = Broker::default();
        assert!(b.update("a", Source::Spotify, (0..16).map(fresh).collect(), false, 100));
        assert_eq!(
            b.try_update("b", Source::Spotify, vec![fresh(16)], false, 100),
            Err(Rejected::InventoryTooLarge)
        );
    }
    #[test]
    fn rejects_a_snapshot_that_fails_validation() {
        let mut b = Broker::default();
        let mut s = spotify();
        let base = s.entries[0].clone();
        s.entries = (0..65)
            .map(|i| Entry {
                id: format!("q{i}"),
                uid: Some(format!("q{i}")),
                ..base.clone()
            })
            .collect();
        assert_eq!(
            b.try_update("conn", Source::Spotify, vec![s], false, 100),
            Err(Rejected::InvalidSnapshot)
        );
    }
    #[test]
    fn rejects_two_snapshots_for_one_session() {
        let mut b = Broker::default();
        let mut twin = spotify();
        twin.title = "Other".into();
        assert_eq!(
            b.try_update("conn", Source::Spotify, vec![spotify(), twin], false, 100),
            Err(Rejected::DuplicateSession)
        );
    }
    #[test]
    fn rejects_a_snapshot_older_than_the_freshness_window() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.observed_ms = 0;
        assert_eq!(
            b.try_update("conn", Source::Spotify, vec![s.clone()], false, 6001),
            Err(Rejected::StaleSnapshot)
        );
        // Exactly 6000 ms old is still fresh enough to accept.
        assert!(b
            .try_update("conn", Source::Spotify, vec![s], false, 6000)
            .is_ok());
    }
    #[test]
    fn rejects_a_snapshot_ahead_of_our_clock() {
        let mut b = Broker::default();
        let mut s = spotify();
        s.observed_ms = 1101;
        assert_eq!(
            b.try_update("conn", Source::Spotify, vec![s.clone()], false, 100),
            Err(Rejected::FutureSnapshot)
        );
        // One second of clock skew is tolerated.
        s.observed_ms = 1100;
        assert!(b
            .try_update("conn", Source::Spotify, vec![s], false, 100)
            .is_ok());
    }
    #[test]
    fn update_reports_the_same_verdict_as_try_update() {
        let mut accept = Broker::default();
        assert_eq!(
            accept.update("conn", Source::Spotify, vec![spotify()], false, 100),
            Broker::default()
                .try_update("conn", Source::Spotify, vec![spotify()], false, 100)
                .is_ok()
        );
        let mut stale = spotify();
        stale.observed_ms = 0;
        let mut reject = Broker::default();
        assert_eq!(
            reject.update("conn", Source::Spotify, vec![stale.clone()], false, 6001),
            Broker::default()
                .try_update("conn", Source::Spotify, vec![stale], false, 6001)
                .is_ok()
        );
    }
    #[test]
    fn invalid_field_is_empty_exactly_when_validate_passes() {
        let mut empty_session = spotify();
        empty_session.session.clear();
        let mut long_title = spotify();
        long_title.title = "x".repeat(513);
        let mut too_many = spotify();
        let base = too_many.entries[0].clone();
        too_many.entries = (0..65)
            .map(|i| Entry {
                id: format!("q{i}"),
                uid: Some(format!("q{i}")),
                ..base.clone()
            })
            .collect();
        let mut duplicate = spotify();
        duplicate.entries.push(duplicate.entries[0].clone());
        let mut negative = spotify();
        negative.position_ms = -1;
        let cases = [
            ("", Source::Spotify, spotify()),
            ("session", Source::Spotify, empty_session),
            ("title_len", Source::Spotify, long_title),
            ("entries_over_64", Source::Spotify, too_many),
            ("entry", Source::Spotify, duplicate),
            ("position", Source::Spotify, negative),
            // A Spotify occurrence cannot satisfy the YouTube id binding.
            ("entry", Source::Youtube, spotify()),
        ];
        for (field, source, snapshot) in cases {
            assert_eq!(validate(source, &snapshot), field.is_empty());
            assert_eq!(invalid_field(source, &snapshot), field);
        }
    }
    fn tab(id: i64) -> TabInfo {
        TabInfo {
            id,
            title: format!("Tab {id}"),
            site: "example.com".into(),
            muted: false,
            window: 1,
        }
    }
    #[test]
    fn youtube_inventory_stores_bounded_tabs() {
        let mut b = Broker::default();
        let tabs = (0..8).map(tab).collect();
        assert!(b
            .try_update_tabs("youtube", Source::Youtube, vec![], tabs, false, 100)
            .is_ok());
        assert_eq!(b.clients["youtube"].tabs.len(), 8);
        assert_eq!(b.clients["youtube"].tabs[7].id, 7);
        // The text bounds are inclusive, so a row at each limit is accepted.
        let edge = TabInfo {
            id: 0,
            title: "t".repeat(512),
            site: "s".repeat(253),
            muted: true,
            window: 0,
        };
        assert!(b
            .try_update_tabs("youtube", Source::Youtube, vec![], vec![edge], false, 101)
            .is_ok());
    }
    #[test]
    fn rejects_every_invalid_tab_without_replacing_good_state() {
        let mut b = Broker::default();
        assert!(b
            .try_update_tabs("youtube", Source::Youtube, vec![], vec![tab(1)], false, 100)
            .is_ok());
        let blank = TabInfo {
            title: String::new(),
            site: String::new(),
            ..tab(2)
        };
        let long_title = TabInfo {
            title: "x".repeat(513),
            ..tab(3)
        };
        let long_site = TabInfo {
            site: "x".repeat(254),
            ..tab(4)
        };
        let negative_id = TabInfo { id: -1, ..tab(5) };
        let negative_window = TabInfo {
            window: -1,
            ..tab(6)
        };
        let too_many: Vec<_> = (0..9).map(tab).collect();
        for tabs in [
            vec![blank],
            vec![long_title],
            vec![long_site],
            vec![negative_id],
            vec![negative_window],
            too_many,
        ] {
            assert_eq!(
                b.try_update_tabs("youtube", Source::Youtube, vec![], tabs, false, 101),
                Err(Rejected::InvalidTab)
            );
        }
        // A refused inventory leaves the last good tab list in place.
        assert_eq!(b.clients["youtube"].received, 100);
        assert_eq!(b.clients["youtube"].tabs, vec![tab(1)]);
    }
    #[test]
    fn spotify_inventory_ignores_tabs() {
        let mut b = Broker::default();
        assert!(b
            .try_update_tabs(
                "spotify",
                Source::Spotify,
                vec![spotify()],
                (0..9).map(tab).collect(),
                false,
                100
            )
            .is_ok());
        assert!(b.clients["spotify"].tabs.is_empty());
    }
    #[test]
    fn a_tab_generation_resolves_only_while_the_client_is_fresh() {
        let mut b = Broker::default();
        assert!(b
            .try_update_tabs("youtube", Source::Youtube, vec![], vec![tab(7)], false, 100)
            .is_ok());
        let (client, found) = b.tab_for("youtube:tab:7", 101).unwrap();
        assert_eq!(client, "youtube");
        assert_eq!(found.id, 7);
        assert!(b.tab_for("youtube:tab:7", 6099).is_some());
        assert!(b.tab_for("youtube:tab:7", 6100).is_none());
        // An id the client did not list is not addressable, and neither is a
        // generation that resolves as a player.
        assert!(b.tab_for("youtube:tab:8", 101).is_none());
        assert!(b.tab_for("youtube:instance", 101).is_none());
        assert!(b.tab_for("youtube:tab:not-a-number", 101).is_none());
        let mut playing = youtube();
        playing.is_playing = true;
        assert!(b
            .try_update_tabs("youtube", Source::Youtube, vec![playing], vec![tab(9)], false, 102)
            .is_ok());
        assert!(b.tab_for("youtube:tab/document/player", 103).is_none());
    }
}
