# Agent Quota

Usage limits for the coding agents a person already uses and signs in to.

## Language

### Quotas and Readings

**Provider**:
The coding agent whose subscription or account supplies a usage limit.

**Reading**:
A provider's reported usage percentage together with the detail needed to interpret it, or a reason usage is unavailable.

**Quota snapshot**:
A provider-reported usage limit observed at a particular time. Repeating the same observation does not make it newer.
_Avoid_: token-cost estimate, live balance

**Quota window**:
A period or pool over which a provider meters usage, such as five hours, a week, or a group of models.

**Stale reading**:
The last successful reading retained after a newer attempt fails, with its age and the latest failure shown.

**Sign-in owner**:
The coding client that manages its own credentials and renews them when possible.
_Avoid_: widget-managed login

### Surfaces and Placement

**Seam**:
The primary-display surface that fuses the resting rail and open overview into one continuous matte-black notch at the display's top edge, or detaches into a rounded floating pill and overview when dragged below that edge.
_Avoid_: separate notch mode, detached inspector

**Taskbar Focus**:
The docked presentation that places a two-line readout inside the primary bottom taskbar beside the notification area and opens a detail popup above it.

**Top bar**:
An application bar or taskbar reserved along the top edge of the primary display (such as MyDockFinder) that shrinks the desktop work area below the physical top edge of the monitor.
