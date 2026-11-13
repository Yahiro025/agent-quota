//! Native artwork is cached separately from the connector-composed display.
/// Every playing player's card needs its own cover, so the cache keeps a few
/// tracks: one entry would evict the displayed player's art on every poll once
/// a second player is playing.
const ART_CACHE_ENTRIES: usize = 8;
#[derive(Default)]
pub struct NativeArtworkCache {
    entries: Vec<(String, String, Vec<String>)>,
}
impl NativeArtworkCache {
    pub fn get(&self, title: &str, artist: &str) -> Option<Vec<String>> {
        self.entries
            .iter()
            .find(|(t, a, _)| t == title && a == artist)
            .map(|(_, _, art)| art.clone())
    }
    /// The first image stored for a track is kept for as long as it plays, so
    /// only art the [`ArtworkGate`] accepted may be passed here.
    pub fn remember(&mut self, title: &str, artist: &str, artwork: Option<&[String]>) {
        if self.entries.iter().any(|(t,a,_)|t==title && a==artist) { return; }
        let Some(art) = artwork.filter(|art| !art.is_empty()) else {
            return;
        };
        if self.entries.len() >= ART_CACHE_ENTRIES {
            self.entries.remove(0);
        }
        self.entries.push((title.into(), artist.into(), art.to_vec()));
    }
}

/// The native metadata a cover belongs to.
#[derive(Clone, Default, PartialEq)]
pub struct ArtContext {
    pub title: String,
    pub artist: String,
    pub album: String,
    pub album_artist: String,
}
impl ArtContext {
    /// Identical cover bytes are expected for a replay, a duplicate queue entry,
    /// or another track of the same album. A bare album name is not enough:
    /// compilations from different artists share names but not covers.
    fn shares_cover_with(&self, other: &Self) -> bool {
        let owner = |c: &Self| -> String {
            if c.album_artist.is_empty() { c.artist.clone() } else { c.album_artist.clone() }
        };
        (self.title == other.title && self.artist == other.artist)
            || (!self.album.is_empty() && self.album == other.album && owner(self) == owner(other))
    }
}

/// How long an unexplained identical cover is withheld. Real thumbnails arrive
/// within a few seconds; after this the image is shown, but only provisionally.
pub const UNCONFIRMED_ART_MS: u64 = 10_000;

