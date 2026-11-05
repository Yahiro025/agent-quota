package main

// The approved Seam design moves the former corner widget to top-centre once,
// then lifts it to the monitor's top edge once more so the notch can sit over a
// top appbar such as MyDockFinder. Each step clears only the saved anchor:
// later launches preserve drag positions and reader preferences as usual.
func migrateSeamSettings(s settings) settings {
	if s.LayoutVersion < 1 {
		s.Compact = false
	}
	if s.LayoutVersion < 2 {
		// A version 1 anchor sits at work.Min.Y (y = 30 under MyDockFinder).
		// Dropping it re-snaps Seam to y = 0 while Compact, Selected, Visible
		// and Docked survive untouched.
		s.HasPos = false
		s.LayoutVersion = 2
	}
	return s
}
