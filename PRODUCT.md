# Product

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Users

Bennett, at his own Windows desk, while coding with several agents. The job is to see each agent's remaining quota without leaving that desk.

## Product Purpose

Show how much quota each coding agent has used, and when it resets, from the Bloom notch he already watches. Success is a glance that stays current, and a failed reader that does not take the others or the desktop down with it.

## Positioning

Usage is read from each agent's own local credentials and CLIs, then shown in the Bloom notch. A neighboring quota widget that opens its own window is not this product.

## Operating Context

Windows desktop. Bloom replaces the taskbar and draws a centered notch at the top of the primary monitor. The user works in Cursor and the other agents at the same time. Quota readings arrive from a helper process. The clock stays inside the notch; Cursor already owns the far corner of its own window.

## Capabilities and Constraints

- Agents: Claude, Kiro, Codex, Cursor, Copilot, Devin, Antigravity, Grok Bot, Command Code, and Qoder. A missing or failed reader shows as failed with a reason. The others keep working.
- The only quota UI is the Bloom notch. Do not launch the floating `agentquota.exe` window.
- The helper runs as `--quota-service` at `%LOCALAPPDATA%\com.sehaz.bloom\agentquota.exe`.
- The Bloom he runs is `%LOCALAPPDATA%\Programs\Bloom\bloom.exe`. Closing it must restore the Windows taskbar through `restore_taskbar_and_exit`.
- The notch is focus-free: no activation, no Escape, no click-outside dismissal.
- Credentials are read-only. They are not cached and not written to logs.
- Quota rows are flat: a usage bar and percent beside the label, the reset on the right. The footer says "Just checked". There is no global "1 failed" count.
- Clicking the agent mark in the resting pill cycles to the next agent in snapshot order. The pill click still opens the panel.
- The resting housing stays centered and only as wide as its contents. It does not become a full-width menu bar.
- Platform is recorded as `adaptive` because the shell is a desktop app, not a website. It ships on Windows only. It does not currently adapt its design per OS, and it does not ship on iOS or Android.
- UI copy is English. Reset times use the local clock.

## Brand Commitments

The desktop shell is named Bloom. This repository is the quota module that Bloom's notch shows.

## Evidence on Hand

- `AGENTS.md` and `docs/specs/2026-10-13-agent-quota-widget-design.md` describe the readers and the widget contract.
- `docs/superpowers/specs/2026-10-28-centered-housing-notch-design.md` records the centered housing.
- `bloom/README.md` is Bloom's public description. It is not a claim about who uses the quota notch.
- No testimonials, pricing, or usage benchmarks are on hand. Do not invent them.

## Product Principles

- The notch is for a glance while coding, not a second application.
- One agent's failure stays that agent's failure.
- Credentials are read, never kept.
- Closing Bloom gives the Windows desktop back.
- The housing stays out of the way of the windows already on screen.
