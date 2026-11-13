# Disposable native deskband probe

This is feasibility code, excluded from the product. It uses frozen images from the existing CPU renderer and reads no credentials or live quota data. Read the [result](../../../verification/taskbar-focus/deskband-probe/RESULT.md) before re-running it.

From the repository root, in PowerShell:

```powershell
& docs/scratch/taskbar-focus/deskband-probe/build.ps1
& .debug-launch/deskband-probe/probe.exe selftest
```

`build.ps1` finds existing Visual C++ Build Tools and a Windows SDK. It installs nothing. Generated fixtures and binaries go to `.debug-launch/deskband-probe/`.

The live driver temporarily changes the taskbar. Before another run, archive the existing evidence and remove only the previous `.debug-launch/deskband-probe/finish.flag`. Start the controller with `probe.exe run`, then run:

```powershell
& docs/scratch/taskbar-focus/deskband-probe/run-live.ps1
```

The driver registers a unique per-user probe CLSID, verifies COM/category discovery, requests shell hosting and waits up to five minutes for observations. Windows displays a toolbar confirmation; `ShowDeskBand` success alone does not mean the band is hosted. Accept that prompt only when intentionally running the probe. Create `.debug-launch/deskband-probe/finish.flag` to finish early.

Its `finally` cleanup restores the original appbar state, closes the controller, hides/unregisters the probe and records final geometry and preservation hashes. Each child command has a twelve-second deadline. If registration already exists, it refuses to overwrite it. Explicit cleanup is available through `probe.exe hide` and `probe.exe unregister`; unregistration verifies the DLL path before removing the unique class key.

`probe.exe autohide-check` is an optional, temporary shell-setting test. Run it only under the outer live driver, which independently restores the original state if the test process fails. Never automatically restart Explorer for this experiment.

`probe.exe popup` requests a synthetic popup toggle and logs it as such. It does not replace a physical pointer/focus test. `probe.exe status` reads class rectangles and probe properties without collecting unrelated window titles or account data.

`band.cpp` paints static embedded raw pixels and posts bounded notifications. It has no credentials, quota readers, file/network I/O, worker threads or external waits in Explorer callbacks. `probe.cpp` owns registration, measurements, isolated checks and the disposable native popup/controller. The popup is a fixture of the existing Seam renderer, not the planned Focus implementation.
