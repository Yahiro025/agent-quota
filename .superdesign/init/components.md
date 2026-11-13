# Components

Shared UI for the Bloom notch quota glance. Custom React, no component library. Vanilla CSS.

## QuotaRingMark
Path: bloom/src/quota/QuotaPanel.tsx
28px dual ring with the agent brand mark centered. Props: `{ a: QuotaAgent }`.

## QuotaHoverCard
Path: bloom/src/quota/QuotaPanel.tsx
Opaque black card. Title is the agent name, optional plan, and a trailing place marker (`2 of 7`). Rows are label, thin usage bar, percent, reset on the right. Props: `{ a: QuotaAgent; place?: string }`.

## QuotaPanel
Path: bloom/src/quota/QuotaPanel.tsx
Open quota space: Used header, pin, close, ring row, flat detail rows, footer Just checked.
