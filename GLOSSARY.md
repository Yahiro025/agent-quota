# Agent Quota

Usage limits for the coding agents a person already uses and signs in to.

## Language

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
