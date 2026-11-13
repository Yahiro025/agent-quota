#include "probe.h"
#include <cstdio>
#include <string>
#include <vector>

static HINSTANCE instance;
static HWND controller, popup, editor, logBox;
static FILE* logFile;
static HWND mouseDownForeground;
static unsigned popupToggles;

static std::wstring Sibling(const wchar_t* name) {
    wchar_t path[32768]{};
    DWORD n = GetModuleFileNameW(nullptr, path, 32768);
    if (!n || n >= 32768) return {};
    std::wstring result(path, n);
    size_t slash = result.find_last_of(L"\\/");
    return result.substr(0, slash + 1) + name;
}

static std::wstring RegistryPath() { return std::wstring(L"Software\\Classes\\CLSID\\") + ProbeGUID; }

static HRESULT TrayOperation(const wchar_t* action) {
    ITrayDeskBand* tray = nullptr;
    HRESULT hr = CoCreateInstance(TrayCLSID, nullptr, CLSCTX_INPROC_SERVER | CLSCTX_LOCAL_SERVER, IID_PPV_ARGS(&tray));
    Trace(20, hr);
    if (FAILED(hr)) return hr;
    if (!wcscmp(action, L"show")) hr = tray->ShowDeskBand(ProbeCLSID);
    else if (!wcscmp(action, L"hide")) hr = tray->HideDeskBand(ProbeCLSID);
    else if (!wcscmp(action, L"query")) hr = tray->IsDeskBandShown(ProbeCLSID);
    else hr = tray->DeskBandRegistrationChanged();
    Trace(21, hr);
    tray->Release();
    return hr;
}

static LSTATUS SetText(HKEY root, const wchar_t* name, const std::wstring& value) {
    return RegSetValueExW(root, name, 0, REG_SZ, reinterpret_cast<const BYTE*>(value.c_str()), DWORD((value.size() + 1) * sizeof(wchar_t)));
}

static int RegisterProbe(bool remove) {
    std::wstring rootPath = RegistryPath();
    std::wstring dll = Sibling(L"probe-band.dll");
    if (dll.empty()) return 1;
    HKEY root = nullptr, child = nullptr;
    LSTATUS status;
    if (remove) {
        wchar_t registered[32768]{};
        DWORD bytes = sizeof(registered);
        status = RegGetValueW(HKEY_CURRENT_USER, (rootPath + L"\\InprocServer32").c_str(), nullptr, RRF_RT_REG_SZ, nullptr, registered, &bytes);
        if (status == ERROR_FILE_NOT_FOUND) { puts("Probe registration already absent"); return 0; }
        if (status || dll != registered) { puts("Refusing to remove an unexpected registration"); return 1; }
        status = RegDeleteTreeW(HKEY_CURRENT_USER, rootPath.c_str());
        printf("Unregister per-user probe: %ld\n", status);
        if (!status) {
            SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST | SHCNF_FLUSHNOWAIT, nullptr, nullptr);
            TrayOperation(L"refresh");
        }
        return status ? 1 : 0;
    }
    if (GetFileAttributesW(dll.c_str()) == INVALID_FILE_ATTRIBUTES) { puts("Probe DLL missing"); return 1; }
    DWORD disposition = 0;
    status = RegCreateKeyExW(HKEY_CURRENT_USER, rootPath.c_str(), 0, nullptr, 0, KEY_WRITE | KEY_READ, nullptr, &root, &disposition);
    if (status) return 1;
    if (disposition != REG_CREATED_NEW_KEY) { RegCloseKey(root); puts("Probe class already registered; explicit cleanup required"); return 1; }
    status = SetText(root, nullptr, L"Agent Quota disposable deskband probe");
    if (!status) status = RegCreateKeyExW(root, L"InprocServer32", 0, nullptr, 0, KEY_WRITE, nullptr, &child, nullptr);
    if (!status) status = SetText(child, nullptr, dll);
    if (!status) status = SetText(child, L"ThreadingModel", L"Apartment");
    if (child) { RegCloseKey(child); child = nullptr; }
    if (!status) status = RegCreateKeyExW(root, L"Implemented Categories\\{00021492-0000-0000-C000-000000000046}", 0, nullptr, 0, KEY_WRITE, nullptr, &child, nullptr);
    if (child) RegCloseKey(child);
    RegCloseKey(root);
    if (status) RegDeleteTreeW(HKEY_CURRENT_USER, rootPath.c_str());
    else {
        SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST | SHCNF_FLUSHNOWAIT, nullptr, nullptr);
        TrayOperation(L"refresh");
    }
    printf("Register per-user probe: %ld\n", status);
    return status ? 1 : 0;
}

