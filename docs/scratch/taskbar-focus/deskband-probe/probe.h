// Disposable native feasibility probe. Never linked into Agent Quota.
#pragma once
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <shellapi.h>
#include <shlobj.h>
#include <shobjidl.h>
#include <comcat.h>
#include <algorithm>
#include <cstdint>
#include <cstring>
#include <new>

inline constexpr CLSID ProbeCLSID = {0x4f66ca94,0xa701,0x4fc2,{0x8e,0xa5,0xc2,0x15,0x2f,0x37,0x6b,0x90}};
inline constexpr CLSID TrayCLSID = {0xe6442437,0x6c68,0x4f52,{0x94,0xdd,0x2c,0xfe,0xd2,0x67,0xef,0xb9}};
inline constexpr wchar_t ProbeGUID[] = L"{4F66CA94-A701-4FC2-8EA5-C2152F376B90}";
inline constexpr wchar_t BandClass[] = L"AgentQuotaDeskbandProbeBand-4F66CA94";
inline constexpr wchar_t ControllerClass[] = L"AgentQuotaDeskbandProbeController-4F66CA94";
inline constexpr wchar_t PopupClass[] = L"AgentQuotaDeskbandProbePopup-4F66CA94";
inline constexpr UINT ToggleMessage = WM_APP + 101;
inline constexpr UINT PointerMessage = WM_APP + 102;
inline constexpr UINT TraceMessage = WM_APP + 103;
inline constexpr wchar_t WidthProp[] = L"AQProbeFrameWidth";
inline constexpr wchar_t HeightProp[] = L"AQProbeFrameHeight";
inline constexpr wchar_t FitProp[] = L"AQProbeFrameFits";
inline constexpr wchar_t DpiProp[] = L"AQProbeDPI";

inline void Trace(unsigned stage, HRESULT result) {
    HWND observer = FindWindowW(ControllerClass, nullptr);
    if (observer) PostMessageW(observer, TraceMessage,
        (static_cast<WPARAM>(GetCurrentProcessId()) << 32) | stage, static_cast<LPARAM>(result));
}

struct Frame {
    int width = 0, height = 0;
    const BYTE* pixels = nullptr;
};

inline Frame DecodeFrame(const BYTE* data, DWORD size) {
    if (!data || size < 12 || memcmp(data, "AQF1", 4)) return {};
    uint32_t w, h;
    memcpy(&w, data + 4, 4);
    memcpy(&h, data + 8, 4);
    if (!w || !h || w > 2048 || h > 1024 || uint64_t(w) * h * 4 + 12 != size) return {};
    return {int(w), int(h), data + 12};
}

inline int DpiIndex(UINT dpi) {
    switch (dpi) { case 96: return 0; case 120: return 1; case 144: return 2; case 192: return 3; }
    return -1;
}

inline Frame ResourceFrame(HINSTANCE module, UINT dpi, bool popup = false) {
    int index = DpiIndex(dpi);
    if (index < 0) return {};
    auto id = MAKEINTRESOURCEW((popup ? 111 : 101) + index);
    HRSRC resource = FindResourceW(module, id, RT_RCDATA);
    if (!resource) return {};
    HGLOBAL data = LoadResource(module, resource);
    return DecodeFrame(static_cast<const BYTE*>(LockResource(data)), SizeofResource(module, resource));
}

inline bool PaintFrame(HDC dc, RECT rect, Frame frame) {
    FillRect(dc, &rect, static_cast<HBRUSH>(GetStockObject(BLACK_BRUSH)));
    if (!frame.pixels || frame.width > rect.right - rect.left || frame.height > rect.bottom - rect.top) return false;
    BITMAPINFO info{};
    info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    info.bmiHeader.biWidth = frame.width;
    info.bmiHeader.biHeight = -frame.height;
    info.bmiHeader.biPlanes = 1;
    info.bmiHeader.biBitCount = 32;
    info.bmiHeader.biCompression = BI_RGB;
    SetDIBitsToDevice(dc, rect.left + (rect.right - rect.left - frame.width) / 2,
        rect.top + (rect.bottom - rect.top - frame.height) / 2,
        frame.width, frame.height, 0, 0, 0, frame.height, frame.pixels, &info, DIB_RGB_COLORS);
    return true;
}

inline void SetNumber(HWND window, const wchar_t* key, uintptr_t value) {
    SetPropW(window, key, reinterpret_cast<HANDLE>(value));
}

inline HWND FindBand() {
    HWND taskbar = FindWindowW(L"Shell_TrayWnd", nullptr);
    if (!taskbar) return nullptr;
    HWND found = nullptr;
    EnumChildWindows(taskbar, [](HWND child, LPARAM param) -> BOOL {
        wchar_t cls[128]{};
        GetClassNameW(child, cls, 128);
        if (!wcscmp(cls, BandClass)) { *reinterpret_cast<HWND*>(param) = child; return FALSE; }
        return TRUE;
    }, reinterpret_cast<LPARAM>(&found));
    return found;
}
