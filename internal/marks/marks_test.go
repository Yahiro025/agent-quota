package marks

import (
	"crypto/sha256"
	"testing"

	"agent-quota/internal/raster"
)

func TestDrawProvidesDistinctAgentMarksAtPillSize(t *testing.T) {
	ids := []string{"claude", "kiro", "codex", "cursor", "copilot", "devin", "antigravity"}
	seen := map[[32]byte]string{}
	for _, id := range ids {
		c := raster.New(24, 24)
		Draw(c, id, 5, 5, 14)
		ink, antialiased := 0, false
		for i := 3; i < len(c.Img.Pix); i += 4 {
			a := c.Img.Pix[i]
			if a != 0 {
				ink++
				antialiased = antialiased || a < 255
			}
		}
		if ink < 80 {
			t.Errorf("%s rendered only %d nontransparent pixels", id, ink)
		}
		if !antialiased {
			t.Errorf("%s has no antialiased tile edge", id)
		}
		digest := sha256.Sum256(c.Img.Pix)
		if other, ok := seen[digest]; ok {
			t.Errorf("%s rendered the same pixels as %s", id, other)
		}
		seen[digest] = id
	}
}
