/// Startup decisions for the dock/taskbar takeover.
///
/// The notch build never takes over the native taskbar or opens the dock
/// window at startup, so every flag is `false`. Setup still initialises the
/// crash marker and restores the taskbar when a previous session left it
/// hidden — that is recovery, not hiding, and is not part of this plan.
pub struct StartupPlan {
    pub hide_taskbar: bool,
    pub register_dock_appbar: bool,
    pub start_watchdog: bool,
    pub open_dock_window: bool,
}

pub fn notch_startup_plan() -> StartupPlan {
    StartupPlan {
        hide_taskbar: false,
        register_dock_appbar: false,
        start_watchdog: false,
        open_dock_window: false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn notch_startup_does_not_touch_the_taskbar_or_dock() {
        let plan = notch_startup_plan();
        assert!(!plan.hide_taskbar);
        assert!(!plan.register_dock_appbar);
        assert!(!plan.start_watchdog);
        assert!(!plan.open_dock_window);
    }
}
