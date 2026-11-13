#include "probe.h"

static HINSTANCE module;
static LONG liveObjects;

class Band final : public IDeskBand2, public IObjectWithSite, public IPersistStream, public IInputObject {
    LONG refs = 1;
    HWND window = nullptr;
    IUnknown* site = nullptr;
    bool composited = false;

    static LRESULT CALLBACK WindowProc(HWND h, UINT msg, WPARAM w, LPARAM l) {
        Band* self = reinterpret_cast<Band*>(GetWindowLongPtrW(h, GWLP_USERDATA));
        if (msg == WM_NCCREATE) {
            self = static_cast<Band*>(reinterpret_cast<CREATESTRUCTW*>(l)->lpCreateParams);
            self->window = h;
            SetWindowLongPtrW(h, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(self));
        }
        if (!self) return DefWindowProcW(h, msg, w, l);
        switch (msg) {
        case WM_MOUSEACTIVATE: return MA_NOACTIVATE;
        case WM_LBUTTONDOWN: {
            HWND observer = FindWindowW(ControllerClass, nullptr);
            if (observer) PostMessageW(observer, PointerMessage, reinterpret_cast<WPARAM>(h),
                reinterpret_cast<LPARAM>(GetForegroundWindow()));
            return 0;
        }
        case WM_LBUTTONUP: {
            HWND observer = FindWindowW(ControllerClass, nullptr);
            if (observer) PostMessageW(observer, ToggleMessage, reinterpret_cast<WPARAM>(h), 0);
            return 0;
        }
        case WM_ERASEBKGND: return 1;
        case WM_DPICHANGED_AFTERPARENT:
        case WM_SIZE: {
            UINT dpi = GetDpiForWindow(h);
            Frame frame = ResourceFrame(module, dpi);
            RECT r{}; GetClientRect(h, &r);
            SetNumber(h, DpiProp, dpi);
            SetNumber(h, WidthProp, frame.width);
            SetNumber(h, HeightProp, frame.height);
            SetNumber(h, FitProp, frame.pixels && frame.width <= r.right && frame.height <= r.bottom);
            InvalidateRect(h, nullptr, FALSE);
            break;
        }
        case WM_PAINT: {
            PAINTSTRUCT ps{};
            HDC dc = BeginPaint(h, &ps);
            RECT r{}; GetClientRect(h, &r);
            PaintFrame(dc, r, ResourceFrame(module, GetDpiForWindow(h)));
            EndPaint(h, &ps);
            return 0;
        }
        case WM_PRINTCLIENT: {
            RECT r{}; GetClientRect(h, &r);
            PaintFrame(reinterpret_cast<HDC>(w), r, ResourceFrame(module, GetDpiForWindow(h)));
            return 0;
        }
        case WM_NCDESTROY:
            RemovePropW(h, WidthProp); RemovePropW(h, HeightProp);
            RemovePropW(h, DpiProp); RemovePropW(h, FitProp);
            SetWindowLongPtrW(h, GWLP_USERDATA, 0);
            self->window = nullptr;
            break;
        }
        return DefWindowProcW(h, msg, w, l);
    }

public:
    Band() { InterlockedIncrement(&liveObjects); }
    ~Band() {
        if (window) DestroyWindow(window);
        if (site) site->Release();
        if (!InterlockedDecrement(&liveObjects)) UnregisterClassW(BandClass, module);
    }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id == IID_IUnknown || id == IID_IOleWindow || id == IID_IDockingWindow || id == IID_IDeskBand || id == IID_IDeskBand2)
            *out = static_cast<IDeskBand2*>(this);
        else if (id == IID_IObjectWithSite) *out = static_cast<IObjectWithSite*>(this);
        else if (id == IID_IPersist || id == IID_IPersistStream) *out = static_cast<IPersistStream*>(this);
        else if (id == IID_IInputObject) *out = static_cast<IInputObject*>(this);
        if (!*out) return E_NOINTERFACE;
        AddRef(); return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return InterlockedIncrement(&refs); }
    ULONG STDMETHODCALLTYPE Release() override {
        LONG remaining = InterlockedDecrement(&refs);
        if (!remaining) delete this;
        return remaining;
    }
    HRESULT STDMETHODCALLTYPE GetWindow(HWND* out) override { if (!out) return E_POINTER; *out = window; return S_OK; }
    HRESULT STDMETHODCALLTYPE ContextSensitiveHelp(BOOL) override { return E_NOTIMPL; }
    HRESULT STDMETHODCALLTYPE ShowDW(BOOL show) override { if (window) ShowWindow(window, show ? SW_SHOWNOACTIVATE : SW_HIDE); return S_OK; }
    HRESULT STDMETHODCALLTYPE CloseDW(DWORD) override { if (window) DestroyWindow(window); return S_OK; }
    HRESULT STDMETHODCALLTYPE ResizeBorderDW(const RECT*, IUnknown*, BOOL) override { return E_NOTIMPL; }
    HRESULT STDMETHODCALLTYPE GetBandInfo(DWORD, DWORD mode, DESKBANDINFO* info) override {
        Trace(6, S_OK);
        if (!info) return E_POINTER;
        if (mode & DBIF_VIEWMODE_VERTICAL) return E_NOTIMPL;
        UINT dpi = window ? GetDpiForWindow(window) : 96;
        Frame frame = ResourceFrame(module, dpi);
        if (!frame.pixels) return E_FAIL;
        if (info->dwMask & DBIM_MINSIZE) info->ptMinSize = {frame.width, frame.height};
        if (info->dwMask & DBIM_MAXSIZE) info->ptMaxSize = {frame.width, -1};
        if (info->dwMask & DBIM_INTEGRAL) info->ptIntegral = {1, 1};
        if (info->dwMask & DBIM_ACTUAL) info->ptActual = {frame.width, frame.height};
        if (info->dwMask & DBIM_TITLE) info->dwMask &= ~DBIM_TITLE;
        if (info->dwMask & DBIM_MODEFLAGS) info->dwModeFlags = DBIMF_NORMAL | DBIMF_NOGRIPPER;
        info->dwMask &= ~DBIM_BKCOLOR;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE CanRenderComposited(BOOL* out) override { if (!out) return E_POINTER; *out = TRUE; return S_OK; }
    HRESULT STDMETHODCALLTYPE SetCompositionState(BOOL state) override { composited = state != FALSE; if (window) InvalidateRect(window, nullptr, FALSE); return S_OK; }
    HRESULT STDMETHODCALLTYPE GetCompositionState(BOOL* out) override { if (!out) return E_POINTER; *out = composited; return S_OK; }
    HRESULT STDMETHODCALLTYPE SetSite(IUnknown* next) override {
        Trace(3, next ? S_OK : S_FALSE);
        if (window) DestroyWindow(window);
        if (site) { site->Release(); site = nullptr; }
        if (!next) return S_OK;
        IOleWindow* parentSite = nullptr;
        HRESULT hr = next->QueryInterface(IID_PPV_ARGS(&parentSite));
        Trace(4, hr);
        if (FAILED(hr)) return hr;
        HWND parent = nullptr;
        hr = parentSite->GetWindow(&parent);
        Trace(5, hr);
        parentSite->Release();
        if (FAILED(hr) || !parent || !IsWindow(parent)) return E_FAIL;
        WNDCLASSEXW wc{sizeof(wc)};
        wc.lpfnWndProc = WindowProc; wc.hInstance = module;
        wc.lpszClassName = BandClass; wc.hCursor = LoadCursorW(nullptr, IDC_ARROW);
        if (!RegisterClassExW(&wc) && GetLastError() != ERROR_CLASS_ALREADY_EXISTS) return HRESULT_FROM_WIN32(GetLastError());
        HWND created = CreateWindowExW(WS_EX_NOACTIVATE, BandClass, L"Agent Quota deskband probe",
            WS_CHILD | WS_CLIPCHILDREN | WS_CLIPSIBLINGS, 0, 0, 1, 1, parent, nullptr, module, this);
        Trace(7, created ? S_OK : HRESULT_FROM_WIN32(GetLastError()));
        if (!created) return HRESULT_FROM_WIN32(GetLastError());
        next->AddRef(); site = next;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE GetSite(REFIID id, void** out) override { if (!out) return E_POINTER; *out = nullptr; return site ? site->QueryInterface(id, out) : E_FAIL; }
    HRESULT STDMETHODCALLTYPE GetClassID(CLSID* out) override { if (!out) return E_POINTER; *out = ProbeCLSID; return S_OK; }
    HRESULT STDMETHODCALLTYPE IsDirty() override { return S_FALSE; }
    HRESULT STDMETHODCALLTYPE Load(IStream*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE Save(IStream*, BOOL) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE GetSizeMax(ULARGE_INTEGER* out) override { if (!out) return E_POINTER; out->QuadPart = 0; return S_OK; }
    // A readout never takes keyboard focus, even if the shell requests activation.
    HRESULT STDMETHODCALLTYPE UIActivateIO(BOOL, MSG*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE HasFocusIO() override { return S_FALSE; }
    HRESULT STDMETHODCALLTYPE TranslateAcceleratorIO(MSG*) override { return S_FALSE; }
};

class Factory final : public IClassFactory {
    LONG refs = 1;
public:
    Factory() { InterlockedIncrement(&liveObjects); }
    ~Factory() { if (!InterlockedDecrement(&liveObjects)) UnregisterClassW(BandClass, module); }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id != IID_IUnknown && id != IID_IClassFactory) return E_NOINTERFACE;
        *out = static_cast<IClassFactory*>(this); AddRef(); return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return InterlockedIncrement(&refs); }
    ULONG STDMETHODCALLTYPE Release() override { LONG n = InterlockedDecrement(&refs); if (!n) delete this; return n; }
    HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer, REFIID id, void** out) override {
        Trace(2, S_OK);
        if (!out) return E_POINTER;
        *out = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        Band* band = new (std::nothrow) Band;
        if (!band) return E_OUTOFMEMORY;
        HRESULT hr = band->QueryInterface(id, out); band->Release(); return hr;
    }
    HRESULT STDMETHODCALLTYPE LockServer(BOOL lock) override { if (lock) InterlockedIncrement(&liveObjects); else InterlockedDecrement(&liveObjects); return S_OK; }
};

STDAPI DllGetClassObject(REFCLSID cls, REFIID id, void** out) {
    Trace(1, S_OK);
    if (!out) return E_POINTER;
    *out = nullptr;
    if (cls != ProbeCLSID) return CLASS_E_CLASSNOTAVAILABLE;
    Factory* factory = new (std::nothrow) Factory;
    if (!factory) return E_OUTOFMEMORY;
    HRESULT hr = factory->QueryInterface(id, out); factory->Release(); return hr;
}
STDAPI DllCanUnloadNow() { return liveObjects ? S_FALSE : S_OK; }
BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, void*) {
    if (reason == DLL_PROCESS_ATTACH) module = instance;
    return TRUE;
}
