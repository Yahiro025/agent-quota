# Agent Quota installation, identity, and motion

## Outcome and approval scope

People install Agent Quota once, find it in Start, and recognize its original app icon. The B2 measured-aperture mark communicates clarity and control. Its motion feels mechanical and deliberate.

The user confirmed public distribution, preservation of the existing widget design, B2 as the logo direction, a tougher appearance, and animation in both brand presentation and the running widget. This document proposes the remaining concrete behavior for review. It does not authorize implementation until the user approves this written design.

## Identity

B2 has four substantial sections around a square opening, sharp chamfers, diagonal seams, and an offset locking detail at the upper right. Preserve those features when constructing editable vector artwork. The generated board is a visual reference, not a production asset or proof of icon readability.

The reference is `C:/Users/Bennett/.codex/generated_images/01a13bdf-b916-7951-971b-43d9eeba9cd6/exec-ef3f3ae0-e143-45b2-b160-286df9b2f139.png`, middle column B2.

Use graphite `#131116`, white `#FFFFFF`, and ice blue `#8BB8FF`. Keep green, amber, and red reserved for quota state. Preserve Inter in the widget. Use Source Sans 3 Semibold for the packaging wordmark after checking the actual font file and retaining its OFL notice.

Deliver an editable SVG mark, transparent PNGs, a monochrome version, a wordmark lockup, and a Windows ICO containing 16, 20, 24, 32, 40, 48, 64, 128, and 256 pixel images. Small versions may widen seams for optical clarity but must preserve the B2 silhouette. The executable and shell shortcuts use the static locked mark.

## Motion

Use one normalized four-section geometry for both the static mark and animated poses. The sections separate slightly, turn in sequence, and return. The offset corner finishes the lock. The final pose matches the static logo exactly. Motion must not resemble a loading indicator or suggest that quota polling is active.

The brand cycle lasts four seconds. Movement occupies the first 1.2 seconds, followed by a 2.8-second hold. Produce a silent square Hyperframes preview on graphite, plus an export suitable for reviewing the motion. Do not add sound, glow, flashes, or moving wordmark text.

The widget uses a restrained six-second cycle. Movement occupies 0.9 seconds, followed by a 5.1-second hold. Place a 20-logical-pixel mark at the right of the inspector header, with a 16-pixel right inset. Preserve the existing `Usage limits` title, header height, quota rows, pill geometry, and hit targets. The closed pill does not gain another icon.

Start the widget sequence when the inspector opens, whether by hover, click, or reveal from another launch. Repeat while the inspector remains open. Closing the inspector cancels animation scheduling and resets to the locked pose. Repainting during movement targets 30 frames per second. During the hold, schedule the next transition rather than running a frame timer.

Respect Windows client-area animation preferences. If animations are disabled, or the preference cannot be read, show the static mark. Recheck on settings changes. The installer follows the same preference. The preview offers a static reduced-motion state.

## Installation and upgrades

Use Inno Setup to package the Go executable without adding a Go runtime dependency. It provides the per-user installation, shortcuts, and uninstall flow instead of making the app maintain its own installer.

Produce `AgentQuotaSetup.exe`. Install under `%LOCALAPPDATA%\Programs\AgentQuota` without elevation. Create a Start menu entry named `Agent Quota` and an Installed Apps uninstall entry. Offer an unchecked desktop shortcut option, an unchecked `Start with Windows` option, and a checked launch-after-install option. Preserve an existing autostart choice on upgrade.

Keep settings under `%APPDATA%\agent-quota`. Upgrades replace installed application files while preserving settings and Claude snapshot data. If an installed app is running, ask the user to close it and retry before replacing files. Do not terminate unrelated agents or remove the development executable.

Show the B2 motion in a bounded installer artwork area using packaged frames. Stop its timer when that page is hidden or setup exits. Preload frames before playback. Animate artwork without altering progress indicators, installer controls, or keyboard interaction. Packaging must not introduce a browser runtime into the installed app.

Uninstall removes installed binaries, shortcuts, and the startup entry when it targets this installed copy. Keep user support data by default. Do not alter coding-agent credentials or Claude settings during install, upgrade, or uninstall.

Package version metadata and third-party notices with each release. Signing is an explicit build input, not an assumed credential. Label unsigned artifacts as unsigned in distribution instructions. Publishing and certificate purchases remain separate actions.

## Ownership and data flow

Keep the Win32 layered window and CPU renderer. `widget.Render(View)` receives an explicit normalized logo pose and remains deterministic, with no clock or timer reads. Static screenshots use the locked pose unless a test explicitly supplies another pose.

The application owns animation start times and visible-state transitions. The window package owns native timer scheduling and the Windows animation preference boundary. All scheduling and painting run on the existing window thread. Quota readers, polling deadlines, and refresh behavior remain independent of logo motion.

Store canonical logo geometry and pose rules in a small brand module. Export the same approved geometry and timing for the Hyperframes preview and installer frames. Avoid separately hand-drawing three versions of the mark.

`DESIGN.md` remains authoritative for widget appearance. After approval, extend it with the identity and motion rules and correct its existing invalid group-valued typography reference. Do not replace the design system or add a splash screen.

## Verification and acceptance

Inspect actual-size icons on light and dark backgrounds at each shipped size. Check the wordmark spelling, monochrome silhouette, seam visibility, and locked-pose agreement across outputs. Compare against the supported agents' marks and the researched developer-tool references. This is a design similarity check, not trademark clearance.

Verify the preview at opening, maximum displacement, locking, and the loop boundary. Verify reduced-motion output separately.

Test widget frames with explicit poses and timestamps. Exercise hover, pin, reveal, close, drag, DPI changes, and animation-preference changes. Confirm timer cancellation when closed and no recurring frame timer during holds. Run the native app to verify focus-free behavior and inspect screenshots at fractional DPI.

Verify fresh installation, Start launch, optional shortcuts, both autostart choices, repeated setup, upgrade with preserved settings, a running-app replacement conflict, and uninstall. Confirm the installed copy works independently of the project directory and the target machine needs no Go installation.

Run `gofmt -l .`, `go vet ./...`, `go test ./...`, and `go run . --shot DIR` for product changes. Run the packaging build and exercise the actual setup executable. A successful compiler run alone does not verify installation or animation.

## Sources

- Current widget header and pure renderer in `widget/widget.go`.
- Current visibility transitions in `main.go` and native timers in `internal/win/win.go`.
- [Inno Setup capabilities](https://jrsoftware.org/isinfo.php).
- [Windows client-area animation preference](https://learn.microsoft.com/en-us/windows/win32/winauto/client-area-animation).
- [Source Sans license](https://github.com/adobe-fonts/source-sans/blob/release/LICENSE.md).
- [Raycast brand asset roles](https://www.raycast.com/press).
