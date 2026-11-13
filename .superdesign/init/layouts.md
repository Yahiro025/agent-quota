# Layouts

## Notch shell
Path: bloom/src/App.tsx and bloom/src/App.css
Centered black housing at the top of the primary monitor, only as wide as its contents, flush to the top edge, 18px bottom radius. Resting order left to right: music waveform, agent mark, weather, clock, battery, one live-state dot. Clicking the mark cycles agents. Clicking the pill opens the last panel. The clock stays in the pill.

## Painted surface
Path: bloom/src/components/EdgeSurface.tsx
Opaque black fill for the housing. The desktop around it is not a Bloom-owned menu bar.
