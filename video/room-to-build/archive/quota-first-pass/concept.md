# Room to build

An original 60-second product film for Agent Quota inside Bloom.

## Hook

**How much room do you have left?**

One enormous quota ring emerges from darkness. The camera travels along its edge, then pulls back: the ring belongs to one agent among seven, all gathered in a single matte-black notch. The question becomes an immediately understandable product benefit: see your limits, resets, and model details without losing your place.

Closing thought: **Room to build.**

## Brief and decisions

- Audience: developers using more than one coding agent. Purpose: a polished launch/demo film.
- Product: Agent Quota, demonstrated in the current Bloom integration. Do not imply Bloom was created by this project or that the standalone has been removed.
- Mode: original autonomous creation. The user requested storyboard first, then full production. No Ink Press reskin.
- Master: 1920 x 1080, 30 fps, exactly 60 seconds, H.264/AAC stereo. Also deliver an SFX-only edition and editable source.
- Pipeline: Remotion + the actual local Bloom frontend captured at high resolution, original vector macro photography and 2.5D camera work, then FFmpeg audio mastering and encoding.
- The supplied Clawd prompt is a reference for ambition, originality, care, and QA. Its mascot, plot, text ban, and mandatory film-only 3D pipeline are not requirements for this product film. Use concise product copy and readable UI.
- Data: frozen fictional demonstration readings only. No accounts, desktop documents, credentials, live usage, or personal media.
- Audio: licensed downloaded music and foley from the Shotcraft asset library, with original source URLs and licenses retained. No audio synthesis in code.

## Visual direction

Selected: **Precision / breathing room.** An optical close-up gives way to a quiet interface. Matte black, soft warm white, authentic marks, and the product's usage colors. The backdrop is an original sculpted horizon, used as a stage rather than decoration. No stock device mockups, space particles, UI rain, logo spins, or invented features.

Alternatives considered: a dense fast-cut developer trailer would bury the calm product; a whimsical desktop-world film would divert attention from the real interface. Both rejected.

Tokens: surface #000000; selection #1c1c20; text #f4f2f6; secondary #b9b3c1; muted #a19aa9; green #32d74b; amber #ffb340; red #ff453a. Inter Regular/Medium/Semibold from the project, enlarged for titles. Warm-white stage #efeee9 is a restrained extension of the product neutrals. Quota colors retain their real meanings. Do not turn the UI into glass.

Motion: precise, deliberate, calm. Premium 42–48-frame camera arrivals, bezier(0.4,0,0.6,1) for substantial travel, no hand shake. One major move per shot; at least 30 frames of settled reading at each landing. Fine movements are meaningful interaction, never idle floating.

## Storyboard

| # | Time / frames | Picture and big idea | Camera / light | Copy | Sound / transition |
|---|---|---|---|---|---|
| 1 | 0–6 / 0–179 | A single authentic Codex quota ring in macro. The arc is both an object of curiosity and a measure of remaining space. | Low left-side grazing view resolves to a clear circle. Black stage, soft white edge light, one green arc. | How much room / do you have left? | Soft arrival and low restrained bed. Ring match cut into its UI slot. |
| 2 | 6–13 / 180–389 | Pull back to the actual seven-agent quota panel. Agent names, distinct rings, one surface. | Gentle left-to-front orbit; land front-on. Warm white stage contrasts the unchanged black UI. | Seven agents. / One clear view. | One broad air sweep, beat landing. No individual logo pings. |
| 3 | 13–20 / 390–599 | Select an agent; the real detail area responds. Five-hour and weekly windows are visible. | Cursor leads a measured push to selected detail. Black stage with neutral separation. | Every window. / In focus. | Real switch click at actual state change. Continuous spatial handoff. |
| 4 | 20–27 / 600–809 | Click reset display: countdown becomes a clock time. One action, two useful ways to plan. | Horizontal macro, level type, then settled hold. Warm white surround. | Know when / you're back. | Two quiet mechanical clicks. Matched framing, no fake countdown digits. |
| 5 | 27–34 / 810–1019 | Select Antigravity and expand the real model-group disclosure. | Downward camera reveal follows the actual expansion. Matte dark. | The detail / behind the number. | Dry tactile opening and a restrained sweep. |
| 6 | 34–41 / 1020–1229 | A staged 92% reading produces the real urgent quota alert. Clicking opens the relevant selected agent. | Alert close-up grows into overview. Red belongs only to the critical reading. | A heads-up. / Before the limit. | One low impact, no alarm or flashing. |
| 7 | 41–48 / 1230–1439 | Show a stale reading with last-good context, refresh, then a healthy snapshot. | Front-on for accuracy. A close crop keeps status and controls readable. | Refresh. / Keep your bearings. | Click, brief transition, soft landing on fresh data. |
| 8 | 48–54 / 1440–1619 | Close the panel; the quiet resting notch remains at the top of an original empty desktop. | Pull back to show the space returned to the user. Warm sculpted horizon. | Always there. / Out of your way. | Music eases; one soft close. |
| 9 | 54–60 / 1620–1799 | Agent Quota title, Inside Bloom, and the product at rest. The circular motif resolves beside it. Final image holds. | Locked symmetrical end frame, black and white. | Agent Quota / Inside Bloom. / Room to build. | Resolution, natural tail, no fade to a black dead frame. |

## Shotcraft mapping and references

- `spotlight-hero-card`: single-subject opening and side-lit depth. Adapt the authentic UI atom rather than copying the demo's paper/gold skin. Custom original ring macro is documented as a new shot, with framing and aliasing QA.
- `cursor-flyover`: cursor leads camera, click triggers a real captured UI state, then a settled reading hold. Use the tuned 2-frame cursor lead and 20-frame travel as a starting point, slowed for this product.
- `shot-transitions`: semantic match/portal transitions only where a quota ring or panel actually leads to its details. No gratuitous flash cuts.
- `PageCam`: high-resolution real textures and layout-space camera; use CSS layout zoom to preserve type sharpness.
- `promo-energy-arc`: slow opening, rising feature sequence, breathing room. Deliberate departure from Q8's crowded launch finale: this product promises quiet, so the final energy resolves into a calm still.
- Deliberate departures from C3/C2: product Inter instead of serif/highlighter annotations; concise benefit titles, with feature names demonstrated by the actual interface. No unrelated promotional skin.
- Apple reference: https://www.apple.com/macbook-pro/ — study restrained product hierarchy, material separation, macro-to-context scale. No Apple images, audio, hardware, copy, or shots reused.
- Shotcraft gallery: https://vincentwei1021.github.io/video-shotcraft/ — motion grammar, not borrowed branding.

## Acceptance

Actual source frontend in all UI shots, with frozen demo data. 60-second low-resolution full-film review before final render. Inspect a frame every two seconds and both sides of every transition. Check title/UI readability at final scale, no clipped footer or overlapping copy, no surprise blank frames, no unlicensed audio, no live data. FFprobe confirms dimensions, frame rate, duration, codecs and stereo. FFmpeg checks audio loudness/true peak, black frames, and frozen frames with planned holds distinguished. Independent clean-context creative/technical review before delivery.
