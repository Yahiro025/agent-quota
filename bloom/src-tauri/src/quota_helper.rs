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
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::Mutex;
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

/// The resting notch height in unscaled px — the same 420
/// `change_notch_mode` multiplies by bloom scale and the monitor's scale
/// factor. `clamp_height` never lets the quota space shrink below it.
pub const NORMAL_HEIGHT: u32 = 420;

/// The running helper's stdin. `pump` parks it here after HELLO so the pipe
/// stays open for the child's whole life — dropping the handle would close
/// the pipe and make the helper exit — and so `refresh_quota` can write to
/// it. `note_exit` clears it once the child is gone.
static HELPER_STDIN: Mutex<Option<ChildStdin>> = Mutex::new(None);

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
/// Also drops the dead child's stdin so a refresh can never write to a
/// closed pipe.
pub fn note_exit(state: &mut HelperState) {
    if let Ok(mut slot) = HELPER_STDIN.lock() {
        *slot = None;
    }
    if state.readings.is_empty() {
        state.unavailable = true;
    }
}

/// One refresh request line for the helper's stdin: `id` names a single
/// agent, `None` asks for every agent at once.
pub fn refresh_line(id: Option<&str>) -> String {
    // json! owns the id's escaping so it can never corrupt the line; the
    // envelope stays literal so the field order is byte-exact whatever map
    // order serde_json was built with.
    let id = serde_json::json!(id.unwrap_or("all"));
    format!("{{\"v\":1,\"op\":\"refresh\",\"id\":{id}}}\n")
}

/// Fits a requested quota-space height between the resting notch height and
/// the monitor, so an over-tall panel scrolls instead of overflowing the
/// screen and a collapsed one lands back at the normal height.
pub fn clamp_height(requested: u32, monitor: u32) -> u32 {
    requested.max(NORMAL_HEIGHT).min(monitor)
}

/// Writes one refresh line to the running helper's stdin. A dead or
/// restarting helper is a no-op: the next snapshot revives the button, so
/// the caller never has to retry.
pub fn write_refresh(id: Option<&str>) -> Result<(), String> {
    let Ok(mut slot) = HELPER_STDIN.lock() else {
        return Err("quota helper stdin lock poisoned".to_string());
    };
    let Some(stdin) = slot.as_mut() else {
        return Ok(());
    };
    let line = refresh_line(id);
    stdin
        .write_all(line.as_bytes())
        .and_then(|()| stdin.flush())
        .map_err(|e| e.to_string())
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

/// Writes HELLO, then parks the child's stdin in HELPER_STDIN so
/// `write_refresh` can reach it. A poisoned lock cannot hold the pipe, so
/// the handle comes back to the caller, which must keep it alive until
/// the read loop ends — dropping it closes the pipe and the helper exits.
fn open_pipe(child: &mut Child) -> std::io::Result<Option<ChildStdin>> {
    let Some(mut stdin) = child.stdin.take() else {
        return Ok(None);
    };
    stdin.write_all(HELLO)?;
    stdin.flush()?;
    match HELPER_STDIN.lock() {
        Ok(mut slot) => {
            *slot = Some(stdin);
            Ok(None)
        }
        Err(_) => Ok(Some(stdin)),
    }
}

/// The protocol half of one helper lifetime: hello, then one line at a
/// time. After HELLO the child's stdin moves into HELPER_STDIN — dropping
/// it would close the pipe and make the helper exit — and stderr is left
/// unread at NUL.
fn pump(child: &mut Child, app: &AppHandle, state: &mut HelperState) -> std::io::Result<()> {
    // Parked at pump scope: when HELPER_STDIN is poisoned the pipe comes
    // back in _keep_open, which must live until the read loop ends —
    // dropping it earlier would close the pipe and make the helper exit.
    let _keep_open = open_pipe(child)?;
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
    fn refresh_line_defaults_to_all() {
        assert_eq!(
            refresh_line(None),
            "{\"v\":1,\"op\":\"refresh\",\"id\":\"all\"}\n"
        );
    }

    #[test]
    fn refresh_line_carries_one_agent() {
        assert!(refresh_line(Some("devin")).contains("\"id\":\"devin\""));
    }

    #[test]
    fn refresh_line_escapes_the_id() {
        // An id must stay inside its JSON string whatever it contains.
        let line = refresh_line(Some("de\"vin"));
        let parsed: Value = serde_json::from_str(line.trim_end()).unwrap();
        assert_eq!(parsed["v"], 1);
        assert_eq!(parsed["op"], "refresh");
        assert_eq!(parsed["id"], "de\"vin");
    }

    #[test]
    fn open_pipe_parks_stdin_until_the_lock_is_poisoned() {
        let spawn = || {
            Command::new("cmd.exe")
                .stdin(Stdio::piped())
                .stdout(Stdio::null())
                .stderr(Stdio::null())
                .spawn()
                .unwrap()
        };

        // Healthy lock: the pipe moves into HELPER_STDIN for refresh.
        let mut child = spawn();
        assert!(open_pipe(&mut child).unwrap().is_none());
        assert!(HELPER_STDIN.lock().unwrap().is_some());
        *HELPER_STDIN.lock().unwrap() = None;
        let _ = child.kill();
        let _ = child.wait();

        // Poisoned lock: the pipe comes back and stays writable, where an
        // early drop would have closed it.
        let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            let _guard = HELPER_STDIN.lock().unwrap();
            panic!("poison the helper stdin lock");
        }));
        let mut child = spawn();
        let mut stdin = open_pipe(&mut child)
            .unwrap()
            .expect("poisoned lock returns the pipe to hold");
        stdin.write_all(b"echo alive\r\n").unwrap();
        drop(stdin);
        let _ = child.wait();
    }

    #[test]
    fn clamp_height_bounds_between_normal_and_monitor() {
        assert_eq!(clamp_height(2000, 1080), 1080);
        assert_eq!(clamp_height(0, 1080), NORMAL_HEIGHT);
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
