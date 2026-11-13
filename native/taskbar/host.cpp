// The shell host only owns pixels, input and the space Explorer grants it.
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <shlobj.h>
#include <shobjidl.h>
#include <docobj.h>
#include <uxtheme.h>
#include <memory>
#include <new>
#include <algorithm>
#include "protocol.h"
#ifndef AQ_HOST_BUILD
#define AQ_HOST_BUILD 1
#endif

static constexpr CLSID HostCLSID={0xe2219e46,0x1471,0x47b5,{0xbb,0x31,0x65,0x4f,0x89,0x24,0x5b,0x94}};
static constexpr wchar_t HostClass[]=L"AgentQuotaTaskbarHost3";
static constexpr UINT LayoutMessage=WM_APP+1,GeometryMessage=WM_APP+2,DeadMessage=WM_APP+3;
static HINSTANCE module;
static LONG liveObjects;
static UINT inputMessage,readyMessage;

struct HookSlot {HWINEVENTHOOK handle=nullptr;HWND window=nullptr,tracked=nullptr;UINT event=0;uint64_t generation=0;};
static thread_local HookSlot hooks[8];
static void CALLBACK EventProc(HWINEVENTHOOK hook,DWORD event,HWND target,LONG object,LONG child,DWORD,DWORD) {
 if(object!=OBJID_WINDOW || child!=CHILDID_SELF)return;
 for(const auto& slot:hooks)if(slot.handle==hook && slot.event==event && slot.tracked==target) {
  PostMessageW(slot.window,event==EVENT_OBJECT_DESTROY?DeadMessage:GeometryMessage,static_cast<WPARAM>(slot.generation),reinterpret_cast<LPARAM>(hook));
  return;
 }
}
static HWINEVENTHOOK Watch(HWND window,HWND target,UINT event,uint64_t generation) {
 DWORD pid=0;GetWindowThreadProcessId(target,&pid);if(!pid)return nullptr;
 for(auto& slot:hooks)if(!slot.handle) {
  HWINEVENTHOOK hook=SetWinEventHook(event,event,nullptr,EventProc,pid,0,WINEVENT_OUTOFCONTEXT);
  if(!hook)return nullptr;
  slot={hook,window,target,event,generation};InterlockedIncrement(&liveObjects);return hook;
 }
 return nullptr;
}
static void Unwatch(HWINEVENTHOOK& hook) {
 if(!hook)return;
 for(auto& slot:hooks)if(slot.handle==hook){slot={};break;}
 UnhookWinEvent(hook);hook=nullptr;InterlockedDecrement(&liveObjects);
}

class Band final:public IDeskBand2,public IObjectWithSite,public IPersistStream,public IInputObject {
 LONG refs=1;
 HWND window=nullptr,owner=nullptr;
 DWORD ownerPID=0,bandID=0;
 IUnknown* site=nullptr;
 HWINEVENTHOOK ownerHook=nullptr,taskbarHook=nullptr;
 uint64_t generation=0;
 std::unique_ptr<unsigned char[]> pixels;
 uint32_t width=118,height=38,frameDPI=96;
 bool visible=false,composited=false,layoutPending=false,geometryPending=false,pressed=false,dragging=false;
 POINT down{};
 RECT reported{};
 UINT reportedDPI=0;
 bool reportedFit=false;

