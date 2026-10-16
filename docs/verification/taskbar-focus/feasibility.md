# Taskbar Focus feasibility

Date: 2026-10-17. Outcome: **FAIL under the approved constraints.**

This is a failed implementation prerequisite, not a completed Focus feature. Product code and the installed executable were not changed. Tasks 2–5 have not started.

## Live machine evidence

Windows 10 Pro 22H2, build 19045.6466. The read-only [diagnostic](../../scratch/taskbar-focus/inspect.go) queries taskbar class rectangles and appbar state; it does not read window titles, change shell layout, or register an extension. Reproduce from the repository root:

```powershell
go run docs/scratch/taskbar-focus/inspect.go
```

The captured [geometry.json](geometry.json) records physical pixels:

| Region | Left | Top | Right | Bottom |
| --- | ---: | ---: | ---: | ---: |
| Primary work area | 0 | 0 | 1920 | 1150 |
| Taskbar | 0 | 1150 | 1920 | 1200 |
| Taskbar rebar | 118 | 1150 | 1545 | 1200 |
| Task-button container | 120 | 1150 | 1545 | 1200 |
| Notification region | 1545 | 1150 | 1920 | 1200 |
| Clock | 1770 | 1150 | 1854 | 1200 |
| Show Desktop | 1914 | 1150 | 1920 | 1200 |

Auto-hide is currently off. The task-button container meets the notification region at x=1545: **zero pixels of unallocated space between these containers**. A visually empty portion of the task-button container remains Explorer-owned; these rectangles do not prove it can be safely reserved for an external widget.

## API assessment

| Candidate | Finding | Constraint conflict |
| --- | --- | --- |
| Notification icon | Accepts an icon and tooltip, without an arbitrary two-line content rectangle. | Cannot preserve the specified persistent readout. |
| Desktop appbar | Negotiates desktop-edge space around the taskbar and other appbars. | Reserves a separate region, not an inline slot beside the tray. |
| Deskband | Custom taskbar content uses a COM object registered as an in-process server. | Requires an Explorer-loaded extension, excluded by the spec. Current documentation also marks deskbands unsupported from Windows 7 onward. |
| External overlay or reparented child window | No verified shell contract found that allocates space to it. | Drawing over the task-button container does not reserve that area. Changing its private layout was explicitly excluded. |
| Taskbar button APIs | Overlays, progress, and thumbnail controls attach to application task buttons. | Do not allocate the requested tray-adjacent readout. |

Primary references checked during this run:

- [NOTIFYICONDATAW](https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-notifyicondataw): icon and tooltip fields.
- [Application desktop toolbars](https://learn.microsoft.com/en-us/windows/win32/shell/application-desktop-toolbars): appbar position negotiation and taskbar coexistence.
- [Band objects](https://learn.microsoft.com/en-us/windows/win32/shell/band-objects): taskbar deskband placement and in-process registration.
- [IDeskBand](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nn-shobjidl_core-ideskband): current support warning.
- [ITaskbarList3](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nn-shobjidl_core-itaskbarlist3): task-button integration scope.

These findings establish **no verified supported route for the combination of constraints**, rather than proving every possible Windows technique impossible. An independent architecture review reached the same conclusion.

## Verification limits

No disposable overlay was launched because no candidate passed the space-allocation prerequisite. Relayout, crowding, auto-hide transitions, Explorer restart, and native Focus interaction therefore remain unverified. Explorer was not restarted and taskbar settings were not changed. Formatting of the diagnostic was checked; product tests were not rerun because no product code changed.

## Concrete revision options

1. **Preserve the current runtime constraints:** retain the full compact readout just above the taskbar, snap it to the tray-side edge on drag release, and open Focus above it. Reuse the existing Go layered window and renderer. This changes the approved requirement from *inside* the taskbar to *immediately above* it; it does not allocate a new desktop strip.
2. **Prioritize true Windows 10 taskbar placement:** investigate a small native deskband bridge that Explorer loads, while the Go app retains quota readers and the Focus popup. This requires relaxing the out-of-process/no-Explorer-DLL constraint and validating a legacy extension path on this machine. Native registration, bridge transport, deployment, and cleanup need a revised architecture; availability must not be promised before a compatibility probe. It is not the currently documented supported route.

The user's always-visible rings, percentage, and countdown remain required in either option. A plain tray icon with hidden details is not proposed as a substitute.

## Plan disposition

Task 1 returned FAIL. Its approved gate explicitly stops Tasks 2–5 and requires review of a revised integration option. Do not install a misleading overlay, build a Focus-only partial feature, or mark the original plan complete.