static int Categories() {
    ICatInformation* info = nullptr;
    HRESULT hr = CoCreateInstance(CLSID_StdComponentCategoriesMgr, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&info));
    if (FAILED(hr)) return 1;
    CATID category = CATID_DeskBand;
    IEnumCLSID* classes = nullptr;
    hr = info->EnumClassesOfCategories(1, &category, 0, nullptr, &classes);
    bool found = false;
    if (classes) {
        CLSID cls{};
        while (classes->Next(1, &cls, nullptr) == S_OK) if (cls == ProbeCLSID) found = true;
        classes->Release();
    }
    info->Release();
    printf("Deskband category discoverable=%s HRESULT=0x%08lX\n", found ? "true" : "false", static_cast<unsigned long>(hr));
    return found ? 0 : 1;
}

static void PrintRect(RECT r) { printf("[%ld,%ld,%ld,%ld]", r.left, r.top, r.right, r.bottom); }

static void Snapshot(bool queryCOM = true) {
    HWND taskbar = FindWindowW(L"Shell_TrayWnd", nullptr);
    APPBARDATA bar{sizeof(bar)};
    bar.hWnd = taskbar;
    SHAppBarMessage(ABM_GETTASKBARPOS, &bar);
    UINT state = UINT(SHAppBarMessage(ABM_GETSTATE, &bar));
    RECT work{}; SystemParametersInfoW(SPI_GETWORKAREA, 0, &work, 0);
    RECT actual{}; if (taskbar) GetWindowRect(taskbar, &actual);
    printf("{\"taskbar\":"); PrintRect(bar.rc);
    printf(",\"taskbar_window\":"); PrintRect(actual);
    printf(",\"work\":"); PrintRect(work);
    printf(",\"appbar_state\":%u,\"auto_hide\":%s", state, (state & ABS_AUTOHIDE) ? "true" : "false");
    if (queryCOM) printf(",\"band_shown_hresult\":%ld", TrayOperation(L"query"));
    printf(",\"nodes\":[");
    bool first = true;
    struct Context { bool* first; } context{&first};
    if (taskbar) EnumChildWindows(taskbar, [](HWND h, LPARAM p) -> BOOL {
        auto ctx = reinterpret_cast<Context*>(p);
        wchar_t cls[128]{}; GetClassNameW(h, cls, 128);
        if (wcscmp(cls, BandClass) && wcscmp(cls, L"MSTaskSwWClass") && wcscmp(cls, L"MSTaskListWClass") && wcscmp(cls, L"TrayNotifyWnd") && wcscmp(cls, L"ReBarWindow32") && wcscmp(cls, L"TrayClockWClass")) return TRUE;
        if (!*ctx->first) printf(",");
        *ctx->first = false;
        RECT r{}; GetWindowRect(h, &r);
        printf("{\"class\":\"%ls\",\"hwnd\":%llu,\"rect\":", cls, static_cast<unsigned long long>(reinterpret_cast<uintptr_t>(h))); PrintRect(r);
        printf(",\"visible\":%s,\"dpi\":%u", IsWindowVisible(h) ? "true" : "false", GetDpiForWindow(h));
        if (!wcscmp(cls, BandClass)) {
            printf(",\"frame\":[%llu,%llu],\"fits\":%s,\"ancestors\":[", static_cast<unsigned long long>(reinterpret_cast<uintptr_t>(GetPropW(h, WidthProp))), static_cast<unsigned long long>(reinterpret_cast<uintptr_t>(GetPropW(h, HeightProp))), GetPropW(h, FitProp) ? "true" : "false");
            HWND parent = GetParent(h);
            for (int i = 0; parent && i < 8; ++i) {
                if (i) printf(",");
                RECT ancestor{}; GetWindowRect(parent, &ancestor); PrintRect(ancestor);
                parent = GetParent(parent);
            }
            printf("]");
        }
        printf("}"); return TRUE;
    }, reinterpret_cast<LPARAM>(&context));
    puts("]}");
}