 void PostGeometry() {if(!geometryPending){geometryPending=true;PostMessageW(window,GeometryMessage,0,0);}}
 void PostLayout() {if(!layoutPending){layoutPending=true;PostMessageW(window,LayoutMessage,0,0);}}
 void Notify(unsigned event,POINT point={}) {
  if(!owner || !generation)return;
  uint64_t packed=event | ((uint64_t(uint32_t(point.x))&0xffffff)<<16) | ((uint64_t(uint32_t(point.y))&0xffffff)<<40);
  PostMessageW(owner,inputMessage,static_cast<WPARAM>(generation),static_cast<LPARAM>(packed));
 }
 bool OwnerValid() const {
  DWORD pid=0;GetWindowThreadProcessId(owner,&pid);
  return owner && IsWindow(owner) && pid==ownerPID;
 }
 void ClearOwner(bool notify,bool schedule=true) {
  if(notify)Notify(9);
  Unwatch(ownerHook);owner=nullptr;ownerPID=0;generation=0;visible=false;pixels.reset();
  RemovePropW(window,L"AQ.Session");RemovePropW(window,L"AQ.Fits");
  if(schedule)PostLayout();
 }
 bool Connect(HWND sender,uint64_t next) {
  DWORD pid=0;GetWindowThreadProcessId(sender,&pid);
  wchar_t cls[64]{};GetClassNameW(sender,cls,64);
  if(!pid || !IsWindow(sender) || wcscmp(cls,L"AgentQuota"))return false;
  if(OwnerValid() && (owner!=sender || ownerPID!=pid))return false;
  Unwatch(ownerHook);
  owner=sender;ownerPID=pid;generation=next;RemovePropW(window,L"AQ.Fits");
  ownerHook=Watch(window,owner,EVENT_OBJECT_DESTROY,generation);
  if(!ownerHook || !OwnerValid()){ClearOwner(false);return false;}
  SetPropW(window,L"AQ.Session",reinterpret_cast<HANDLE>(static_cast<uintptr_t>(generation)));
  reportedDPI=0;
  return true;
 }
 LRESULT Receive(HWND sender,const COPYDATASTRUCT* data) {
  if(!data || data->dwData!=PacketMagic || data->cbData>sizeof(PacketHeader)+MaxWidth*MaxHeight*4)return FALSE;
  Packet packet=DecodePacket(data->lpData,data->cbData);
  if(!packet.valid)return FALSE;
  const auto& h=packet.header;
  if(h.flags&PacketConnect) {if(!Connect(sender,h.generation))return FALSE;}
  if(sender!=owner || h.generation!=generation || !OwnerValid())return FALSE;
  if(h.flags==PacketDisconnect){ClearOwner(false);return TRUE;}
  size_t bytes=size_t(h.width)*h.height*4;
  std::unique_ptr<unsigned char[]> next(new(std::nothrow)unsigned char[bytes]);
  if(!next)return FALSE;
  memcpy(next.get(),packet.pixels,bytes);
  bool resized=width!=h.width || height!=h.height || frameDPI!=h.dpi || visible!=bool(h.flags&PacketVisible);
  pixels=std::move(next);width=h.width;height=h.height;frameDPI=h.dpi;visible=(h.flags&PacketVisible)!=0;
  InvalidateRect(window,nullptr,FALSE);
  // Negotiation is deferred: shell callbacks must see a fully committed frame.
  // A retained shell rectangle still needs an acknowledgement: Go observed the
  // invalidated fit and must be told when this frame's negotiation completes.
  if(resized){RemovePropW(window,L"AQ.Fits");reportedDPI=0;PostLayout();}else PostGeometry();
  return TRUE;
 }
 void Relayout() {
  layoutPending=false;
  if(site) {
   IOleCommandTarget* command=nullptr;
   if(SUCCEEDED(site->QueryInterface(IID_PPV_ARGS(&command)))) {
    VARIANT value{};value.vt=VT_I4;value.lVal=static_cast<LONG>(bandID);
    command->Exec(&CGID_DeskBand,DBID_BANDINFOCHANGED,OLECMDEXECOPT_DODEFAULT,&value,nullptr);
    command->Release();
   }
  }
  PostGeometry();
 }
 void Geometry() {
  geometryPending=false;
  RECT rect{},client{};GetWindowRect(window,&rect);GetClientRect(window,&client);
  UINT dpi=GetDpiForWindow(window);
  RECT rootRect{};GetWindowRect(GetAncestor(window,GA_ROOT),&rootRect);
  bool contained=rect.left>=rootRect.left && rect.top>=rootRect.top && rect.right<=rootRect.right && rect.bottom<=rootRect.bottom;
  bool fits=contained && visible && pixels && dpi==frameDPI && client.right>=static_cast<LONG>(width) && client.bottom>=static_cast<LONG>(height);
  SetPropW(window,L"AQ.Fits",reinterpret_cast<HANDLE>(uintptr_t(fits)));
  SetPropW(window,L"AQ.Width",reinterpret_cast<HANDLE>(uintptr_t(width)));
  SetPropW(window,L"AQ.Height",reinterpret_cast<HANDLE>(uintptr_t(height)));
  SetPropW(window,L"AQ.DPI",reinterpret_cast<HANDLE>(uintptr_t(dpi)));
  SetPropW(window,L"AQ.Visible",reinterpret_cast<HANDLE>(uintptr_t(visible)));
  if(memcmp(&rect,&reported,sizeof(rect)) || dpi!=reportedDPI || fits!=reportedFit) {
   reported=rect;reportedDPI=dpi;reportedFit=fits;Notify(1);
  }
 }
 void Paint(HDC dc) {
  RECT rect{};GetClientRect(window,&rect);
  if(rect.right<=0 || rect.bottom<=0)return;
  HDC target=nullptr;
  HPAINTBUFFER buffer=BeginBufferedPaint(dc,&rect,BPBF_TOPDOWNDIB,nullptr,&target);
  if(!buffer)target=dc;
  // Ask Explorer to paint its own material, including themed/translucent bars.
  HRESULT background=DrawThemeParentBackground(window,target,&rect);
  SetPropW(window,L"AQ.ParentBackground",reinterpret_cast<HANDLE>(uintptr_t(SUCCEEDED(background))));
  if(FAILED(background))FillRect(target,&rect,GetSysColorBrush(COLOR_3DFACE));
  if(visible && !dragging && pixels && GetDpiForWindow(window)==frameDPI && rect.right>=static_cast<LONG>(width) && rect.bottom>=static_cast<LONG>(height)) {
   BITMAPINFO info{};info.bmiHeader.biSize=sizeof(BITMAPINFOHEADER);info.bmiHeader.biWidth=static_cast<LONG>(width);info.bmiHeader.biHeight=-static_cast<LONG>(height);
   info.bmiHeader.biPlanes=1;info.bmiHeader.biBitCount=32;info.bmiHeader.biCompression=BI_RGB;
   void* bits=nullptr;HBITMAP bitmap=CreateDIBSection(target,&info,DIB_RGB_COLORS,&bits,nullptr,0);
   HDC source=bitmap?CreateCompatibleDC(target):nullptr;
   if(bitmap && source) {
    memcpy(bits,pixels.get(),size_t(width)*height*4);
    HGDIOBJ old=SelectObject(source,bitmap);
    BLENDFUNCTION blend{AC_SRC_OVER,0,255,AC_SRC_ALPHA};
    AlphaBlend(target,(rect.right-static_cast<LONG>(width))/2,(rect.bottom-static_cast<LONG>(height))/2,width,height,source,0,0,width,height,blend);
    SelectObject(source,old);
   }
   if(source)DeleteDC(source);if(bitmap)DeleteObject(bitmap);
  }
  if(buffer)EndBufferedPaint(buffer,TRUE);
 }
 static LRESULT CALLBACK WindowProc(HWND h,UINT msg,WPARAM w,LPARAM l) {
  Band* self=reinterpret_cast<Band*>(GetWindowLongPtrW(h,GWLP_USERDATA));
  if(msg==WM_NCCREATE){self=static_cast<Band*>(reinterpret_cast<CREATESTRUCTW*>(l)->lpCreateParams);self->window=h;SetWindowLongPtrW(h,GWLP_USERDATA,reinterpret_cast<LONG_PTR>(self));}
  if(!self)return DefWindowProcW(h,msg,w,l);
  switch(msg) {
  case WM_COPYDATA:return self->Receive(reinterpret_cast<HWND>(w),reinterpret_cast<COPYDATASTRUCT*>(l));
  case WM_MOUSEACTIVATE:return MA_NOACTIVATE;
  case WM_SETCURSOR:SetCursor(LoadCursorW(nullptr,self->dragging?IDC_SIZEALL:IDC_HAND));return TRUE;
  case LayoutMessage:self->Relayout();return 0;
  case GeometryMessage:self->Geometry();return 0;
  case DeadMessage:if(w==self->generation && reinterpret_cast<HWINEVENTHOOK>(l)==self->ownerHook)self->ClearOwner(false);return 0;
  case WM_DPICHANGED_AFTERPARENT:
  case WM_SIZE:self->PostGeometry();InvalidateRect(h,nullptr,FALSE);return 0;
  case WM_THEMECHANGED:
  case WM_SYSCOLORCHANGE:
  case WM_SETTINGCHANGE:
  case WM_DWMCOLORIZATIONCOLORCHANGED:InvalidateRect(h,nullptr,FALSE);return 0;
  case WM_LBUTTONDOWN:
   self->pressed=true;self->dragging=false;GetCursorPos(&self->down);SetCapture(h);self->Notify(3,self->down);return 0;
  case WM_MOUSEMOVE:
   if(self->pressed) {
    POINT p{};GetCursorPos(&p);
    if(!self->dragging && (abs(p.x-self->down.x)+abs(p.y-self->down.y)>4)){self->dragging=true;self->Notify(4,self->down);InvalidateRect(h,nullptr,FALSE);}
    if(self->dragging)self->Notify(5,p);
   }
   return 0;
  case WM_LBUTTONUP:
   if(self->pressed) {
    POINT p{};GetCursorPos(&p);bool moved=self->dragging;
    self->pressed=self->dragging=false;ReleaseCapture();
    self->Notify(moved?6:2,p);InvalidateRect(h,nullptr,FALSE);
   }
   return 0;
  case WM_CAPTURECHANGED:
   if(self->pressed){self->pressed=self->dragging=false;self->Notify(7);InvalidateRect(h,nullptr,FALSE);}return 0;
  case WM_RBUTTONUP:{POINT p{};GetCursorPos(&p);self->Notify(8,p);return 0;}
  case WM_ERASEBKGND:return 1;
  case WM_PAINT:{PAINTSTRUCT ps{};HDC dc=BeginPaint(h,&ps);self->Paint(dc);EndPaint(h,&ps);return 0;}
  case WM_PRINTCLIENT:self->Paint(reinterpret_cast<HDC>(w));return 0;
  case WM_NCDESTROY:
   self->ClearOwner(true,false);Unwatch(self->taskbarHook);self->layoutPending=self->geometryPending=false;
   for(const auto* key:{L"AQ.Build",L"AQ.Protocol",L"AQ.Fits",L"AQ.Width",L"AQ.Height",L"AQ.DPI",L"AQ.Visible",L"AQ.Session",L"AQ.ParentBackground"})RemovePropW(h,key);
   self->window=nullptr;SetWindowLongPtrW(h,GWLP_USERDATA,0);break;
  }
  return DefWindowProcW(h,msg,w,l);
 }
public:
 Band(){BufferedPaintInit();InterlockedIncrement(&liveObjects);}
 ~Band(){if(window)DestroyWindow(window);if(site)site->Release();BufferedPaintUnInit();if(!InterlockedDecrement(&liveObjects))UnregisterClassW(HostClass,module);}
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void** out)override {
  if(!out)return E_POINTER;*out=nullptr;
  if(id==IID_IUnknown || id==IID_IOleWindow || id==IID_IDockingWindow || id==IID_IDeskBand || id==IID_IDeskBand2)*out=static_cast<IDeskBand2*>(this);
  else if(id==IID_IObjectWithSite)*out=static_cast<IObjectWithSite*>(this);
  else if(id==IID_IPersist || id==IID_IPersistStream)*out=static_cast<IPersistStream*>(this);
  else if(id==IID_IInputObject)*out=static_cast<IInputObject*>(this);
  if(!*out)return E_NOINTERFACE;AddRef();return S_OK;
 }
 ULONG STDMETHODCALLTYPE AddRef()override{return InterlockedIncrement(&refs);}
 ULONG STDMETHODCALLTYPE Release()override{LONG n=InterlockedDecrement(&refs);if(!n)delete this;return n;}
 HRESULT STDMETHODCALLTYPE GetWindow(HWND* out)override{if(!out)return E_POINTER;*out=window;return S_OK;}
 HRESULT STDMETHODCALLTYPE ContextSensitiveHelp(BOOL)override{return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE ShowDW(BOOL show)override{if(window)ShowWindow(window,show?SW_SHOWNOACTIVATE:SW_HIDE);return S_OK;}
 HRESULT STDMETHODCALLTYPE CloseDW(DWORD)override{if(window)DestroyWindow(window);return S_OK;}
 HRESULT STDMETHODCALLTYPE ResizeBorderDW(const RECT*,IUnknown*,BOOL)override{return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE GetBandInfo(DWORD id,DWORD mode,DESKBANDINFO* info)override{
  if(!info)return E_POINTER;if(mode&DBIF_VIEWMODE_VERTICAL)return E_NOTIMPL;bandID=id;
  UINT dpi=window?GetDpiForWindow(window):96;
  RECT rootRect{};if(window)GetWindowRect(GetAncestor(window,GA_ROOT),&rootRect);
  bool usable=visible && MulDiv(static_cast<int>(height),static_cast<int>(dpi),static_cast<int>(frameDPI))<=rootRect.bottom-rootRect.top;
  int wanted=usable?MulDiv(static_cast<int>(width),static_cast<int>(dpi),static_cast<int>(frameDPI)):0;
  int wantedHeight=usable?MulDiv(static_cast<int>(height),static_cast<int>(dpi),static_cast<int>(frameDPI)):1;
  if(info->dwMask&DBIM_MINSIZE)info->ptMinSize={wanted,wantedHeight};
  if(info->dwMask&DBIM_MAXSIZE)info->ptMaxSize={wanted,-1};
  if(info->dwMask&DBIM_ACTUAL)info->ptActual={wanted,wantedHeight};
  if(info->dwMask&DBIM_INTEGRAL)info->ptIntegral={1,1};
  if(info->dwMask&DBIM_TITLE)info->dwMask&=~DBIM_TITLE;
  if(info->dwMask&DBIM_MODEFLAGS)info->dwModeFlags=DBIMF_NORMAL|DBIMF_NOGRIPPER;
  info->dwMask&=~DBIM_BKCOLOR;return S_OK;
 }
 HRESULT STDMETHODCALLTYPE CanRenderComposited(BOOL* out)override{if(!out)return E_POINTER;*out=TRUE;return S_OK;}
 HRESULT STDMETHODCALLTYPE SetCompositionState(BOOL value)override{composited=value!=FALSE;if(window)InvalidateRect(window,nullptr,FALSE);return S_OK;}
 HRESULT STDMETHODCALLTYPE GetCompositionState(BOOL* out)override{if(!out)return E_POINTER;*out=composited;return S_OK;}
 HRESULT STDMETHODCALLTYPE SetSite(IUnknown* next)override{
  if(window)DestroyWindow(window);layoutPending=geometryPending=false;if(site){site->Release();site=nullptr;}if(!next)return S_OK;
  inputMessage=RegisterWindowMessageW(L"AgentQuota.Taskbar.Input.1");readyMessage=RegisterWindowMessageW(L"AgentQuota.Taskbar.Ready.1");
  if(!inputMessage || !readyMessage)return E_FAIL;
  IOleWindow* parentSite=nullptr;HRESULT hr=next->QueryInterface(IID_PPV_ARGS(&parentSite));if(FAILED(hr))return hr;
  HWND parent=nullptr;hr=parentSite->GetWindow(&parent);parentSite->Release();if(FAILED(hr)||!IsWindow(parent))return E_FAIL;
  WNDCLASSEXW wc{sizeof(wc)};wc.lpfnWndProc=WindowProc;wc.hInstance=module;wc.lpszClassName=HostClass;
  if(!RegisterClassExW(&wc)&&GetLastError()!=ERROR_CLASS_ALREADY_EXISTS)return HRESULT_FROM_WIN32(GetLastError());
  if(!CreateWindowExW(WS_EX_NOACTIVATE,HostClass,L"Agent Quota",WS_CHILD|WS_CLIPSIBLINGS|WS_CLIPCHILDREN,0,0,0,1,parent,nullptr,module,this))return HRESULT_FROM_WIN32(GetLastError());
  next->AddRef();site=next;
  SetPropW(window,L"AQ.Build",reinterpret_cast<HANDLE>(uintptr_t(AQ_HOST_BUILD)));
  SetPropW(window,L"AQ.Protocol",reinterpret_cast<HANDLE>(uintptr_t(ProtocolVersion)));
  HWND root=GetAncestor(window,GA_ROOT);
  taskbarHook=Watch(window,root,EVENT_OBJECT_LOCATIONCHANGE,0);
  if(!taskbarHook){DestroyWindow(window);site->Release();site=nullptr;return E_FAIL;}
  PostMessageW(HWND_BROADCAST,readyMessage,AQ_HOST_BUILD,reinterpret_cast<LPARAM>(window));
  PostGeometry();return S_OK;
 }
 HRESULT STDMETHODCALLTYPE GetSite(REFIID id,void** out)override{if(!out)return E_POINTER;*out=nullptr;return site?site->QueryInterface(id,out):E_FAIL;}
 HRESULT STDMETHODCALLTYPE GetClassID(CLSID* out)override{if(!out)return E_POINTER;*out=HostCLSID;return S_OK;}
 HRESULT STDMETHODCALLTYPE IsDirty()override{return S_FALSE;}
 HRESULT STDMETHODCALLTYPE Load(IStream*)override{return S_OK;}
 HRESULT STDMETHODCALLTYPE Save(IStream*,BOOL)override{return S_OK;}
 HRESULT STDMETHODCALLTYPE GetSizeMax(ULARGE_INTEGER* out)override{if(!out)return E_POINTER;out->QuadPart=0;return S_OK;}
 HRESULT STDMETHODCALLTYPE UIActivateIO(BOOL,MSG*)override{return S_OK;}
 HRESULT STDMETHODCALLTYPE HasFocusIO()override{return S_FALSE;}
 HRESULT STDMETHODCALLTYPE TranslateAcceleratorIO(MSG*)override{return S_FALSE;}
};

class Factory final:public IClassFactory {
 LONG refs=1;
public:
 Factory(){InterlockedIncrement(&liveObjects);}
 ~Factory(){if(!InterlockedDecrement(&liveObjects))UnregisterClassW(HostClass,module);}
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void** out)override{if(!out)return E_POINTER;*out=nullptr;if(id!=IID_IUnknown&&id!=IID_IClassFactory)return E_NOINTERFACE;*out=static_cast<IClassFactory*>(this);AddRef();return S_OK;}
 ULONG STDMETHODCALLTYPE AddRef()override{return InterlockedIncrement(&refs);}
 ULONG STDMETHODCALLTYPE Release()override{LONG n=InterlockedDecrement(&refs);if(!n)delete this;return n;}
 HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer,REFIID id,void** out)override{if(!out)return E_POINTER;*out=nullptr;if(outer)return CLASS_E_NOAGGREGATION;Band* b=new(std::nothrow)Band;if(!b)return E_OUTOFMEMORY;HRESULT hr=b->QueryInterface(id,out);b->Release();return hr;}
 HRESULT STDMETHODCALLTYPE LockServer(BOOL lock)override{if(lock)InterlockedIncrement(&liveObjects);else InterlockedDecrement(&liveObjects);return S_OK;}
};
STDAPI DllGetClassObject(REFCLSID cls,REFIID id,void** out){if(!out)return E_POINTER;*out=nullptr;if(cls!=HostCLSID)return CLASS_E_CLASSNOTAVAILABLE;Factory* f=new(std::nothrow)Factory;if(!f)return E_OUTOFMEMORY;HRESULT hr=f->QueryInterface(id,out);f->Release();return hr;}
STDAPI DllCanUnloadNow(){return liveObjects?S_FALSE:S_OK;}
BOOL WINAPI DllMain(HINSTANCE h,DWORD reason,void*){if(reason==DLL_PROCESS_ATTACH){module=h;}return TRUE;}
