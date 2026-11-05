# Bloom media connectors

The bridge runs inside Bloom, on authenticated IPv4 loopback. Spotify and Brave
receive different random keys; no Spotify login or browser account tokens are
read. The browser key stays in its extension worker, never in YouTube's page.
Generated files are local credentials: do not commit or share them.

Build local bundles with Bun after the candidate has created its connector config:

```powershell
bun run connectors/runtime/build.js "$env:LOCALAPPDATA/com.sehaz.bloom/media-connectors.json"
```

Use the configuration produced by the exact Bloom instance being tested. A
separate test host uses a separate configuration. Stop that host before starting
the matching candidate; two bridge listeners must never share a port.

For Spicetify, preserve `config-xpui.ini`, existing extensions and patched Apps
resources in a dated durable backup. Copy only `generated/bloom-media.js` to
Spicetify's user Extensions folder. Add it to the existing extension list without
replacing the list. Apply through the installed Spicetify CLI's `refresh -e -n`
command; restart Spotify normally only when needed and authorized. Restore the
original extension list and resources after a temporary canary. Do not read or
copy account credentials or Spotify session data.

For Brave, copy the built `generated/youtube` folder to its durable home
`%LOCALAPPDATA%\com.sehaz.bloom\connectors\youtube`, then load that folder once
through Developer mode → Load unpacked and remove any older entry that points at
a deleted folder. This is the runtime connector, distinct from the temporary
Connector Check popup. The extension requests the `tabs` permission, which Brave
warns about as "Read your browsing history". The worker uses it only to list
audible tabs' title, hostname and mute state to Bloom over the authenticated
loopback bridge; nothing else about a tab is read, nothing is stored, and nothing
is sent anywhere else. Non-audible tabs are never inspected. The worker also
inspects permitted YouTube pages and binds each action to Chrome's tab and
document IDs. Normal unsupported pages remain under native media controls.
Remove the temporary proof extension after runtime validation, preserving other
extensions. Do not launch a separate browser profile or bypass browser policy.

In compact Bloom, open **Up next** beside Audio Output. The transport stays
visible. Selecting an item uses its source occurrence identity and current queue
revision. Only observed playback confirms success. After uncertainty, inspect
playback, then use **Check queue**; nothing automatically retries a selection.
Multiple active/ambiguous players disable the queue rather than guessing a tab.

Bounds: four authenticated connections, sixteen sessions total, sixty-four
transferred upcoming rows per session, one MiB WebSocket messages, and five-second
selection deadlines. YouTube lists over 5,000 entries are explicitly unavailable.
A Spotify row with no internal occurrence is explicitly unavailable. Repeated copies of the same occurrence id stay available.
The drawer states when only a prefix of a larger queue is shown.

Native Spotify duplicate selection and the installed Brave proof popup passed
before integration. Those do not constitute acceptance of the final Bloom drawer;
the exact matched candidate still requires source-mode, multi-source and native UI
canary tests. Do not deploy or restart production Bloom as part of this build step.