static void Log(const wchar_t* event, HWND before = nullptr, HWND after = nullptr) {
    wchar_t line[320]{};
    int length = editor ? GetWindowTextLengthW(editor) : 0;
    swprintf_s(line, L"%llu %s foreground=%llu->%llu edit_chars=%d toggles=%u\r\n",
        static_cast<unsigned long long>(GetTickCount64()), event,
        static_cast<unsigned long long>(reinterpret_cast<uintptr_t>(before)),
        static_cast<unsigned long long>(reinterpret_cast<uintptr_t>(after)), length, popupToggles);
    if (logBox) {
        int end = GetWindowTextLengthW(logBox);
        SendMessageW(logBox, EM_SETSEL, end, end);
        SendMessageW(logBox, EM_REPLACESEL, FALSE, reinterpret_cast<LPARAM>(line));
    }
    if (logFile) { fwprintf(logFile, L"%s", line); fflush(logFile); }
}

static LRESULT CALLBACK PopupProc(HWND h, UINT msg, WPARAM w, LPARAM l) {
    switch (msg) {
    case WM_MOUSEACTIVATE: return MA_NOACTIVATE;
    case WM_ERASEBKGND: return 1;
    case WM_LBUTTONUP: ShowWindow(h, SW_HIDE); return 0;
    case WM_PAINT: {
        PAINTSTRUCT ps{}; HDC dc = BeginPaint(h, &ps);
        RECT r{}; GetClientRect(h, &r);
        PaintFrame(dc, r, ResourceFrame(instance, GetDpiForWindow(h), true));
        EndPaint(h, &ps); return 0;
    }
    }
    return DefWindowProcW(h, msg, w, l);
}

static void Toggle(HWND band, bool synthetic) {
    if (!band || band != FindBand()) return;
    HWND before = GetForegroundWindow();
    if (IsWindowVisible(popup)) ShowWindow(popup, SW_HIDE);
    else {
        UINT dpi = GetDpiForWindow(band);
        Frame frame = ResourceFrame(instance, dpi, true);
        if (!frame.pixels) return;
        RECT anchor{}, work{}; GetWindowRect(band, &anchor);
        SystemParametersInfoW(SPI_GETWORKAREA, 0, &work, 0);
        int x = std::clamp(int(anchor.right) - frame.width, int(work.left), std::max(int(work.left), int(work.right) - frame.width));
        int y = std::max(int(work.top), int(anchor.top) - frame.height);
        SetWindowPos(popup, HWND_TOPMOST, x, y, frame.width, frame.height, SWP_NOACTIVATE | SWP_SHOWWINDOW);
    }
    ++popupToggles;
    Log(synthetic ? L"synthetic_popup_toggle" : L"band_button_up", before, GetForegroundWindow());
    if (!synthetic && mouseDownForeground) Log(L"down_to_popup_focus", mouseDownForeground, GetForegroundWindow());
}

