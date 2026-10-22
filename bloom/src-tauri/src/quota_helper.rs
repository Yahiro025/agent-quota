//! Supervisor for the headless agentquota helper.
//!
//! Bloom spawns `agentquota.exe --quota-service` beside its own executable
//! and keeps it alive for the app's lifetime. The helper speaks one JSON
//! object per line on stdout (see the Protocol section of
//! `docs/superpowers/specs/2026-10-22-bloom-agent-quota-design.md`); stderr
//! is logs only and never enters the protocol. When the helper dies, the
//! last snapshot stays visible and the supervisor respawns after a short
//! delay, so a missing or crashing helper degrades quota to "unavailable"
//! without touching any other panel.

use serde_json::Value;
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::time::Duration;
use tauri::{AppHandle, Emitter};

/// `hello` opens every session: the helper answers `ready`, then one
/// snapshot of its current book, then starts a refresh of every agent.
const HELLO: &[u8] = b"{\"v\":1,\"op\":\"hello\"}\n";

/// Tauri event carrying the readings array to the frontend.
const EVENT_SNAPSHOT: &str = "quota-snapshot";

/// Breathing room between a helper exit and the next spawn, so a helper
/// that dies on startup cannot tight-loop the supervisor thread.
const RESTART_DELAY: Duration = Duration::from_secs(1);

/// One parsed line's effect on the supervisor.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Apply {
    Ready,
    Snapshot,
    Bye,
    Error,
    Ignore,
}

/// The last snapshot the helper sent, plus whether quota is unavailable.
///
/// `unavailable` is only ever set by `note_exit` (helper gone with nothing
/// to show) and cleared by the next snapshot, so it can only be true while
/// `readings` is empty.
#[derive(Default)]
pub struct HelperState {
    pub readings: Vec<Value>,
    pub unavailable: bool,
}

/// Builds the helper process command: `exe --quota-service`, pipes for the
/// protocol, and no console window — the helper is a console binary spawned
/// from a GUI app, so without CREATE_NO_WINDOW a console would flash open.
pub fn helper_command(exe: &Path) -> Command {
    let mut command = Command::new(exe);
    command
        .arg("--quota-service")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        command.creation_flags(CREATE_NO_WINDOW);
    }
    command
}

/// Folds one stdout line into `state`.
///
/// Only a `snapshot` line mutates `readings`: it replaces them wholesale
/// and clears `unavailable`, whether or not `ready` arrived first. `ready`,
/// `bye`, and `error` never touch `readings` and never set `unavailable` —
/// only `note_exit` can — so a live helper's error keeps the last snapshot
/// on screen. Blank lines, non-JSON noise, and unknown ops are ignored.
pub fn apply_line(state: &mut HelperState, line: &str) -> Apply {
    let line = line.trim();
    if line.is_empty() {
        return Apply::Ignore;
    }
    let Ok(message) = serde_json::from_str::<Value>(line) else {
        return Apply::Ignore;
    };
    match message.get("op").and_then(Value::as_str) {
        Some("ready") => Apply::Ready,
        Some("snapshot") => match message.get("readings").and_then(Value::as_array) {
            Some(readings) => {
                state.readings = readings.clone();
                state.unavailable = false;
                Apply::Snapshot
            }
            None => Apply::Ignore,
        },
        Some("bye") => Apply::Bye,
        Some("error") => Apply::Error,
        _ => Apply::Ignore,
    }
}

/// Records a helper exit. With no snapshot on file the quota surface is
/// unavailable; with one, the last readings stay and `unavailable` stays
/// false so the panel keeps showing slightly-stale data until respawn.
pub fn note_exit(state: &mut HelperState) {
    if state.readings.is_empty() {
        state.unavailable = true;
    }
}

/// Starts the supervisor on its own thread and returns at once. Setup must
/// not block on the helper: the notch opens whether or not agentquota.exe
/// exists.
pub fn start(app: &AppHandle) {
    let app = app.clone();
    std::thread::spawn(move || supervise(&app));
}

/// The spawn-read-restart loop. `state` outlives every child, which is what
/// keeps the last snapshot on screen across restarts.
fn supervise(app: &AppHandle) {
    let exe = helper_exe();
    let mut state = HelperState::default();
    loop {
        match run_once(&exe, app, &mut state) {
            Ok(()) => crate::diagnostics::record("quota_helper", "helper exited"),
            Err(err) => {
                crate::diagnostics::record("quota_helper", &format!("helper failed: {err}"))
            }
        }
        note_exit(&mut state);
        std::thread::sleep(RESTART_DELAY);
    }
}

/// The default helper location: `agentquota.exe` beside the running
/// executable. Falls back to a bare name so a PATH install still resolves.
fn helper_exe() -> PathBuf {
    std::env::current_exe()
        .ok()
        .and_then(|exe| exe.parent().map(|dir| dir.join("agentquota.exe")))
        .unwrap_or_else(|| PathBuf::from("agentquota.exe"))
}

