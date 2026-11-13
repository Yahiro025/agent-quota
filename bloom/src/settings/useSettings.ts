import { useState, useEffect, useCallback, useRef } from "react";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { enable, disable, isEnabled } from "@tauri-apps/plugin-autostart";
import { getVersion } from "@tauri-apps/api/app";
import type { UpdateCheckResult } from "../updater";
import { useSettingsSync } from "../hooks/useSettingsSync";
import { reloadIfMirrorWasStale } from "../hooks/settingsMirror";
import { hexToHsl } from "../theme";
import type { WidgetConfig } from "./types";
import {
	parseHiddenAgents,
	toggleHiddenAgent,
	QUOTA_HIDDEN_AGENTS_KEY
} from "../quota/agentVisibility";

function readBool(val: string | null): boolean {
	return val === "true";
}

export function useSettings() {
	const [saveError, setSaveError] = useState(
		() => sessionStorage.getItem("bloom-settings-save-error") !== null
	);
	const saveQueue = useRef<Promise<void>>(Promise.resolve());
	const saveFailed = useRef(false);
	const [autostart, setAutostart] = useState(false);
	const [timerSoundEnabled, setTimerSoundEnabled] = useState(
		() => localStorage.getItem("bloom-timer-sound-enabled") !== "false"
	);
	const [musicModeEnabled, setMusicModeEnabled] = useState(true);
	const [notchRememberPanel, setNotchRememberPanel] = useState(
		() => (localStorage.getItem("bloom-notch-remember-panel") ??
			localStorage.getItem("bloom-notch-click-to-open")) !== "false"
	);
	const [notchDefaultPanel, setNotchDefaultPanel] = useState<"timer" | "quotas">(
		() => localStorage.getItem("bloom-notch-default-panel") === "quotas" ? "quotas" : "timer"
	);
	const [quotaHiddenAgents, setQuotaHiddenAgents] = useState<string[]>(() =>
		parseHiddenAgents(localStorage.getItem(QUOTA_HIDDEN_AGENTS_KEY))
	);
	const [musicCompactNotch, setMusicCompactNotch] = useState(true);
	const [shelfPersist, setShelfPersist] = useState(
		() => localStorage.getItem("bloom-shelf-persist") === "true"
	);
	const [volumeOverlayEnabled, setVolumeOverlayEnabled] = useState(true);
	const [volumeEdgeEnabled, setVolumeEdgeEnabled] = useState(
		() => localStorage.getItem("bloom-volume-edge-enabled") !== "false"
	);
	const [brightnessOverlayEnabled, setBrightnessOverlayEnabled] = useState(
		() => localStorage.getItem("bloom-brightness-overlay-enabled") !== "false"
	);
	const [brightnessEdgeEnabled, setBrightnessEdgeEnabled] = useState(
		() => localStorage.getItem("bloom-brightness-edge-enabled") !== "false"
	);
	const [mediaAmbienceEnabled, setMediaAmbienceEnabled] = useState(true);
	const [mediaCompactGlowEnabled, setMediaCompactGlowEnabled] = useState(true);
	const [mediaLayout, setMediaLayout] = useState<"classic" | "compact">(
		() => (localStorage.getItem("bloom-media-layout") as "classic" | "compact") || "classic"
	);
	const [cornersEnabled, setCornersEnabled] = useState(
		() => localStorage.getItem("bloom-corners-enabled") === "true"
	);
	const [showUpdateIndicator, setShowUpdateIndicator] = useState(
		() => localStorage.getItem("bloom-show-update-indicator") !== "false"
	);
	const [timeFormat24h, setTimeFormat24h] = useState(
		() => localStorage.getItem("bloom-time-format-24h") === "true"
	);
	const [statusWidgets, setStatusWidgets] = useState<WidgetConfig>({
		left: ["cpu"],
		right: ["net"]
	});
	const [notchMode, setNotchMode] = useState("fixed");
	const [notchEdgeDelay, setNotchEdgeDelay] = useState(() => {
		const val = localStorage.getItem("bloom-notch-edge-delay");
		return val !== null ? parseInt(val) : 200;
	});
	const [lowBatteryThreshold, setLowBatteryThreshold] = useState(20);
	const [updateStatus, setUpdateStatus] = useState<
		"idle" | "checking" | "available" | "uptodate" | "error" | "downloading" | "installing"
	>("idle");
	const [updateVersion, setUpdateVersion] = useState("");
	const [appVersion, setAppVersion] = useState("");
	const [autoUpdate, setAutoUpdate] = useState(
		() => localStorage.getItem("bloom-auto-update") === "true"
	);
	const [scale, setScale] = useState(() =>
		parseFloat(localStorage.getItem("bloom-scale") || "1.0")
	);
	const [themeMode, setThemeMode] = useState(
		() => localStorage.getItem("bloom-theme-mode") || "dark"
	);
	const [themeColor, setThemeColor] = useState(
		() => localStorage.getItem("bloom-theme-color") || "#007aff"
	);
	const [themeOpacity, setThemeOpacity] = useState(() => {
		const val = localStorage.getItem("bloom-theme-opacity");
		return val !== null ? parseFloat(val) : 0.8;
	});
	const [themeSaturation, setThemeSaturation] = useState(() => {
		const val = localStorage.getItem("bloom-theme-saturation");
		return val !== null ? parseFloat(val) : 0.5;
	});
	const [themeBrightness, setThemeBrightness] = useState(() => {
		const val = localStorage.getItem("bloom-theme-brightness");
		return val !== null ? parseFloat(val) : 0.15;
	});
	const [exportStatus, setExportStatus] = useState<"idle" | "exporting" | "success" | "error">(
		"idle"
	);
	const [importStatus, setImportStatus] = useState<"idle" | "importing" | "success" | "error">(
		"idle"
	);

	// ── Load all settings from backend + localStorage ──
	const loadAllSettings = useCallback(async () => {
		try {
			const settings: Record<string, string> = await invoke("load_settings");
			if (await reloadIfMirrorWasStale(settings)) return;
			const getVal = (key: string) => {
				const val = settings[key];
				if (val !== undefined && val !== null) return String(val);
				return localStorage.getItem(key);
			};

			const apply = <T>(
				val: string | null,
				setter: (v: T) => void,
				transform: (v: string) => T
			) => {
				if (val !== null) setter(transform(val));
			};

			apply(getVal("bloom-timer-sound-enabled"), setTimerSoundEnabled, readBool);
			apply(getVal("bloom-music-mode-enabled"), setMusicModeEnabled, readBool);
			apply(getVal("bloom-notch-remember-panel") ?? getVal("bloom-notch-click-to-open"),
				setNotchRememberPanel, readBool);
			apply(getVal("bloom-notch-default-panel"), setNotchDefaultPanel,
				(v) => v === "quotas" ? "quotas" : "timer");
			apply(getVal(QUOTA_HIDDEN_AGENTS_KEY), setQuotaHiddenAgents, parseHiddenAgents);
			apply(getVal("bloom-music-compact-notch"), setMusicCompactNotch, readBool);
			apply(getVal("bloom-shelf-persist"), setShelfPersist, readBool);
			apply(getVal("bloom-volume-overlay-enabled"), setVolumeOverlayEnabled, readBool);
			apply(getVal("bloom-brightness-overlay-enabled"), setBrightnessOverlayEnabled, readBool);
			apply(getVal("bloom-media-ambience-enabled"), setMediaAmbienceEnabled, readBool);
			apply(getVal("bloom-media-compact-glow-enabled"), setMediaCompactGlowEnabled, readBool);
			apply(getVal("bloom-corners-enabled"), setCornersEnabled, readBool);
			apply(getVal("bloom-time-format-24h"), setTimeFormat24h, readBool);
			apply(getVal("bloom-show-update-indicator"), setShowUpdateIndicator, readBool);
			apply(getVal("bloom-auto-update"), setAutoUpdate, readBool);
			apply(getVal("bloom-volume-edge-enabled"), setVolumeEdgeEnabled, readBool);
			apply(getVal("bloom-brightness-edge-enabled"), setBrightnessEdgeEnabled, readBool);

			apply(getVal("bloom-scale"), setScale, parseFloat);
			apply(getVal("bloom-low-battery-threshold"), setLowBatteryThreshold, parseInt);

			apply(getVal("bloom-notch-mode"), setNotchMode, (v) => (v === "auto-hide" ? "smart" : v));
			apply(getVal("bloom-notch-edge-delay"), setNotchEdgeDelay, parseInt);

			apply(getVal("bloom-theme-mode"), setThemeMode, (v) => v);
			apply(getVal("bloom-theme-color"), setThemeColor, (v) => v);
			apply(getVal("bloom-theme-opacity"), setThemeOpacity, parseFloat);
			apply(getVal("bloom-theme-saturation"), setThemeSaturation, parseFloat);
			apply(getVal("bloom-theme-brightness"), setThemeBrightness, parseFloat);

			const widgetsVal = getVal("bloom-status-widgets");
			if (widgetsVal) {
				try {
					const parsed = JSON.parse(widgetsVal);
					if (parsed && Array.isArray(parsed.left) && Array.isArray(parsed.right)) {
						setStatusWidgets(parsed);
					}
				} catch {}
			}
		} catch (e) {
			console.error("Failed to load settings:", e);
		}
	}, []);

	// The file write is authoritative. Serialize controls that change several
	// settings, and update the local mirror only after the backend confirms the
	// write. A failed save reloads the page from persisted values and leaves a
	// visible error instead of keeping an optimistic control value on screen.
	const saveSetting = (key: string, value: string | null): Promise<boolean> => {
		const operation = saveQueue.current.then(async () => {
			if (saveFailed.current) return false;
			try {
				await invoke<void>("save_setting", { key, value });
			} catch (error) {
				console.error(`Failed to save ${key}:`, error);
				saveFailed.current = true;
				sessionStorage.setItem("bloom-settings-save-error", "true");
				setSaveError(true);
				window.setTimeout(() => window.location.reload(), 0);
				return false;
			}
			try {
				if (value === null) localStorage.removeItem(key);
				else localStorage.setItem(key, value);
				sessionStorage.removeItem("bloom-settings-save-error");
			} catch (error) {
				// The file is saved; a mirror failure must not be reported as a failed write.
				console.warn("Settings mirror update failed:", error);
			}
			setSaveError(false);
			return true;
		});
		saveQueue.current = operation.then(() => {});
		return operation;
	};

	const dismissSaveError = () => {
		sessionStorage.removeItem("bloom-settings-save-error");
		setSaveError(false);
	};

	// ── Initialize on mount ──
	useEffect(() => {
		loadAllSettings();

		isEnabled()
			.then(setAutostart)
			.catch(() => {});

		getVersion()
			.then((ver) => setAppVersion(ver || "3.1.2"))
			.catch(() => setAppVersion("3.1.2"));

		checkForUpdates(false);
	}, []);

	// ── Sync settings from other windows ──
	useSettingsSync({
		"bloom-notch-mode": setNotchMode,
		"bloom-notch-edge-delay": setNotchEdgeDelay,
		"bloom-timer-sound-enabled": setTimerSoundEnabled,
		"bloom-music-mode-enabled": setMusicModeEnabled,
		"bloom-notch-remember-panel": setNotchRememberPanel,
		"bloom-notch-default-panel": (v: unknown) =>
			setNotchDefaultPanel(v === "quotas" ? "quotas" : "timer"),
		[QUOTA_HIDDEN_AGENTS_KEY]: (v) => setQuotaHiddenAgents(parseHiddenAgents(v)),
		"bloom-music-compact-notch": setMusicCompactNotch,
		"bloom-media-ambience-enabled": setMediaAmbienceEnabled,
		"bloom-media-compact-glow-enabled": setMediaCompactGlowEnabled,
		"bloom-media-layout": setMediaLayout,
		"bloom-corners-enabled": setCornersEnabled,
		"bloom-show-update-indicator": setShowUpdateIndicator,
		"bloom-time-format-24h": setTimeFormat24h,
		"bloom-low-battery-threshold": setLowBatteryThreshold,
		"bloom-scale": setScale,
		"bloom-auto-update": setAutoUpdate,
		"bloom-volume-overlay-enabled": setVolumeOverlayEnabled,
		"bloom-volume-edge-enabled": setVolumeEdgeEnabled,
		"bloom-brightness-overlay-enabled": setBrightnessOverlayEnabled,
		"bloom-brightness-edge-enabled": setBrightnessEdgeEnabled,
		"bloom-theme-mode": setThemeMode,
		"bloom-theme-color": setThemeColor,
		"bloom-theme-opacity": setThemeOpacity,
		"bloom-theme-saturation": setThemeSaturation,
		"bloom-theme-brightness": setThemeBrightness
    }, [], (key) => { if (key === QUOTA_HIDDEN_AGENTS_KEY) setQuotaHiddenAgents([]); });

	// ── Listen for system accent changes (adaptive theme) ──
	useEffect(() => {
		const unlisten = listen<string>("system-accent-changed", (event) => {
			const mode = localStorage.getItem("bloom-theme-mode") || "dark";
			if (mode === "adaptive") {
				try {
					const hsl = hexToHsl(event.payload);
					setThemeSaturation(hsl.s / 100);
					setThemeBrightness(hsl.l / 100);
				} catch (e) {
					console.error("Failed to parse system accent color change HSL:", e);
				}
			}
		});
		return () => {
			unlisten.then((fn) => fn());
		};
	}, []);

	// ── Update checker ──
	const checkForUpdates = async (manual = true) => {
		setUpdateStatus("checking");
		try {
			const result = await invoke<UpdateCheckResult>("check_for_updates", { force: manual });
			if (result.available) {
				setUpdateStatus("available");
				setUpdateVersion(result.version ?? "");
			} else {
				setUpdateStatus("uptodate");
			}
		} catch (e) {
			console.error("Updater error:", e);
			setUpdateStatus("error");
		}
	};

	const installUpdate = async () => {
		try {
			setUpdateStatus("downloading");
			await invoke("install_update");
			setUpdateStatus("idle");
		} catch (e) {
			console.error(e);
			setUpdateStatus("error");
		}
	};

	// Reflect install progress triggered from any window (including auto-update).
	useEffect(() => {
		const unlisten = listen<{ status: string; progress?: number }>(
			"auto-update-status",
			(event) => {
				switch (event.payload.status) {
					case "downloading":
						setUpdateStatus("downloading");
						break;
					case "installing":
						setUpdateStatus("installing");
						break;
					case "done":
						setUpdateStatus((prev) =>
							prev === "downloading" || prev === "installing" ? "idle" : prev
						);
						break;
				}
			}
		);
		return () => {
			unlisten.then((fn) => fn());
		};
	}, []);

	// ── Autostart ──
	const toggleAutostart = async () => {
		try {
			const currentlyEnabled = await isEnabled();
			if (currentlyEnabled) {
				await disable();
				setAutostart(false);
			} else {
				// Dev runs from target/debug; registering it in HKCU Run would hijack login.
				if (!import.meta.env.DEV) await enable();
				setAutostart(true);
			}
		} catch (err) {}
	};

	// ── Simple boolean toggles ──
	const toggleTimerSound = () => {
		const next = !timerSoundEnabled;
		setTimerSoundEnabled(next);
		saveSetting("bloom-timer-sound-enabled", String(next));
	};

	const toggleMusicMode = () => {
		const next = !musicModeEnabled;
		setMusicModeEnabled(next);
		saveSetting("bloom-music-mode-enabled", String(next));
	};

	const toggleNotchRememberPanel = () => {
		const next = !notchRememberPanel;
		setNotchRememberPanel(next);
		saveSetting("bloom-notch-remember-panel", String(next));
	};

	const changeNotchDefaultPanel = (panel: "timer" | "quotas") => {
		setNotchDefaultPanel(panel);
		saveSetting("bloom-notch-default-panel", panel);
	};

	const toggleQuotaAgent = (id: string) => {
		const next = toggleHiddenAgent(quotaHiddenAgents, id);
		setQuotaHiddenAgents(next);
		saveSetting(QUOTA_HIDDEN_AGENTS_KEY, JSON.stringify(next));
	};

	const toggleMusicCompactNotch = () => {
		const next = !musicCompactNotch;
		setMusicCompactNotch(next);
		saveSetting("bloom-music-compact-notch", String(next));
	};

	const toggleShelfPersist = () => {
		const next = !shelfPersist;
		setShelfPersist(next);
		saveSetting("bloom-shelf-persist", String(next));
	};

	const toggleMediaLayout = (layout: "classic" | "compact") => {
		setMediaLayout(layout);
		saveSetting("bloom-media-layout", layout);
	};

	const toggleVolumeOverlay = () => {
		const next = !volumeOverlayEnabled;
		setVolumeOverlayEnabled(next);
		saveSetting("bloom-volume-overlay-enabled", String(next));
	};

	const toggleVolumeEdge = () => {
		const next = !volumeEdgeEnabled;
		setVolumeEdgeEnabled(next);
		saveSetting("bloom-volume-edge-enabled", String(next));
	};

	const toggleBrightnessOverlay = () => {
		const next = !brightnessOverlayEnabled;
		setBrightnessOverlayEnabled(next);
		saveSetting("bloom-brightness-overlay-enabled", String(next));
	};

	const toggleBrightnessEdge = () => {
		const next = !brightnessEdgeEnabled;
		setBrightnessEdgeEnabled(next);
		saveSetting("bloom-brightness-edge-enabled", String(next));
	};

	const toggleAmbience = () => {
		const next = !mediaAmbienceEnabled;
		setMediaAmbienceEnabled(next);
		saveSetting("bloom-media-ambience-enabled", String(next));
	};

	const toggleCompactGlow = () => {
		const next = !mediaCompactGlowEnabled;
		setMediaCompactGlowEnabled(next);
		saveSetting("bloom-media-compact-glow-enabled", String(next));
	};

	const toggleCorners = () => {
		const next = !cornersEnabled;
		setCornersEnabled(next);
		saveSetting("bloom-corners-enabled", String(next));
	};

	const toggleUpdateIndicator = () => {
		const next = !showUpdateIndicator;
		setShowUpdateIndicator(next);
		saveSetting("bloom-show-update-indicator", String(next));
	};

	const toggleTimeFormat24h = () => {
		const next = !timeFormat24h;
		setTimeFormat24h(next);
		saveSetting("bloom-time-format-24h", String(next));
	};

	const toggleAutoUpdate = () => {
		const next = !autoUpdate;
		setAutoUpdate(next);
		saveSetting("bloom-auto-update", String(next));
	};

	// ── Value setters ──
	const setNotchModeValue = (newMode: string) => {
		setNotchMode(newMode);
		saveSetting("bloom-notch-mode", newMode);
	};

	const handleNotchEdgeDelayChange = (val: number) => {
		setNotchEdgeDelay(val);
		saveSetting("bloom-notch-edge-delay", val.toString());
	};

	const handleThresholdChange = (val: number) => {
		setLowBatteryThreshold(val);
		saveSetting("bloom-low-battery-threshold", val.toString());
	};

	const handleScaleChange = (val: number) => {
		setScale(val);
		saveSetting("bloom-scale", val.toString());
	};

	const handleWidgetsChange = (config: WidgetConfig) => {
		setStatusWidgets(config);
		saveSetting("bloom-status-widgets", JSON.stringify(config));
	};

	// ── Theme handlers ──
	const handleThemeModeChange = async (mode: string) => {
		setThemeMode(mode);
		saveSetting("bloom-theme-mode", mode);

		if (mode === "adaptive") {
			try {
				const accentHex = await invoke<string>("get_system_accent_color");
				const hsl = hexToHsl(accentHex);
				setThemeSaturation(hsl.s / 100);
				saveSetting("bloom-theme-saturation", String(hsl.s / 100));
				setThemeBrightness(hsl.l / 100);
				saveSetting("bloom-theme-brightness", String(hsl.l / 100));
			} catch (e) {
				console.error("Failed to parse adaptive accent HSL:", e);
			}
		}
	};

	const handleThemeColorChange = (color: string) => {
		setThemeColor(color);
		saveSetting("bloom-theme-color", color);

		try {
			const hsl = hexToHsl(color);
			setThemeSaturation(hsl.s / 100);
			saveSetting("bloom-theme-saturation", String(hsl.s / 100));
			setThemeBrightness(hsl.l / 100);
			saveSetting("bloom-theme-brightness", String(hsl.l / 100));
		} catch (e) {
			console.error("Failed to parse custom color HSL:", e);
		}
	};

	const handleOpacityChange = (value: number) => {
		setThemeOpacity(value);
		saveSetting("bloom-theme-opacity", String(value));
	};

	const handleSaturationChange = (value: number) => {
		setThemeSaturation(value);
		saveSetting("bloom-theme-saturation", String(value));
	};

	const handleBrightnessChange = (value: number) => {
		setThemeBrightness(value);
		saveSetting("bloom-theme-brightness", String(value));
	};

	// ── Reset to defaults ──
	const resetToDefaults = async () => {
		try {
			const { ask } = await import("@tauri-apps/plugin-dialog");
			const confirmed = await ask(
				"All settings return to their defaults and Bloom restarts. Pinned apps and custom icons are kept.",
				{
					title: "Reset Bloom to Defaults",
					kind: "warning",
					okLabel: "Reset and Restart",
					cancelLabel: "Cancel"
				}
			);
			if (!confirmed) return;

			await invoke("reset_settings");

			// settings.json is cleared, but every window also caches bloom keys
			// in localStorage; drop them so the restart can't resurrect values.
			// The first-run/version sentinels are lifecycle markers, not
			// preferences — keeping them avoids replaying the splash, which
			// would also re-trigger the first-run autostart enable.
			const keep = new Set(["bloom-first-run", "bloom-app-version"]);
			for (const key of Object.keys(localStorage)) {
				if (key.startsWith("bloom-") && !keep.has(key)) localStorage.removeItem(key);
			}

			await invoke("restart_bloom").catch(console.error);
		} catch (e) {
			console.error("Reset failed:", e);
		}
	};

	// ── Export / Import ──
	// The backend opens the native dialog and owns the file path, so these
	// cannot be used to read or write arbitrary files from a webview.
	const handleExportSettings = async () => {
		setExportStatus("exporting");
		try {
			const filePath = await invoke<string | null>("export_settings_to_file");
			if (filePath) {
				setExportStatus("success");
				setTimeout(() => setExportStatus("idle"), 2000);
			} else {
				setExportStatus("idle");
			}
		} catch (e) {
			console.error("Export failed:", e);
			setExportStatus("error");
			setTimeout(() => setExportStatus("idle"), 3000);
		}
	};

	const handleImportSettings = async () => {
		setImportStatus("importing");
		try {
			const filePath = await invoke<string | null>("import_settings_from_file");
			if (filePath) {
				await loadAllSettings();
				setImportStatus("success");
				setTimeout(() => setImportStatus("idle"), 2000);
			} else {
				setImportStatus("idle");
			}
		} catch (e) {
			console.error("Import failed:", e);
			setImportStatus("error");
			setTimeout(() => setImportStatus("idle"), 3000);
		}
	};

	return {
		saveError,
		dismissSaveError,
		// System
		autostart,
		toggleAutostart,
		autoUpdate,
		toggleAutoUpdate,
		lowBatteryThreshold,
		handleThresholdChange,
		timeFormat24h,
		toggleTimeFormat24h,
		showUpdateIndicator,
		toggleUpdateIndicator,
		scale,
		handleScaleChange,
		cornersEnabled,
		toggleCorners,

		// Theme
		themeMode,
		handleThemeModeChange,
		themeColor,
		handleThemeColorChange,
		themeOpacity,
		handleOpacityChange,
		themeSaturation,
		handleSaturationChange,
		themeBrightness,
		handleBrightnessChange,

		// Notch
		notchMode,
		setNotchModeValue,
		notchEdgeDelay,
		handleNotchEdgeDelayChange,
		timerSoundEnabled,
		toggleTimerSound,
		musicModeEnabled,
		toggleMusicMode,
		notchRememberPanel,
		toggleNotchRememberPanel,
		notchDefaultPanel,
		changeNotchDefaultPanel,
		quotaHiddenAgents,
		toggleQuotaAgent,
		musicCompactNotch,
		toggleMusicCompactNotch,
		shelfPersist,
		toggleShelfPersist,
		mediaLayout,
		toggleMediaLayout,
		mediaAmbienceEnabled,
		toggleAmbience,
		mediaCompactGlowEnabled,
		toggleCompactGlow,


		// Widgets
		statusWidgets,
		handleWidgetsChange,

		// Overlays
		volumeOverlayEnabled,
		toggleVolumeOverlay,
		volumeEdgeEnabled,
		toggleVolumeEdge,
		brightnessOverlayEnabled,
		toggleBrightnessOverlay,
		brightnessEdgeEnabled,
		toggleBrightnessEdge,

		// Updates
		updateStatus,
		updateVersion,
		appVersion,
		checkForUpdates,
		installUpdate,

		// Import / Export
		exportStatus,
		importStatus,
		handleExportSettings,
		handleImportSettings,

		// Utilities
		restartBloom: () => invoke("restart_bloom"),
		resetToDefaults,
		quitBloom: () => invoke("quit_bloom")
	};
}
