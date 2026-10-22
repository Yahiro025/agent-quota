//! Features kept in the tree but switched off for this build.

// The startup update check is off; updater.rs stays so the manual commands
// keep working and re-enabling the check is a one-line change.
pub fn run_update_check_on_startup() -> bool {
    false
}

// Website announcements are off; only the test reads this so far, and the
// hook keeps the same flag on the frontend.
#[allow(dead_code)]
pub fn show_announcements() -> bool {
    false
}

#[cfg(test)]
mod tests {
    #[test]
    fn excluded_features_stay_off() {
        assert!(!super::run_update_check_on_startup());
        assert!(!super::show_announcements());
    }
}
