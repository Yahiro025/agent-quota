use std::path::PathBuf;
use std::process::Command;

fn main() {
    // The helper has to exist before rustc embeds it. A quota-reader change
    // rebuilds that exe; everything else leaves the previous embed in place.
    embed_helper();
    tauri_build::build()
}

fn embed_helper() {
    let manifest_dir = PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap());
    let repo = manifest_dir.join("..").join("..");
    println!("cargo:rerun-if-changed=../../go.mod");
    println!("cargo:rerun-if-changed=../../go.sum");
    println!("cargo:rerun-if-changed=../../main.go");
    println!("cargo:rerun-if-changed=../../quota");
    println!("cargo:rerun-if-changed=../../internal");

    let out = PathBuf::from(std::env::var("OUT_DIR").unwrap()).join("agentquota.exe");
    let status = Command::new(find_go())
        .current_dir(&repo)
        // A GUI subsystem keeps the worker from opening its own console.
        // Bloom still spawns it with CREATE_NO_WINDOW.
        .args(["build", "-ldflags", "-H windowsgui", "-o"])
        .arg(&out)
        .arg(".")
        .status()
        .expect("go build of the embedded quota helper failed to start");
    if !status.success() {
        panic!("go build of the embedded quota helper failed");
    }

    // include_bytes! receives this as a string literal. Forward slashes avoid
    // Windows backslashes being read as escape characters.
    let rendered = out.to_string_lossy().replace('\\', "/");
    println!("cargo:rustc-env=AGENTQUOTA_HELPER={rendered}");
}

fn find_go() -> PathBuf {
    if let Some(paths) = std::env::var_os("PATH") {
        for dir in std::env::split_paths(&paths) {
            let candidate = dir.join("go.exe");
            if candidate.is_file() {
                return candidate;
            }
        }
    }
    PathBuf::from(r"C:\Program Files\Go\bin\go.exe")
}
