# Seam — smoked graphite glass

The user selected Seam and delegated the material choice among glassmorphism, Liquid Glass, translucency and vibrancy. The chosen visual direction is dense smoked-graphite translucency, restrained foreground vibrancy and a fine neutral edge highlight.

[Open the refined visual](seam-smoked-glass.png).

This is a generated visual target, not a capture of a changed native app. The editor backdrop is illustrative and is not implementation source. The full-frame PNG is a reference for painting the widget, not an image to paste into its runtime.

## Pinned design

- Preserve the user's dropdown silhouette: full-width connection to a wider upper strip, small reverse-rounded concave shoulder joins, straight sides and softly rounded lower corners.
- Preserve the shallow seven-agent Seam layout, authentic marks, Inter, quota-basis labels and existing controls.
- Keep foreground text and marks opaque and crisp. Ambient color transmission belongs to the surface, not text opacity.
- Keep selection neutral; amber and green retain their usage meanings.
- Claude's Week70% is separate from its5h19% and five-hour reset in12m. No weekly reset is invented.
- Add depth through a fine edge and bounded offset shadow, rather than a thick silver bevel, colored halo or large refractive lens.

## Native material intent

The current Go/Win32 layered renderer provides premultiplied per-pixel alpha through UpdateLayeredWindow. A conservative CPU-painted translation can use a#17191f body tint at approximately92% opacity, a slightly more transmissive perimeter, a fine neutral highlight and the existing opaque foreground roles. A neutral selected lift at5% white keeps selection independent of amber severity. These are starting values for the native renderer, not measured properties of the generated image.

For token arithmetic only, the92%-tinted body over pure white becomes#2a2b31. The planned#a19aa9 muted text against that body is5.18:1. A5%-white selection lift becomes#35363b;#b9b3c1 secondary text against it is5.90:1. Actual native rendering, DPI, backgrounds and control states still need verification.

True backdrop blur and optical refraction are separate compositor features; lowering alpha does not implement either. Do not label this CPU material as Apple's Liquid Glass or introduce a GPU/dependency migration to imitate it. Preserve an opaque/high-contrast material fallback when transparency would reduce legibility.

## Guidance consulted

- [Apple: Materials](https://developer.apple.com/design/human-interface-guidelines/materials) — restrained use of Liquid Glass and denser regular material where background content could affect text legibility.
- [Microsoft: DWM_SYSTEMBACKDROP_TYPE](https://learn.microsoft.com/en-us/windows/win32/api/dwmapi/ne-dwmapi-dwm_systembackdrop_type) — native system backdrop categories; not proof of compatibility with this custom-shaped layered window.
- [Microsoft: System backdrops](https://learn.microsoft.com/en-us/windows/apps/develop/ui/system-backdrops) — distinguishes opaque Mica from semi-transparent Acrylic and its compositor requirements.

Date anchor:16October2026, Asia/Singapore. Generated with the built-in image tool, using the selected shape-refined Seam as the edit target. Inspection checked retained shape, all seven values/names/bases, the reset qualifier, controls and the material treatment. No native code changed in this visual-refinement pass.
