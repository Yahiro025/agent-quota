# Bloom + Agent Quota — One notch away

A 90-second product film built with Video Shotcraft's motion grammar, real frontend captures, original staging and licensed external audio. Hook: **Your day. One notch away.**

## Deliverables

- `out/Bloom-One-Notch-Away.mp4` — 1080p master with music and effects.
- `out/Bloom-One-Notch-Away-SFX.mp4` — identical picture with effects only.
- `storyboard-full-project.md` — concept, hook, 12-shot storyboard and feature mapping.
- `credits.txt`, `LICENSES/`, `public/fonts/Inter-OFL.txt` — asset and license records.
- `out/qa/`, `out/parity/results.json`, `out/audio-sync.json`, `out/audio-mastering.json` — verification evidence.

The film covers the adaptive notch and live activities, music/layouts/visualizer, calendar/Pomodoro, Shelf files/text, command center, Wi-Fi/Bluetooth, weather and hardware widgets, volume/brightness/app mixer, seven agent quotas, Overview, and personalization. Small command-center shortcuts are visible; they are not individually exercised. The existing dock stays outside this companion film, following the product's established scope.

## Editorial method

This is an authored motion-graphics demo, using captured source UI with fictional backend data. Cursor actions sequence genuine captured states. It is not a live recording, OS/network/account test, or native-performance measurement. The soundtrack-responsive bars, perspective planes and camera choreography are editorial graphics. Media artwork is original; the track title and artist identify the licensed soundtrack.

The archived quota-only draft is retained under `archive/` for provenance. The current production is `src/FullFilm.tsx` and `src/FullTimeline.ts`; `src/Film.tsx` supplies shared stage/type components.

## Edit

The Shotcraft workbench is at **http://localhost:5198/** while its local process is running. Import “Bloom + Agent Quota · One notch away” to expose 12 shot units, editable titles, music and individual SFX clips. Shots contain their camera choreography and transitions. Workbench preview adds one second of timeline padding; exact exports use 2700 frames.

The Windows setup uses real file copies in the installed workbench because its Unix symlink launcher fails on this host. `scripts/workbench-sync.ps1`, followed by the installed `gen-index.mjs` and project `workbench-normalize.mjs`, refreshes those copies. `scripts/parity.mjs` compares imported and original compositions at representative frames. The music card uses an envelope-baked derivative of the same downloaded track; editor levels are unmastered. GUI export on Windows is not verified; use the reproducible render commands below for the delivery master.

Remotion Studio: `npm run studio`, then http://localhost:3010/RoomToBuild .

## Rebuild

From this folder, with Node, Chrome and FFmpeg available:

```powershell
npm ci
npm run preview
npm run render
npm run audio
npm run audio:sfx
npm run master
node scripts/mux.mjs
```

The rendering script explicitly selects Chrome at `C:/Program Files/Google/Chrome/Application/chrome.exe`. Change that path for another host. Capture scripts use the existing Playwright package in the repository's `.scratch/qa/node_modules`, and require Bloom's Vite frontend at port 5174. The saved public captures are sufficient to render; recapture is optional. `scripts/capture-full.mjs --panels` refreshes panels and leaves saved edge-overlay captures intact.

The BGM master uses two-pass loudness normalization toward −14 LUFS with a −2 dBTP PCM ceiling to leave room for AAC encoding overshoot. The encoded MP4 is measured again. The sparse SFX edition preserves quiet space rather than forcing music-like integrated loudness. Both editions use the same 2700-frame picture and cue timings. No audio was synthesized.

Motion references: [Video Shotcraft gallery](https://vincentwei1021.github.io/video-shotcraft/) and the exact selected card records in `shotcraft-full-references.json`. Visual study: [Apple MacBook Pro](https://www.apple.com/macbook-pro/). No Apple assets or copy reused.
