# Playback routing: architecture checkpoint

Date: 2026-10-29. Read-only independent architecture consultation by `astra-advisor`; not a final code review or live acceptance result.

## Concrete source findings

The existing media display selects the first playing enumerated GSMTC session, while transport commands independently call `GetCurrentSession()`. `MediaInfo` has no target identity. Source anchors: `bloom/src-tauri/src/services.rs:1158–1184,1223–1369`, `types.rs:24–38`.

GSMTC identifies an app, not a Brave tab/document. Therefore, attaching a connector queue to native metadata based on title/artist or app name cannot establish an exact player association. Spotify's local-versus-remote device behavior also needs explicit verification.

## Recommended production boundary

Use one backend-owned active target: either a connector instance/session/document generation, or a retained GSMTC session handle and generation. All media metadata, timing, queue, confirmation and playback controls for a connector-owned surface must come from that same connector. Every UI command carries the displayed opaque generation. Native fallback commands use the retained displayed session, not a fresh current-session lookup. System volume and output stay system controls.

Browser identity must include extension instance, tab ID, main-frame document ID and player generation. Tab/document identifiers come from extension APIs, not page-supplied payloads. The page nonce and queue revision provide only the inner occurrence boundary. Relevant Chrome reference: https://developer.chrome.com/docs/extensions/reference/api/runtime#type-MessageSender .

Fail closed when multiple playing connectors, ambiguous browser players, stale inventories, or a conflicting unknown source prevent a reliable choice. Keep GSMTC fallback with queue unavailable. Connector authority establishes that Bloom's own displayed state and commands target the same player; it does not prove that player is the OS-selected browser session. Preserving that precise OS choice in ambiguous multiple-tab cases requires another trustworthy association or unavailable behavior, never inference.

## Transport recommendation, not yet chosen or proved

Prefer one authenticated loopback WebSocket listener inside Bloom, using a maintained frame/handshake implementation such as `tokio-tungstenite` and Tokio networking. The Spicetify proof established localhost HTTP fetch, not WebSocket/CSP support; verify WebSocket access before adding a production dependency. If unsupported, use a maintained HTTP exchange implementation such as Axum, not a handwritten general HTTP server or a second always-running process.

Bind only `127.0.0.1`, fail closed on a port collision, authenticate with separate source-scoped random secrets, and validate host/origin in addition to credentials. Credentials stay in the MV3 worker, outside YouTube page world. No shell, arbitrary fetch, or filesystem endpoints. Revalidate browser tab/document before dispatch. Chromium worker reconnect behavior must be explicit; WebSocket lifetime reference: https://developer.chrome.com/docs/extensions/how-to/web-platform/websockets .

Initial bounds to evaluate: 1 MiB messages, 64 transferred rows, 16 sessions, bounded connections and channels, one pending selection per session, heartbeat expiry, new generations after reconnect, five-second observed confirmation, request deduplication, and no automatic replay after uncertainty. Origin alone is not authentication.

## Scope and evidence limits

The consultation read the amended spec, relevant media source and connector proof code. No files were changed by the reviewer; no tests, installed-app inspection or live validation were performed. The main agent persisted this checkpoint. No production transport, arbitration policy, credentials or app code has been installed based on it. The source proof gate and human-assisted Brave test remain open.