static LRESULT CALLBACK ControllerProc(HWND h, UINT msg, WPARAM w, LPARAM l) {
    switch (msg) {
    case WM_CREATE:
        CreateWindowW(L"STATIC", L"DISPOSABLE DESKBAND PROBE — frozen quota values, existing renderer\r\nType in the focus sentinel, then click the taskbar readout. The popup should leave typing here.", WS_CHILD | WS_VISIBLE, 18, 15, 850, 47, h, nullptr, instance, nullptr);
        editor = CreateWindowExW(WS_EX_CLIENTEDGE, L"EDIT", L"", WS_CHILD | WS_VISIBLE | WS_TABSTOP | ES_AUTOHSCROLL, 18, 72, 830, 32, h, reinterpret_cast<HMENU>(100), instance, nullptr);
        logBox = CreateWindowExW(WS_EX_CLIENTEDGE, L"EDIT", L"", WS_CHILD | WS_VISIBLE | ES_MULTILINE | ES_READONLY | WS_VSCROLL, 18, 122, 830, 330, h, nullptr, instance, nullptr);
        return 0;
    case WM_COMMAND:
        if (LOWORD(w) == 100 && HIWORD(w) == EN_CHANGE) Log(L"sentinel_changed", GetForegroundWindow(), GetForegroundWindow());
        return 0;
    case PointerMessage:
        if (reinterpret_cast<HWND>(w) == FindBand()) {
            mouseDownForeground = reinterpret_cast<HWND>(l);
            Log(L"band_button_down", mouseDownForeground, GetForegroundWindow());
        }
        return 0;
    case ToggleMessage: Toggle(reinterpret_cast<HWND>(w), l != 0); return 0;
    case TraceMessage: {
        wchar_t event[96]{};
        swprintf_s(event, L"native_pid_%u_stage_%u_hr_%08lX", static_cast<unsigned>(w >> 32), static_cast<unsigned>(w), static_cast<unsigned long>(l));
        Log(event); return 0;
    }
    case WM_CLOSE: DestroyWindow(h); return 0;
    case WM_DESTROY:
        if (popup) DestroyWindow(popup);
        Log(L"controller_closed");
        PostQuitMessage(0); return 0;
    }
    return DefWindowProcW(h, msg, w, l);
}

static int RunController() {
    if (FindWindowW(ControllerClass, nullptr)) { puts("Probe controller already running"); return 1; }
    _wfopen_s(&logFile, Sibling(L"events.log").c_str(), L"w, ccs=UTF-8");
    WNDCLASSEXW wc{sizeof(wc)};
    wc.hInstance = instance; wc.hCursor = LoadCursorW(nullptr, IDC_ARROW);
    wc.hbrBackground = reinterpret_cast<HBRUSH>(COLOR_WINDOW + 1);
    wc.lpfnWndProc = ControllerProc; wc.lpszClassName = ControllerClass;
    if (!RegisterClassExW(&wc)) return 1;
    wc.lpfnWndProc = PopupProc; wc.lpszClassName = PopupClass;
    if (!RegisterClassExW(&wc)) return 1;
    popup = CreateWindowExW(WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE | WS_EX_TOPMOST, PopupClass, L"Agent Quota disposable popup", WS_POPUP, 0, 0, 10, 10, nullptr, nullptr, instance, nullptr);
    controller = CreateWindowExW(0, ControllerClass, L"Agent Quota — deskband probe", WS_OVERLAPPEDWINDOW, 120, 140, 900, 520, nullptr, nullptr, instance, nullptr);
    if (!popup || !controller) return 1;
    SetWindowPos(controller, nullptr, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER | SWP_NOACTIVATE | SWP_SHOWWINDOW);
    Log(L"controller_ready");
    MSG msg{};
    while (GetMessageW(&msg, nullptr, 0, 0) > 0) { TranslateMessage(&msg); DispatchMessageW(&msg); }
    if (logFile) fclose(logFile);
    return 0;
}

class TestSite final : public IOleWindow {
    LONG refs = 1;
    HWND window;
public:
    explicit TestSite(HWND h) : window(h) {}
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id != IID_IUnknown && id != IID_IOleWindow) return E_NOINTERFACE;
        *out = static_cast<IOleWindow*>(this); AddRef(); return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return InterlockedIncrement(&refs); }
    ULONG STDMETHODCALLTYPE Release() override { LONG n = InterlockedDecrement(&refs); if (!n) delete this; return n; }
    HRESULT STDMETHODCALLTYPE GetWindow(HWND* out) override { if (!out) return E_POINTER; *out = window; return S_OK; }
    HRESULT STDMETHODCALLTYPE ContextSensitiveHelp(BOOL) override { return E_NOTIMPL; }
};

