# Left and Right agent selection

With the pointer over the open panel, press Left or Right to select the previous or next visible agent. Selection wraps at both ends of the displayed row. Each press advances once. Holding a key does not repeatedly rotate the selection. Failed agents remain selectable when displayed.

The panel retains its open and pinned state. A changed selection resets detail scrolling and disclosures, then saves the selected agent. Modified arrows and arrows outside the panel keep their normal behavior. The widget remains focus-free.

## Implementation choices

The navigation data is the displayed item list, effective selected ID, and direction. `navigation.go` owns the selection transition shared by mouse, menu, and keyboard callers. It uses the rendered selection when a hidden saved selection has fallen back to the first displayed agent.

| Input design | Decision |
|---|---|
| Activate the widget and handle ordinary keyboard messages | Rejected because activation changes the existing focus-free behavior. |
| Register bare arrows as temporary global hotkeys | Rejected because a delayed unregister can reserve arrows outside the widget. |
| Dedicated low-level keyboard hook with scoped capture | Selected because it can pass each complete key sequence while the foreground window retains focus. |

The hook thread owns each arrow's Up, Passed, or Captured state. A key first pressed outside the widget remains passed until release. A captured key remains consumed until release, including after pointer departure. The hook posts selection commands to the UI thread. It never renders, saves settings, or reads providers. Commands include an opening generation so a closed panel cannot leave stale commands for a later opening.

The UI thread publishes an immutable alpha mask after successful layered-window presentation. The hook checks that mask, current visibility, pointer position, occlusion, modifiers, and menu state. A first review found that `WindowFromPoint` alone did not enforce the widget contour across threads. The mask fixes that difference. Windows behavior is described in [Microsoft's hit-testing explanation](https://devblogs.microsoft.com/oldnewthing/20101230-00/?p=11873). Hook threading and return behavior follow [LowLevelKeyboardProc](https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelkeyboardproc).

## Verification

The original selection regression failed because the app had no arrow-navigation handler. The implemented tests check both directions, both wraps, a failed agent, hidden-agent fallback, empty and single-item rows, closed panels, dragging, persistence, disclosure reset, and pin preservation.

The pure ownership test checks repeats, release after departure, entry while holding a passed key, modifier eligibility, and unrelated keys. Native desktop tests are opt-in because pointer position and occlusion are live state.

The native fixture installs a downstream receiver hook before the production hook. It injects uniquely tagged test keys through Windows `SendInput`. The receiver consumes any passed test event, so the foreground application receives none of the test keys. The fixture checks actual Windows hook invocation, one step per press, captured repeat and release suppression, passed hold-entry, captured hold-departure, shadow pixels, disabled scope, and unchanged foreground HWND. A separate native test checks hidden windows, stale commands, menu pause, cross-thread scope, actual UI command delivery, and hook cleanup.

Removing the hit-mask check through a Go build overlay produces an extra selection over the shadow. The native fixture fails with `[1 -1 1]` instead of `[1 -1]`. Restoring the mask passes. Production source was not changed for the mutation run. See [shadow-red.txt](shadow-red.txt) and [native-hook.txt](native-hook.txt).

Run the checks from the repository root.

```powershell
gofmt -l .
go vet ./...
go test ./... -p 1 -count=1 -timeout 90s
$env:AGENTQUOTA_KEYBOARD_LIVE_TEST='1'
go test ./internal/win -run 'TestLiveAgentKeyboardHookDelivery|TestAgentKeyboardNative' -count=1 -v -timeout 30s
Remove-Item Env:AGENTQUOTA_KEYBOARD_LIVE_TEST
```

The package suite runs serially because several existing native tests create topmost windows. An early parallel run occluded the pointer-sensitive fixture. Both new desktop tests now require the explicit opt-in. [go-tests.txt](go-tests.txt) records the final ordinary suite. The controlled native input test is direct keyboard-path evidence. It does not represent a human pressing physical keys.

## Ownership and review

Model the Domain shaped the displayed-order selection and explicit key ownership states. Boundary Discipline kept Windows input in `internal/win` and app state in `navigation.go`. Prove It Works required actual Windows hook delivery and a failing shadow mutation. Laziness Protocol kept the existing renderer and native deskband unchanged.

The throughput checkpoint is complete. Input exploration and architecture review ran independently as read-only work. The main agent implemented all changes under AGENTS.md. The UI thread owns selection, window state, and persistence. The hook thread owns held-key state. Atomic scope and mask values cross that boundary. One implementation owner kept those rules consistent.

Independent critical review identified the shadow test gap and desktop-test isolation issue. Both were corrected and checked. The scoped additions contain no new explanatory comments. No product decision remains open.

## Installation

Both the installed executable and the root `agentquota.exe` use the verified build. [final-install.json](final-install.json) records hashes and running paths. The existing taskbar host remains `68843005`. `--taskbar-status` reports `agent_keys_ready` from the running widget's native window. Previous binaries and settings are backed up under `.debug-launch/keyboard-navigation/`.

The original checkout remains authoritative. The dirty native work is preserved. No broad staging, reset, clean, commit, or PR was performed for this local feature.
