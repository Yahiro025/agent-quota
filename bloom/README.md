<div align="center">

<img 
  src="https://github.com/user-attachments/assets/962887ec-636b-4e0f-90eb-0862c0feefca" 
  width="72"
/>

# Bloom

<br/>

<!-- HERO SHOWCASE — full-width cinematic shot or video -->
<!-- ![Hero](your-hero-url) -->
<img width="1920" height="1080" alt="Bloom Hero" src="https://github.com/user-attachments/assets/22041f25-a69e-457d-80d9-7dfbfbed2d29" />

<video width="1920" height="1080" src="https://github.com/user-attachments/assets/fa7f949b-d9c3-4a92-b00d-e1132c0f3ff1">

</div>

---

Bloom makes your Windows desktop feel alive.

Every transition is a physics simulation.
Every element responds to touch.
Every pixel is in motion.

Your desktop has been asleep for years.
This wakes it up.

---

## The Island

<!-- SHOWCASE: GIF or short clip — Island expanding, cycling through modes (3-5s) -->
<!-- ![Island Demo](your-island-gif-url) -->

<p align="center">
  <img width="430" height="70" alt="Bloom Island" src="https://github.com/user-attachments/assets/0d723558-9df4-4214-b20e-4a1f97eb1f22" />
</p>

A notch at the top of your screen that adapts to what you're doing.

Scroll or swipe to switch modes.
Watch it transform.

**Music** — album art, track info, playback controls. A visualizer that reacts to five frequency bands with spring physics. It moves when the music plays.

<!-- SHOWCASE: GIF — music mode reacting to a song -->
<!-- ![Music Visualizer](your-music-gif-url) -->

**Status** — CPU, disk, and network stats. Your desktop, summarized.

**Timer** — A Pomodoro timer. Focus without switching apps.

Each transition is spring-loaded.
Width, height, border-radius, position — all animate independently.
It feels mechanical. In a good way.

---

## Under the Hood

<!-- SHOWCASE: Optional — architecture diagram or visual of the 5-window system -->
<!-- ![Architecture](your-arch-url) -->

A Rust backend that speaks directly to the Windows shell.

Global hooks intercepting keys before Windows sees them.
WASAPI capturing system audio in real-time.
COM controlling your media sessions.
WMI monitoring your hardware.

The whole thing sleeps when you don't need it.
The audio visualizer pauses when nothing's playing.
The cursor monitor hides when the dock is gone.
Thumbnails only refresh on focus.

It's fast because it has to be.

---

<!-- SHOWCASE: Full-width cinematic video or GIF montage -->
<!-- ![Bloom Montage](your-montage-url) -->

---

## 🛡️ Microsoft Defender

Some users may see a Microsoft Defender warning when installing Bloom.

The executable was submitted directly to Microsoft for analysis. Microsoft reviewed the file and confirmed that it **does not meet their criteria for malware or potentially unwanted applications**, and **the detection has been removed**.

<p align="center">
  <img width="800" alt="Microsoft Security Intelligence" src="https://github.com/user-attachments/assets/67ddddde-9c68-4338-a0f2-c5645e416514" />
</p>

<details>
<summary>Still seeing the detection?</summary>

Your system may still have the previous Defender signature cached. Microsoft recommends updating your Defender security intelligence.

Open **Command Prompt as Administrator** and run:

```cmd
cd "C:\Program Files\Windows Defender"
MpCmdRun.exe -removedefinitions -dynamicsignatures
MpCmdRun.exe -SignatureUpdate
```

</details>

## Get It Running

**Download** the latest build from [Releases](https://github.com/SehajveerSingh2005/bloom/releases/latest).

Or build from source:

```bash
git clone https://github.com/SehajveerSingh2005/bloom.git
cd bloom
bun install
bun run tauri dev
```

You'll need [Rust](https://rustup.rs/) and [Bun](https://bun.sh/). That's it.

---

## Development

One command runs the real desktop app with live reload:

```bash
bun run dev:app   # or: npm run dev:app
```

It starts Vite (HMR) and `tauri dev`, which builds the Rust side in debug mode and
launches the actual notch window — not a browser preview. Because Bloom is
single-instance, the command first closes an installed Bloom gracefully; if that
cannot be done safely it asks you to quit Bloom from its tray icon and retry.

What updates while it runs:

- **React components and CSS** — hot-swapped in the running window, state kept
  where React Refresh can preserve it. No rebuild, no restart.
- **Other frontend modules** — the webview reloads itself; window state resets.
- **Rust (`src-tauri`)** — cargo rebuilds incrementally and relaunches the app
  automatically. Native state resets; the quota helper is respawned.

Stop with `Ctrl+C` in the dev terminal (or the tray's Quit). Both restore the
Windows taskbar, which Bloom hides while it runs.

Development never packages or installs anything. Release builds stay separate:
`bun run build` for the frontend, `bun run tauri build` / `bun run release` for
installers. The dev instance also skips autostart registration so your login
entry keeps pointing at the installed build.

---

## Contributing

Bloom is open source.
Found a bug? Open an issue.
Have an idea? Open a feature request first — we'll agree on the approach before you write code. See [CONTRIBUTING.md](CONTRIBUTING.md).
Want to just say it's cool? A star goes a long way.

Licensed under [GPLv3](LICENSE).

---

<div align="center">

**Your desktop is waiting.**

</div>