static int SelfTest() {
    int failures = 0;
    auto check = [&](bool ok, const char* label) { printf("%s %s\n", ok ? "PASS" : "FAIL", label); if (!ok) ++failures; };
    BYTE invalid[16]{};
    check(!DecodeFrame(nullptr, 0).pixels && !DecodeFrame(invalid, sizeof(invalid)).pixels, "reject invalid frame header");
    memcpy(invalid, "AQF1", 4); uint32_t huge = 0xffffffff; memcpy(invalid + 4, &huge, 4);
    check(!DecodeFrame(invalid, sizeof(invalid)).pixels, "reject overflowing frame dimensions");
    for (UINT dpi : {96u,120u,144u,192u}) {
        check(ResourceFrame(instance, dpi).pixels && ResourceFrame(instance, dpi, true).pixels, "embedded readout and popup decode");
    }
    HMODULE dll = LoadLibraryW(Sibling(L"probe-band.dll").c_str());
    check(dll != nullptr, "native deskband loads outside Explorer");
    if (!dll) return 1;
    auto getFactory = reinterpret_cast<HRESULT(WINAPI*)(REFCLSID, REFIID, void**)>(GetProcAddress(dll, "DllGetClassObject"));
    auto canUnload = reinterpret_cast<HRESULT(WINAPI*)()>(GetProcAddress(dll, "DllCanUnloadNow"));
    check(getFactory && canUnload, "COM exports present");
    if (!getFactory || !canUnload) { FreeLibrary(dll); return 1; }
    IClassFactory* factory = nullptr;
    HRESULT hr = getFactory(ProbeCLSID, IID_PPV_ARGS(&factory));
    check(SUCCEEDED(hr), "class factory activation");
    if (!factory) { FreeLibrary(dll); return 1; }
    IDeskBand2* band = nullptr;
    hr = factory->CreateInstance(nullptr, IID_PPV_ARGS(&band));
    check(SUCCEEDED(hr), "deskband activation");
    if (band) {
        HWND host = CreateWindowExW(WS_EX_NOACTIVATE, L"STATIC", L"Probe isolated host", WS_POPUP, 0, 0, 400, 100, nullptr, nullptr, instance, nullptr);
        TestSite* site = new TestSite(host);
        IObjectWithSite* attached = nullptr;
        band->QueryInterface(IID_PPV_ARGS(&attached));
        check(attached && SUCCEEDED(attached->SetSite(site)), "real child attaches to isolated host");
        HWND child = nullptr; band->GetWindow(&child);
        check(child && GetParent(child) == host, "child belongs to supplied host");
        DESKBANDINFO info{}; info.dwMask = DBIM_MINSIZE | DBIM_ACTUAL | DBIM_TITLE | DBIM_MODEFLAGS;
        check(SUCCEEDED(band->GetBandInfo(1, DBIF_VIEWMODE_NORMAL, &info)) && !(info.dwMask & DBIM_TITLE), "reports real size and suppresses title");
        SetWindowPos(child, nullptr, 0, 0, info.ptMinSize.x, info.ptMinSize.y, SWP_NOACTIVATE | SWP_NOZORDER);
        check(GetPropW(child, FitProp) != nullptr, "readout fits native child without resampling");
        check(SendMessageW(child, WM_MOUSEACTIVATE, reinterpret_cast<WPARAM>(host), MAKELPARAM(HTCLIENT, WM_LBUTTONDOWN)) == MA_NOACTIVATE, "mouse activation refused");
        IInputObject* input = nullptr; band->QueryInterface(IID_PPV_ARGS(&input));
        HWND before = GetForegroundWindow();
        if (input) { input->UIActivateIO(TRUE, nullptr); input->Release(); }
        band->ShowDW(TRUE);
        check(before == GetForegroundWindow(), "shell activation and showing preserve foreground");
        band->CloseDW(0);
        if (attached) { attached->SetSite(nullptr); attached->Release(); }
        site->Release(); DestroyWindow(host); band->Release();
    }
    factory->Release();
    check(canUnload() == S_OK, "COM objects and native child release cleanly");
    FreeLibrary(dll);
    return failures ? 1 : 0;
}