/// Browsers publish a new track's title before its thumbnail, so for a while
/// the new track arrives with the previous track's image. The gate withholds
/// that image until a different one arrives, unless the two tracks can share a
/// cover. Rarer legitimate repeats (a single and its album version share a
/// cover, podcasts carry no album) appear after [`UNCONFIRMED_ART_MS`], and
/// stay provisional so a thumbnail arriving even later still replaces them.
#[derive(Default)]
pub struct ArtworkGate {
    track: Option<String>,
    shown: Option<(Vec<String>, ArtContext)>,
    stale: Option<(Vec<String>, ArtContext, u64)>,
}
impl ArtworkGate {
    /// `track` is an identity that changes with every track change, including
    /// a switch between two entries with the same title.
    pub fn filter(
        &mut self,
        track: &str,
        context: &ArtContext,
        art: Option<Vec<String>>,
        now: u64,
    ) -> Option<Vec<String>> {
        if self.track.as_deref() != Some(track) {
            // Capture the cover at the switch, not the last emission: the first
            // poll after a switch usually has no image. A quick second switch
            // keeps the cover that is still being withheld.
            if let Some((shown, from)) = self.shown.take() {
                self.stale = Some((shown, from, now));
            }
            self.track = Some(track.into());
        }
        if let (Some((old, from, since)), Some(new)) = (&self.stale, &art) {
            if new != old || from.shares_cover_with(context) {
                self.stale = None;
            } else if now < since.saturating_add(UNCONFIRMED_ART_MS) {
                return None;
            }
        }
        if let Some(new) = &art {
            self.shown = Some((new.clone(), context.clone()));
        }
        art
    }
    /// A provisional image must not be cached: the cache never replaces an
    /// entry, so it would hide the real thumbnail if it arrives later.
    pub fn is_provisional(&self) -> bool {
        self.stale.is_some()
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    fn art(s: &str) -> Option<Vec<String>> {
        Some(vec![s.to_owned()])
    }
    fn video(title: &str) -> ArtContext {
        ArtContext { title: title.into(), artist: "Channel".into(), ..Default::default() }
    }
    fn song(title: &str, artist: &str, album: &str) -> ArtContext {
        ArtContext { title: title.into(), artist: artist.into(), album: album.into(), ..Default::default() }
    }
    const WAIT: u64 = UNCONFIRMED_ART_MS;
    /// One media poll, caching exactly as the media worker does.
    fn poll(
        gate: &mut ArtworkGate,
        cache: &mut NativeArtworkCache,
        track: &str,
        context: &ArtContext,
        art: Option<Vec<String>>,
        now: u64,
    ) -> Option<Vec<String>> {
        let shown = gate.filter(track, context, art, now);
        if !gate.is_provisional() {
            cache.remember(&context.title, &context.artist, shown.as_deref());
        }
        shown
    }
    #[test]
    fn previous_cover_is_withheld_until_the_new_one_arrives() {
        let mut gate = ArtworkGate::default();
        assert_eq!(gate.filter("tab1\nA", &video("A"), art("jazz"), 0), art("jazz"));
        // Native text has not caught up yet: the caller already passes None.
        assert_eq!(gate.filter("tab2\nB", &video("B"), None, 2000), None);
        // The browser swapped the title first but still serves the old thumbnail.
        assert_eq!(gate.filter("tab2\nB", &video("B"), art("jazz"), 4000), None);
        assert_eq!(gate.filter("tab2\nB", &video("B"), art("jazz"), WAIT), None);
        assert_eq!(gate.filter("tab2\nB", &video("B"), art("dev"), WAIT + 1000), art("dev"));
        assert!(!gate.is_provisional());
        assert_eq!(gate.filter("tab2\nB", &video("B"), art("dev"), WAIT + 3000), art("dev"));
    }
    #[test]
    fn a_thumbnail_later_than_the_wait_still_replaces_the_old_cover() {
        let (mut gate, mut cache) = (ArtworkGate::default(), NativeArtworkCache::default());
        poll(&mut gate, &mut cache, "n\nA", &video("A"), art("jazz"), 0);
        assert_eq!(poll(&mut gate, &mut cache, "n\nB", &video("B"), art("jazz"), 1), None);
        // Past the wait the image is shown, but provisionally and uncached.
        assert_eq!(poll(&mut gate, &mut cache, "n\nB", &video("B"), art("jazz"), WAIT + 1), art("jazz"));
        assert!(gate.is_provisional());
        assert!(cache.get("B", "Channel").is_none());
        assert_eq!(poll(&mut gate, &mut cache, "n\nB", &video("B"), art("dev"), WAIT + 9000), art("dev"));
        assert_eq!(cache.get("B", "Channel"), art("dev"));
    }
    #[test]
    fn withheld_art_is_never_cached_so_the_next_poll_reads_again() {
        let (mut gate, mut cache) = (ArtworkGate::default(), NativeArtworkCache::default());
        poll(&mut gate, &mut cache, "n\nA", &video("A"), art("jazz"), 0);
        assert_eq!(cache.get("A", "Channel"), art("jazz"));
        poll(&mut gate, &mut cache, "n\nB", &video("B"), art("jazz"), 2000);
        assert!(cache.get("B", "Channel").is_none());
        poll(&mut gate, &mut cache, "n\nB", &video("B"), art("dev"), 4000);
        assert_eq!(cache.get("B", "Channel"), art("dev"));
    }
    #[test]
    fn selecting_a_duplicate_entry_keeps_its_cover() {
        let mut gate = ArtworkGate::default();
        let zoo = video("Me at the zoo");
        gate.filter("g\n1:jNQXAC9IVRw", &zoo, art("zoo"), 0);
        assert_eq!(gate.filter("g\n4:jNQXAC9IVRw", &zoo, art("zoo"), 10), art("zoo"));
        assert!(!gate.is_provisional());
    }
    #[test]
    fn the_next_track_of_one_album_keeps_the_shared_cover() {
        let mut gate = ArtworkGate::default();
        gate.filter("s\n1", &song("One", "Band", "Album"), art("cover"), 0);
        assert_eq!(gate.filter("s\n2", &song("Two", "Band", "Album"), art("cover"), 10), art("cover"));
        let mut gate = ArtworkGate::default();
        let with_owner = |title: &str, artist: &str| ArtContext {
            album_artist: "Various".into(),
            ..song(title, artist, "Hits")
        };
        gate.filter("s\n1", &with_owner("One", "A"), art("cover"), 0);
        assert_eq!(gate.filter("s\n2", &with_owner("Two", "B"), art("cover"), 10), art("cover"));
    }
    #[test]
    fn an_album_name_from_another_artist_does_not_share_a_cover() {
        let mut gate = ArtworkGate::default();
        gate.filter("s\n1", &song("One", "Queen", "Greatest Hits"), art("queen"), 0);
        let abba = song("Two", "ABBA", "Greatest Hits");
        assert_eq!(gate.filter("s\n2", &abba, art("queen"), 10), None);
    }
    #[test]
    fn an_unexplained_identical_cover_appears_after_the_wait() {
        // A single followed by a track from the album that reuses its cover, or
        // podcast episodes without an album.
        let mut gate = ArtworkGate::default();
        gate.filter("n\nA", &song("Lead", "Band", "Lead - Single"), art("cover"), 0);
        let album = song("Other", "Band", "Album");
        assert_eq!(gate.filter("n\nB", &album, art("cover"), 10), None);
        assert_eq!(gate.filter("n\nB", &album, art("cover"), WAIT + 10), art("cover"));
    }
    #[test]
    fn different_cover_after_a_track_change_is_shown_at_once() {
        let mut gate = ArtworkGate::default();
        gate.filter("s\nA", &video("A"), art("x"), 0);
        assert_eq!(gate.filter("s\nB", &video("B"), art("y"), 10), art("y"));
    }
    #[test]
    fn unchanged_track_keeps_its_cover_across_polls() {
        let mut gate = ArtworkGate::default();
        for t in 0..10 {
            assert_eq!(gate.filter("s\nA", &video("A"), art("x"), t * WAIT), art("x"));
        }
        assert!(!gate.is_provisional());
    }
    #[test]
    fn stale_cover_survives_a_quick_second_switch() {
        let mut gate = ArtworkGate::default();
        gate.filter("s\nA", &video("A"), art("x"), 0);
        assert_eq!(gate.filter("s\nB", &video("B"), art("x"), 10), None);
        assert_eq!(gate.filter("s\nC", &video("C"), art("x"), 20), None);
        assert_eq!(gate.filter("s\nC", &video("C"), art("z"), 30), art("z"));
    }
    #[test]
    fn returning_to_the_previous_track_shows_its_cover() {
        let mut gate = ArtworkGate::default();
        gate.filter("s\nA", &video("A"), art("x"), 0);
        assert_eq!(gate.filter("s\nB", &video("B"), art("x"), 10), None);
        assert_eq!(gate.filter("s\nA2", &video("A"), art("x"), 20), art("x"));
    }
    #[test]
    fn delayed_native_metadata_fetches_new_art_after_connector_track_change() {
        let mut cache = NativeArtworkCache::default();
        let a = vec!["artA".to_owned()];
        let b = vec!["artB".to_owned()];
        cache.remember("A", "Artist", Some(&a));
        assert_eq!(cache.get("A", "Artist"), Some(a));
        // Connector B has no native image yet. A composed B/None emission must
        // never become a successful native cache hit when GSMTC catches up.
        cache.remember("B", "Artist", None);
        assert!(cache.get("B", "Artist").is_none());
        cache.remember("B", "Artist", Some(&b));
        assert_eq!(cache.get("B", "Artist"), Some(b));
        assert!(cache.get("B", "Other artist").is_none());
    }
    #[test]
    fn missing_native_thumbnail_remains_retryable() {
        let mut cache = NativeArtworkCache::default();
        cache.remember("B", "Artist", None);
        assert!(cache.get("B", "Artist").is_none());
    }
}
