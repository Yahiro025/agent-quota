# Bloom + Agent Quota discovery record

Date: 2026-10-22
Thread: 7065c746-4c11-42b0-9764-7bbaa630c07f

The written questionnaire file was not saved in the design session. These are the decisions that session locked, in the order they were settled.

1. One notch for this person's own desktop, containing Agent Quota and Bloom's non-dock capabilities.
2. Compact resting display. Ongoing activities such as music and timers stay reachable. Larger tools open on demand. Quota stays out of the resting display until opened or an urgent condition occurs.
3. Urgent alerts may expand the notch briefly, then restore the previous view. Typing, dragging, pairing, and an unfinished connection defer alerts.
4. Bloom's shape, typography, motion, and existing controls stay. Quotas use that language.
5. Opening behavior stays Bloom's. Overview and Agent Quotas are extra panels, not a new default.
6. The native Agent Quota taskbar view is out. The product is notch-only. The native Agent Quota app remains the rollback.
7. One Tauri app from the Bloom 3.10.0 checkout owns the notch, edge HUDs, settings, and Windows interaction. A headless Go helper owns the seven readers.
8. Dock lifecycle is removed: startup, AppBar, taskbar hiding, recovery, and shutdown restoration. Top-edge placement over MyDockFinder and fullscreen suppression carry forward.
9. Release and announcement services are excluded.
10. Passive interaction stays focus-free. A deliberate click on a text input may take focus.
11. The Focus/DND control must perform a real supported action or be an accurately labelled settings shortcut. Inspection showed it only toggles local UI state.