static int AutoHideCheck() {
    APPBARDATA bar{sizeof(bar)};
    bar.hWnd = FindWindowW(L"Shell_TrayWnd", nullptr);
    if (!bar.hWnd) return 1;
    UINT original = UINT(SHAppBarMessage(ABM_GETSTATE, &bar));
    bar.lParam = original | ABS_AUTOHIDE;
    SHAppBarMessage(ABM_SETSTATE, &bar);
    Sleep(1800);
    bool enabled = (SHAppBarMessage(ABM_GETSTATE, &bar) & ABS_AUTOHIDE) != 0;
    // No COM call between changing the shell state and restoring it.
    Snapshot(false);
    bar.lParam = original;
    SHAppBarMessage(ABM_SETSTATE, &bar);
    Sleep(1000);
    puts("RESTORED"); Snapshot();
    return enabled && UINT(SHAppBarMessage(ABM_GETSTATE, &bar)) == original ? 0 : 1;
}

int wmain(int argc, wchar_t** argv) {
    instance = GetModuleHandleW(nullptr);
    SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) return 1;
    int result = 0;
    const wchar_t* command = argc > 1 ? argv[1] : L"status";
    if (!wcscmp(command, L"register")) result = RegisterProbe(false);
    else if (!wcscmp(command, L"unregister")) result = RegisterProbe(true);
    else if (!wcscmp(command, L"selftest")) result = SelfTest();
    else if (!wcscmp(command, L"categories")) result = Categories();
    else if (!wcscmp(command, L"activate")) {
        IDeskBand2* band = nullptr;
        hr = CoCreateInstance(ProbeCLSID, nullptr, CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&band));
        printf("Probe CoCreateInstance HRESULT=0x%08lX\n", static_cast<unsigned long>(hr));
        if (band) band->Release();
        result = FAILED(hr) ? 1 : 0;
    }
    else if (!wcscmp(command, L"run")) result = RunController();
    else if (!wcscmp(command, L"status")) Snapshot();
    else if (!wcscmp(command, L"autohide-check")) result = AutoHideCheck();
    else if (!wcscmp(command, L"restore-state") && argc == 3) {
        wchar_t* end = nullptr;
        unsigned long state = wcstoul(argv[2], &end, 10);
        if (!end || *end || state > (ABS_AUTOHIDE | ABS_ALWAYSONTOP)) result = 1;
        else {
            APPBARDATA bar{sizeof(bar)};
            bar.hWnd = FindWindowW(L"Shell_TrayWnd", nullptr); bar.lParam = state;
            if (!bar.hWnd) result = 1;
            else { SHAppBarMessage(ABM_SETSTATE, &bar); result = SHAppBarMessage(ABM_GETSTATE, &bar) == state ? 0 : 1; }
        }
    }
    else if (!wcscmp(command, L"popup") || !wcscmp(command, L"quit")) {
        HWND h = FindWindowW(ControllerClass, nullptr);
        if (!h) result = 1;
        else if (!wcscmp(command, L"quit")) PostMessageW(h, WM_CLOSE, 0, 0);
        else PostMessageW(h, ToggleMessage, reinterpret_cast<WPARAM>(FindBand()), 1);
    } else if (!wcscmp(command, L"show") || !wcscmp(command, L"hide") || !wcscmp(command, L"query")) {
        hr = TrayOperation(command);
        printf("%ls HRESULT=0x%08lX\n", command, static_cast<unsigned long>(hr));
        result = FAILED(hr) ? 1 : 0;
    } else { puts("Commands: selftest register show hide query status run popup quit unregister autohide-check"); result = 1; }
    CoUninitialize();
    return result;
}
