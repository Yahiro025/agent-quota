# Codex optimization rollback preparation

No deployment or rollback has been performed. As observed on 2026-10-29, the
running Bloom executable was `C:\Users\Bennett\AppData\Local\bloom\bloom.exe`
and its helper was `C:\Users\Bennett\AppData\Local\com.sehaz.bloom\agentquota.exe`.
The previous known-running versions are identified by SHA-256:

| Binary | SHA-256 |
| --- | --- |
| Bloom | `F763DF936158ACCBEE724EE7DAD0584D3D70A1DDCBEC18123CF95DE1DA60EE7F` |
| Helper | `6AE486991426EA4E6F7B9612EFF8F305886E8F3724C522A181C2B829195B7C61` |

Copies were made to `%TEMP%\agentquota-rollback-20261029\bloom-before.exe` and
`%TEMP%\agentquota-rollback-20261029\agentquota-before.exe`; both hashes matched
the originals immediately after copying. The binary version is identified by hash;
no release version was verified. Temporary storage may be cleaned by Windows: before
deployment, copy the backups to durable storage and verify the hashes again.

Potential rollback triggers include incorrect or stale quota/reset values, persistent
helper crashes, unacceptable latency or memory growth, and regressions in other
providers or Bloom's window and taskbar behavior. Investigate the symptom and obtain
explicit approval before touching production.

For an approved rollback, confirm the current executable paths and verify both
backup hashes. Close Bloom using its `restore_taskbar_and_exit` flow and verify
that Bloom and its helper have exited and the taskbar is restored. Preserve the
current binaries for diagnosis. Restore the matching prior Bloom and helper binaries
to their confirmed paths, then verify their hashes before launching Bloom. Restore
both: Bloom embeds a helper and may replace the extracted copy when it starts.
If the installed layout or embedded helper differs from the recorded pair, first
establish a matching prior package; do not assume that restoring only the helper
will persist. Restarting, restoring and deploying require explicit approval.

After restoration, verify the notch appears, Codex usage and reset match an
independent reading, refresh succeeds, the other providers behave normally, the
taskbar is intact, and the helper process stays healthy without orphaned children.
