// Single command for live desktop development: frees the notch from the
// installed Bloom (single-instance mutex), then hands over to `tauri dev`
// (Vite HMR for the UI, cargo watch rebuild + relaunch for Rust).
import { spawn, spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const installed = path.join(process.env.LOCALAPPDATA || "", "bloom", "bloom.exe");

function ps(script) {
	const r = spawnSync("powershell", ["-NoProfile", "-Command", script], { encoding: "utf8" });
	return (r.stdout || "").trim();
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const pids = ps(
	`Get-Process bloom -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq '${installed}' } | ForEach-Object { $_.Id }`
)
	.split(/\s+/)
	.filter(Boolean);

if (pids.length > 0) {
	console.log(`dev: closing installed Bloom (${pids.join(", ")}) so the dev instance can own the notch`);
	ps(`Add-Type @'
using System; using System.Runtime.InteropServices;
public class DevClose {
  delegate bool P(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] static extern bool EnumWindows(P cb, IntPtr l);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  public static void Close(uint target) {
    EnumWindows((h, l) => { uint pid; GetWindowThreadProcessId(h, out pid);
      if (pid == target && IsWindowVisible(h)) PostMessage(h, 0x0010, IntPtr.Zero, IntPtr.Zero);
      return true; }, IntPtr.Zero);
  }
}
'@; foreach ($p in @(${pids.join(",")})) { [DevClose]::Close([uint32]$p) }`);

	let alive = true;
	for (let i = 0; i < 24 && alive; i++) {
		await sleep(500);
		alive = ps(`@(Get-Process -Id ${pids.join(",")} -ErrorAction SilentlyContinue).Count`) !== "0";
	}
	if (alive) {
		// Older builds deadlock in webview teardown after restoring the taskbar.
		// Killing that zombie is safe only once the taskbar is back; otherwise
		// leave it to the tray so the desktop never loses its taskbar.
		const taskbar = ps(`Add-Type @'
using System; using System.Runtime.InteropServices;
public class DevTaskbar {
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindow(string c, string w);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
}
'@; [DevTaskbar]::IsWindowVisible([DevTaskbar]::FindWindow('Shell_TrayWnd', $null))`);
		if (taskbar !== "True") {
			console.error("dev: installed Bloom did not exit and the taskbar is still hidden; quit Bloom from its tray icon and retry");
			process.exit(1);
		}
		console.log("dev: teardown hung after taskbar restore; killing the zombie process");
		for (const pid of pids) spawnSync("taskkill", ["/PID", pid, "/F"]);
		await sleep(1000);
	}
}

const cli = path.join(root, "node_modules", "@tauri-apps", "cli", "tauri.js");
const child = spawn(process.execPath, [cli, "dev"], { cwd: root, stdio: "inherit" });
child.on("exit", (code) => process.exit(code ?? 0));