# Bloom — Quiet Instrument interaction proposal

Approved direction: Quiet Instrument aesthetic moodboard, selected by Bennett. This is a separate design study; it does not approve or implement changes in the running app.

## Material to behavior
| Reference | Product expression | Candidate building blocks |
| --- | --- | --- |
| Satin metal / hinge | Fine control boundaries, single restrained highlight, 80 ms press, 120 ms hover; precise sliders | Existing Settings controls, CSS state transitions, native range/button |
| Black woven cloth | Opaque matte black, soft tonal selection pad, sharp stationary labels | Existing useSlidingPad; measured movement, 220–240 ms settle |
| Water glass | Clear hierarchy and a crisp pale blue focus edge; stable hover detail with countdown and exact clock | Existing QuotaHoverCard; Lucide icons; CSS focus-visible |
| Limestone stair | Consistent 4/8 spacing rhythm; explicit hierarchy; inline disclosure rather than another floating layer | Existing detail rows and disclosure; 180–220 ms content settle |
| Sea / sky horizon | Stable aligned baselines and tabular numbers; horizontal quota bars update once | Existing flat quota row, transform scaleX 300 ms; immediate exact number |
| Daylight / paper / curtains | Warm off-white on a normal settings window; high contrast, quiet content replacement | Existing settings layout, Inter, Framer Motion AnimatePresence |

## Motion grammar
- Hover 120 ms, press 80 ms; focus appears immediately.
- Selection 220–240 ms with cubic-bezier(.22,1,.36,1), no overshoot.
- Enter / reveal 180–220 ms with up to 3–4 px travel; exit 120–140 ms.
- Updated usage bar 300 ms; update number immediately, no odometer or decorative wave.
- State replacement 140 ms. Never use animation completion to determine the real state.
- Existing controls remain interactive during transitions; new selection replaces the old destination.
- Reduced motion removes travel, interpolation, spinning and animation while preserving text/state feedback.
- No ambient motion. No continuous sheen, parallax, ornamental ripple, 3D tilt or spring bounce.
- Actual native window contour and input-region animation must remain synchronized; the main integration design owns that geometry.

## Semantics and invariants
- Operational notch and quota reading surfaces remain opaque matte black. Metal, glass, cloth, and paper are cues, not literal texture overlays.
- Use pale blue for focus/interaction; preserve green/amber/red usage thresholds and provider artwork.
- Keep Inter; tabular numerals for percentage, clock and resets.
- Flat quota rows: name and percent, thin usage bar, reset aligned right, footer “Just checked”; no global failure count.
- Failure / waiting / stale remain distinct; stale retains the last good value; unknown values are never invented zeros.
- No new activation, Escape, click-outside or global keyboard behavior is proposed for Bloom's focus-free surface.
- The keyboard controls in the independent HTML document do not prove native focus-free accessibility.
- Proposed normal-settings surface colors require contrast verification in the real settings window before adoption.

## Existing source / adoption ownership
- bloom/package.json: React, framer-motion, lucide-react are declared project dependencies. No new component or animation package is needed.
- bloom/src/quota/useSlidingPad.ts: measured selection pad with resize/initial-mount instant placement and reduced motion.
- bloom/src/quota/QuotaPanel.tsx: QuotaHoverCard, AnimatePresence, useReducedMotion, flat detail composition.
- bloom/src/quota/QuotaSpace.css: opaque quota surface, semantic data colors, selection animation.
- bloom/src/components/EdgeSurface.tsx and EdgeSurface.css: one contour rendering owner; preserve ownership.
- bloom/src/Settings.tsx and Settings.css: restyle existing controls rather than adding another settings component library.

## Library provenance and limits
UI/UX Pro Max is a searchable guidance library, not a downloadable UI component kit.
Verified matches:
- styles.csv: minimalism-and-swiss-style / Minimalism & Swiss Style.
- ux-guidelines.csv: Easing Functions, Reduced Motion, Excessive Motion.
- quick-reference.md: interruptible/cancellable transitions, immediate state feedback, tabular numerals, semantic colors.
The automatic system generator also returned landing-page scaffolding and unrelated font/color suggestions; those are rejected for this desktop surface. React stack searches returned no verified match after the one permitted retry; stack-specific mapping here comes from the actual Bloom source, not a fabricated database result.

The standalone preview uses native HTML/CSS/Web Animations to demonstrate the proposed feel without installing dependencies. In Bloom, reuse the existing Framer Motion and CSS equivalents. It does not constitute a product implementation or native interaction verification.



## Apply the same language across Bloom
| Surface | Treatment | Behavior |
| --- | --- | --- |
| Resting notch | Existing centered opaque-black housing; sharp Inter status text, clock inside | Preserve content-width housing and agent-mark cycling; main integration work owns shape and native hit geometry |
| Quota | Flat rows, semantic bars, tabular values, soft local selection | Stable positions during refresh; no count-up animation; preserve last good data and exact reset detail |
| Command Center / WiFi / Bluetooth | Same opaque surface and metal-style controls; clear selected / connecting / unavailable labels | Immediate press feedback; one local connection status; error and retry stay with the affected control |
| Volume / brightness | Fine monochrome tracks and restrained thumb; visible values | Pointer-tracking update immediately; do not ease behind the pointer or change the native setting on hover |
| Media | Keep real album art in its own colours; reuse transport buttons and typography | Immediate play/pause feedback; brief content crossfade on a track change; no ambient pulsing |
| Shelf / dock lists | Flat content, quiet tonal selection, clear drop destination | Retain current drag ownership; follow the pointer directly; use no theatrical pickup/landing animation |
| Settings | Proposed warm stone/paper background, dark ink, simple group dividers, same controls | Familiar keyboard controls, visible focus, explicit labels; reserve blue for interaction rather than health |
| Local alerts / empty states | Plain status and recovery action with room to read | State is visible immediately; optional short fade; never move the desktop or open another UI unexpectedly |

## Review evidence
- Standalone sampler rendered at 728 px and 390 px; no console messages in either render.
- Those are visual/layout checks, not native-window or operating-system interaction checks.
- Interactive samples are isolated simulations; no product files, settings, services, or dependency manifests changed.

