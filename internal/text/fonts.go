package text

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
)

// Keep the widget independent of installed fonts so its text metrics do not vary
// between machines.
var (
	//go:embed assets/Inter-Regular.ttf
	interRegular []byte
	//go:embed assets/Inter-Medium.ttf
	interMedium []byte
	//go:embed assets/Inter-SemiBold.ttf
	interSemiBold []byte
)

type faceKey struct {
	font   *font.Font
	weight float32
}

type fonts struct {
	map_  *fontscan.FontMap
	faces map[faceKey]*font.Face
}

func newFonts() *fonts {
	f := &fonts{map_: fontscan.NewFontMap(nil), faces: make(map[faceKey]*font.Face)}
	for _, item := range []struct {
		name string
		data []byte
	}{
		{"Inter Regular", interRegular},
		{"Inter Medium", interMedium},
		{"Inter SemiBold", interSemiBold},
	} {
		if err := f.map_.AddFont(bytes.NewReader(item.data), item.name, "Inter"); err != nil {
			panic(fmt.Sprintf("agent-quota: load bundled %s font: %v", item.name, err))
		}
	}
	return f
}

var wght = opentype.MustNewTag("wght")

func (f *fonts) face(r rune, weight float32) *font.Face {
	f.map_.SetQuery(fontscan.Query{
		Families: []string{"Inter"},
		Aspect:   font.Aspect{Style: font.StyleNormal, Weight: font.Weight(weight), Stretch: font.StretchNormal},
	})
	got := f.map_.ResolveFace(r)
	if got == nil {
		return nil
	}
	k := faceKey{font: got.Font, weight: weight}
	if cached := f.faces[k]; cached != nil {
		return cached
	}
	face := font.NewFace(got.Font)
	face.SetVariations([]font.Variation{{Tag: wght, Value: weight}})
	if len(face.Coords()) == 0 {
		face = got
	}
	f.faces[k] = face
	return face
}
