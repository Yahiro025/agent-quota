package quota

import (
	"testing"
	"time"
)

func TestPollingUsesThirtySecondsForLocalAPIAndFiveMinutesForSlowProviders(t *testing.T) {
	b := NewBook()
	now := time.Unix(1_000, 0)
	on := ids(ItemCodex, ItemCursor, ItemCopilot, ItemDevin, ItemClaude, ItemKiro, ItemAntigravity, ItemCommandCode, ItemQoder)
	cheap := on[:4]
	started(t, b, on, false, now, false, on)
	for _, id := range on {
		b.Finished(id, newReading(10, "ok"), true, now.Add(5*time.Second))
	}
	started(t, b, on, false, now.Add(29*time.Second), false, nil)
	started(t, b, on, false, now.Add(30*time.Second), false, cheap)
	for _, id := range cheap {
		b.Finished(id, Fail("temporary failure"), true, now.Add(35*time.Second))
	}
	started(t, b, on, false, now.Add(60*time.Second), false, cheap)
	for _, id := range cheap {
		b.Finished(id, newReading(10, "ok"), true, now.Add(65*time.Second))
	}
	started(t, b, on, false, now.Add(299*time.Second), false, cheap)
	for _, id := range cheap {
		b.Finished(id, newReading(10, "ok"), true, now.Add(300*time.Second))
	}
	started(t, b, on, false, now.Add(300*time.Second), false, ids(ItemClaude, ItemKiro, ItemAntigravity, ItemCommandCode, ItemQoder))
	started(t, b, on, true, now.Add(301*time.Second), false, cheap)
	started(t, b, on, true, now.Add(302*time.Second), false, nil)
}

func TestDisablingAndReenablingProviderDoesNotRetainItsDueTime(t *testing.T) {
	b := NewBook()
	now := time.Unix(1_000, 0)
	started(t, b, ids(ItemKiro), false, now, false, ids(ItemKiro))
	b.Finished(ItemKiro, newReading(1, "ok"), true, now.Add(time.Second))
	started(t, b, nil, false, now.Add(2*time.Second), true, nil)
	started(t, b, ids(ItemKiro), false, now.Add(3*time.Second), false, ids(ItemKiro))
}