/// One helper lifetime: spawn, say hello, consume stdout until EOF or bye,
/// then reap. A kill is only sent on abnormal ends — after `bye` the child
/// is already on its way out and killing it could interrupt its cleanup.
fn run_once(exe: &Path, app: &AppHandle, state: &mut HelperState) -> std::io::Result<()> {
    let mut child = helper_command(exe).spawn()?;
    let result = pump(&mut child, app, state);
    if result.is_err() {
        let _ = child.kill();
    }
    let _ = child.wait();
    result
}

/// The protocol half of one helper lifetime: hello, then one line at a
/// time. `child.stdin` is deliberately not taken — dropping it would close
/// the pipe and make the helper exit — and stderr is left unread at NUL.
fn pump(child: &mut Child, app: &AppHandle, state: &mut HelperState) -> std::io::Result<()> {
    if let Some(stdin) = child.stdin.as_mut() {
        stdin.write_all(HELLO)?;
        stdin.flush()?;
    }
    let Some(stdout) = child.stdout.take() else {
        return Ok(());
    };
    for line in BufReader::new(stdout).lines() {
        let line = line?;
        match apply_line(state, &line) {
            Apply::Snapshot => {
                let _ = app.emit(EVENT_SNAPSHOT, state.readings.clone());
            }
            // The message field is a reason, never a reading body, and the
            // protocol carries no credentials — it is safe to log.
            Apply::Error => {
                let message = serde_json::from_str::<Value>(&line)
                    .ok()
                    .and_then(|v| v.get("message").and_then(Value::as_str).map(str::to_owned))
                    .unwrap_or_default();
                crate::diagnostics::record(
                    "quota_helper",
                    &format!("helper reported error: {message}"),
                );
            }
            Apply::Ready => crate::diagnostics::record("quota_helper", "helper ready"),
            Apply::Bye => break,
            Apply::Ignore => {}
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn helper_argv_is_quota_service() {
        let command = helper_command(Path::new("C:/helpers/agentquota.exe"));
        assert_eq!(
            command.get_program(),
            std::ffi::OsStr::new("C:/helpers/agentquota.exe")
        );
        let args: Vec<&std::ffi::OsStr> = command.get_args().collect();
        assert_eq!(args, [std::ffi::OsStr::new("--quota-service")]);
    }

    #[test]
    fn snapshot_replaces_readings() {
        let mut state = HelperState::default();

        // A snapshot may arrive before ready.
        let first = apply_line(
            &mut state,
            r#"{"v":1,"op":"snapshot","readings":[{"id":"claude","state":"ok","used":42.0}]}"#,
        );
        assert_eq!(first, Apply::Snapshot);
        assert_eq!(state.readings.len(), 1);
        assert_eq!(state.readings[0]["id"], "claude");

        // Ready does not clear readings.
        assert_eq!(
            apply_line(&mut state, r#"{"v":1,"op":"ready","agents":["claude"]}"#),
            Apply::Ready
        );
        assert_eq!(state.readings.len(), 1);

        // A later snapshot replaces them again.
        let second = apply_line(
            &mut state,
            r#"{"v":1,"op":"snapshot","readings":[{"id":"kiro","state":"failed","err":"timeout"},{"id":"codex","state":"ok","used":7.0}]}"#,
        );
        assert_eq!(second, Apply::Snapshot);
        assert_eq!(state.readings.len(), 2);
        assert_eq!(state.readings[0]["id"], "kiro");

        // Bye does not clear readings and does not set unavailable.
        assert_eq!(
            apply_line(&mut state, r#"{"v":1,"op":"bye"}"#),
            Apply::Bye
        );
        assert_eq!(state.readings.len(), 2);
        assert!(!state.unavailable);
    }

    #[test]
    fn exit_keeps_last_snapshot() {
        let mut state = HelperState::default();
        apply_line(
            &mut state,
            r#"{"v":1,"op":"snapshot","readings":[{"id":"codex","state":"stale","used":71.5}]}"#,
        );

        // An error line and a non-JSON line keep the snapshot and leave
        // unavailable false.
        assert_eq!(
            apply_line(&mut state, r#"{"v":1,"op":"error","message":"kiro timeout"}"#),
            Apply::Error
        );
        assert_eq!(state.readings.len(), 1);
        assert!(!state.unavailable);
        assert_eq!(apply_line(&mut state, "not json at all"), Apply::Ignore);
        assert_eq!(state.readings.len(), 1);
        assert!(!state.unavailable);

        note_exit(&mut state);
        assert_eq!(state.readings.len(), 1);
        assert_eq!(state.readings[0]["id"], "codex");
        assert!(!state.unavailable);
    }

    #[test]
    fn exit_without_snapshot_is_unavailable() {
        let mut state = HelperState::default();

        // A blank line returns Ignore and changes nothing.
        assert_eq!(apply_line(&mut state, "   "), Apply::Ignore);
        assert!(state.readings.is_empty());
        assert!(!state.unavailable);

        note_exit(&mut state);
        assert!(state.readings.is_empty());
        assert!(state.unavailable);
    }
}
