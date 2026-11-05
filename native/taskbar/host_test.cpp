#define AQ_HOST_BUILD 1
#include "host.cpp"
#include <cstdio>
#include <cstdlib>
#include <vector>

static void Check(bool ok,const char* what){if(!ok){printf("FAIL %s\n",what);exit(1);}printf("PASS %s\n",what);}
static void Pump(unsigned milliseconds=30){ULONGLONG end=GetTickCount64()+milliseconds;do{MSG msg{};while(PeekMessageW(&msg,nullptr,0,0,PM_REMOVE)){TranslateMessage(&msg);DispatchMessageW(&msg);}MsgWaitForMultipleObjects(0,nullptr,FALSE,10,QS_ALLINPUT);}while(GetTickCount64()<end);}
static unsigned geometryNotices=0;
static LRESULT CALLBACK OwnerProc(HWND h,UINT message,WPARAM w,LPARAM l){
 if(message==inputMessage && (uint64_t(l)&0xffff)==1){++geometryNotices;return 0;}
 return DefWindowProcW(h,message,w,l);
}
static LRESULT CALLBACK ParentProc(HWND h,UINT message,WPARAM w,LPARAM l){
 if(message==WM_ERASEBKGND){RECT rect{};GetClientRect(h,&rect);HBRUSH brush=CreateSolidBrush(RGB(36,48,62));FillRect(reinterpret_cast<HDC>(w),&rect,brush);DeleteObject(brush);return 1;}
 return DefWindowProcW(h,message,w,l);
}
static void CheckTransparentPaint(HWND host,HWND owner,unsigned dpi){
 PacketHeader header{PacketMagic,PacketVisible,9,148,48,dpi,ProtocolVersion};
 std::vector<unsigned char> packet(sizeof(header)+148*48*4,0);memcpy(packet.data(),&header,sizeof(header));
 auto pixel=packet.data()+sizeof(header)+(5*148+5)*4;pixel[1]=128;pixel[3]=128;
 pixel=packet.data()+sizeof(header)+(10*148+10)*4;pixel[0]=20;pixel[1]=160;pixel[2]=240;pixel[3]=255;
 COPYDATASTRUCT data{PacketMagic,static_cast<DWORD>(packet.size()),packet.data()};
 Check(SendMessageW(host,WM_COPYDATA,reinterpret_cast<WPARAM>(owner),reinterpret_cast<LPARAM>(&data))!=0,"accept premultiplied transparent pixels");
 BITMAPINFO info{};info.bmiHeader.biSize=sizeof(BITMAPINFOHEADER);info.bmiHeader.biWidth=148;info.bmiHeader.biHeight=-48;info.bmiHeader.biPlanes=1;info.bmiHeader.biBitCount=32;
 HDC dc=CreateCompatibleDC(nullptr);void* bits=nullptr;HBITMAP bitmap=CreateDIBSection(dc,&info,DIB_RGB_COLORS,&bits,nullptr,0);HGDIOBJ old=SelectObject(dc,bitmap);
 SendMessageW(host,WM_PRINTCLIENT,reinterpret_cast<WPARAM>(dc),PRF_CLIENT);
 Check(GetPixel(dc,0,0)==RGB(36,48,62),"transparent margin uses parent material");
 COLORREF mixed=GetPixel(dc,5,5);
 Check(abs(int(GetRValue(mixed))-18)<=2 && abs(int(GetGValue(mixed))-152)<=2 && abs(int(GetBValue(mixed))-31)<=2,"antialias alpha blends over parent material");
 Check(GetPixel(dc,10,10)==RGB(240,160,20),"opaque mark colors remain unchanged");
 SelectObject(dc,old);DeleteObject(bitmap);DeleteDC(dc);
}
class TestSite final:public IOleWindow,public IOleCommandTarget {
 LONG refs=1;
public:
 HWND parent=nullptr;IDeskBand2* band=nullptr;bool retainAllocation=false;
 HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id,void** out)override{if(!out)return E_POINTER;*out=nullptr;if(id==IID_IUnknown||id==IID_IOleWindow)*out=static_cast<IOleWindow*>(this);if(id==IID_IOleCommandTarget)*out=static_cast<IOleCommandTarget*>(this);if(!*out)return E_NOINTERFACE;AddRef();return S_OK;}
 ULONG STDMETHODCALLTYPE AddRef()override{return InterlockedIncrement(&refs);}
 ULONG STDMETHODCALLTYPE Release()override{return InterlockedDecrement(&refs);}
 HRESULT STDMETHODCALLTYPE GetWindow(HWND* out)override{*out=parent;return S_OK;}
 HRESULT STDMETHODCALLTYPE ContextSensitiveHelp(BOOL)override{return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE QueryStatus(const GUID*,ULONG,OLECMD[],OLECMDTEXT*)override{return E_NOTIMPL;}
 HRESULT STDMETHODCALLTYPE Exec(const GUID* group,DWORD command,DWORD,VARIANT*,VARIANT*)override{
  if(!group||*group!=CGID_DeskBand||command!=DBID_BANDINFOCHANGED)return E_NOTIMPL;
  if(retainAllocation)return S_OK;
  DESKBANDINFO info{};info.dwMask=DBIM_MINSIZE|DBIM_ACTUAL;band->GetBandInfo(1,0,&info);
  HWND child=nullptr;band->GetWindow(&child);SetWindowPos(child,nullptr,0,0,info.ptActual.x,48,SWP_NOACTIVATE|SWP_NOZORDER);return S_OK;
 }
};
static bool Send(HWND host,HWND owner,uint64_t generation,uint32_t flags,unsigned width=148,unsigned height=48,unsigned dpi=120){
 PacketHeader header{PacketMagic,flags,generation,width,height,dpi,ProtocolVersion};
 if(flags==PacketDisconnect){header.width=header.height=0;}
 std::vector<unsigned char> packet(sizeof(header)+size_t(header.width)*header.height*4,0xff);memcpy(packet.data(),&header,sizeof(header));
 COPYDATASTRUCT data{PacketMagic,static_cast<DWORD>(packet.size()),packet.data()};return SendMessageW(host,WM_COPYDATA,reinterpret_cast<WPARAM>(owner),reinterpret_cast<LPARAM>(&data))!=0;
}
int main(){
 SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);module=GetModuleHandleW(nullptr);CoInitializeEx(nullptr,COINIT_APARTMENTTHREADED);
 WNDCLASSW wc{};wc.lpfnWndProc=OwnerProc;wc.hInstance=module;wc.lpszClassName=L"AgentQuota";RegisterClassW(&wc);
 HWND owner=CreateWindowW(L"AgentQuota",L"test",WS_POPUP,0,0,10,10,nullptr,nullptr,module,nullptr);
 wc.lpfnWndProc=ParentProc;wc.lpszClassName=L"AgentQuotaTaskbarPaintTest";RegisterClassW(&wc);
 TestSite site;site.parent=CreateWindowW(wc.lpszClassName,L"test parent",WS_POPUP,0,0,600,48,nullptr,nullptr,module,nullptr);
 IClassFactory* factory=nullptr;Check(SUCCEEDED(DllGetClassObject(HostCLSID,IID_PPV_ARGS(&factory))),"factory");
 IDeskBand2* band=nullptr;Check(SUCCEEDED(factory->CreateInstance(nullptr,IID_PPV_ARGS(&band))),"band instance");factory->Release();site.band=band;
 IObjectWithSite* withSite=nullptr;band->QueryInterface(IID_PPV_ARGS(&withSite));Check(SUCCEEDED(withSite->SetSite(static_cast<IOleWindow*>(&site))),"site attach");
 HWND host=nullptr;band->GetWindow(&host);UINT dpi=GetDpiForWindow(host);
 Check(Send(host,owner,9,PacketConnect|PacketVisible,148,48,dpi),"accept bounded frame");Pump();
 Check(GetPropW(host,L"AQ.Fits")!=nullptr,"fit after deferred layout");
 CheckTransparentPaint(host,owner,dpi);
 Pump();unsigned before=geometryNotices;site.retainAllocation=true;
 Check(Send(host,owner,9,PacketVisible,140,48,dpi),"accept resized frame in retained shell allocation");
 bool invalidated=GetPropW(host,L"AQ.Fits")==nullptr;Pump();
 printf("TRACE resized frame: invalidated=%u restored=%u geometry_notifications=%u\n",unsigned(invalidated),unsigned(GetPropW(host,L"AQ.Fits")!=nullptr),geometryNotices-before);
 Check(geometryNotices>before,"resized frame acknowledges restored fit when shell rectangle is unchanged");
 site.retainAllocation=false;
 Check(!Send(host,owner,8,PacketVisible,148,48,dpi),"reject stale generation");
 Check(!Send(host,owner,9,8,148,48,dpi),"reject unknown flags");
 Check(Send(host,owner,9,0,148,48,dpi),"collapse without hide");Pump();RECT rect{};GetWindowRect(host,&rect);Check(rect.right==rect.left,"collapse releases width");
 Check(Send(host,owner,9,PacketVisible,148,48,dpi),"expand same session");Pump();Check(GetPropW(host,L"AQ.Fits")!=nullptr,"expanded frame fits");
 Check(Send(host,owner,9,PacketDisconnect),"disconnect");Pump();Check(GetPropW(host,L"AQ.Session")==nullptr,"disconnect clears owner");
 Check(Send(host,owner,10,PacketConnect|PacketVisible,148,48,dpi),"reconnect generation");Pump();DestroyWindow(owner);Pump(200);
 Check(GetPropW(host,L"AQ.Session")==nullptr,"owner destruction clears connection");GetWindowRect(host,&rect);Check(rect.right==rect.left,"owner destruction collapses width");
 Check(DllCanUnloadNow()==S_FALSE,"hooks keep module loaded");
 withSite->SetSite(nullptr);
 Check(SUCCEEDED(withSite->SetSite(static_cast<IOleWindow*>(&site))),"reuse same band object after teardown");
 owner=CreateWindowW(L"AgentQuota",L"replacement",WS_POPUP,0,0,10,10,nullptr,nullptr,module,nullptr);
 band->GetWindow(&host);dpi=GetDpiForWindow(host);
 Check(Send(host,owner,11,PacketConnect|PacketVisible,148,48,dpi),"replacement attachment");Pump();Check(GetPropW(host,L"AQ.Fits")!=nullptr,"replacement deferred layout works");
 DestroyWindow(owner);Pump(100);withSite->SetSite(nullptr);withSite->Release();band->Release();DestroyWindow(site.parent);Pump();
 Check(DllCanUnloadNow()==S_OK,"all hooks and objects released");CoUninitialize();return 0;
}
