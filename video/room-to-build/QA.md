# Delivery verification — 2026-10-26

The deliverables are `out/Bloom-One-Notch-Away.mp4` and `out/Bloom-One-Notch-Away-SFX.mp4`.

| Check | Evidence / result |
|---|---|
| Whole companion scope | 12-shot feature mapping in `storyboard-full-project.md`; actual captured panels and collapsed simultaneous activities appear in the film. |
| Format | Both: 1920×1080, 30 fps, 2700 frames, exactly 90.000 seconds, H.264 High, yuv420p, Rec.709, AAC stereo 48kHz. |
| Music edition loudness | Encoded MP4: −14.05 LUFS, −1.18 dBTP. The PCM ceiling was lowered to −2 dBTP after the first AAC encode overshot. |
| SFX edition loudness | Encoded MP4: −20.32 LUFS, −2.35 dBTP. Sparse effects retain their quiet spaces. |
| Picture parity | Identical video-stream SHA-256 in both MP4 editions; see `out/qa-report.json`. |
| Visual sampling | All 45 frames at 0, 2, …, 88 seconds reviewed in three contact sheets; additional full-resolution music, critical quota, settings and final-frame checks. |
| Black / frozen intervals | No black detections at the configured strict threshold; no frozen interval of 4 seconds or more. These detectors supplement visual inspection. |
| Interactions | Music pause preserves progress; Shelf tiles retain coordinates at landing; one quota cursor targets the closed-state disclosure; complete Media Layout choice and compact-player result are visible. |
| Click synchronization | Eleven waveform-correlation probes against rendered SFX PCM: maximum absolute offset 16 samples at 48kHz, or 0.01 video frame. Source click peak is 14.7ms into the decoded asset. |
| Music timing | 120 BPM edited bed; estimated source grid residual maximum 48.3ms. This is not a claim of perfect transient detection. |
| Editable project parity | Eight imported-vs-original representative 1080p frames: zero differing pixels. This verifies picture reconstruction, not GUI export on Windows. |
| Source validation | Video project's TypeScript check passes. Product source was not changed for video production. |
| Licensing | Downloaded Mixkit music and four SFX listed with source and license URLs in `credits.txt`; Inter, marks, Bloom and Shotcraft provenance retained. No audio synthesis. |

The film is an editorial demonstration using real frontend screenshots and fictional backend state. It does not establish live native OS, network, device, account or performance behavior. Perspective travel, visualizer choreography and cursor sequences are authored animation. The Clawd prompt was a reference for ambition and process; the user explicitly requested a 90-second product motion-graphics film, so its mascot, no-text rule and 60-second duration do not apply.

The independent reviewer first found four continuity/composition defects; all four were fixed and passed a targeted recheck. Final encoded-file review found no remaining blocker: the reviewer decoded HD frames, confirmed both video-stream hashes and file hashes, and independently measured final AAC click offsets at f415, f2160 and f2538 in both editions as −0.01, 0 and 0 frames. Numerical audio checks do not substitute for a human listening preference review; no claim of a listening pass is made.
