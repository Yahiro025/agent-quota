package win

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const taskbarCLSID = `{E2219E46-1471-47B5-BB31-654F89245B94}`
const taskbarRegistry = `Software\Classes\CLSID\` + taskbarCLSID
const taskbarProtocol = 2
const packetMagic = 0x31425141

var (
	pEnumChildWindows      = user32.NewProc("EnumChildWindows")
	pGetClassName          = user32.NewProc("GetClassNameW")
	pGetAncestor           = user32.NewProc("GetAncestor")
	pGetProp               = user32.NewProc("GetPropW")
	pIsWindow              = user32.NewProc("IsWindow")
	pGetWindowPID          = user32.NewProc("GetWindowThreadProcessId")
	pSendMessageTimeout    = user32.NewProc("SendMessageTimeoutW")
	pPeekTaskbarMessage    = user32.NewProc("PeekMessageW")
	pRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	taskbarInput           = registerMessage("AgentQuota.Taskbar.Input.1")
	taskbarReady           = registerMessage("AgentQuota.Taskbar.Ready.1")
	taskbarCreated         = registerMessage("TaskbarCreated")
)

func registerMessage(name string) uint32 {
	p, _ := windows.UTF16PtrFromString(name)
	r, _, _ := pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(p)))
	return uint32(r)
}

type TaskbarSnapshot struct {
	Available, Fits, Hidden bool
	Bounds, Tray, Readout   image.Rectangle
	DPI                     int
	Build                   uint32
}

func (s TaskbarSnapshot) DockTarget(p image.Point, width int) bool {
	return s.Available && !s.Hidden && p.In(image.Rect(max(s.Bounds.Min.X, s.Tray.Min.X-width-24), s.Bounds.Min.Y, s.Tray.Min.X, s.Bounds.Max.Y))
}

// TaskbarBridge transports only opaque pixels and input. A generation rejects
// queued events from an earlier attachment; it is not an authentication token.
type TaskbarBridge struct {
	window     uintptr
	hostPID    uint32
	generation uint64
	expected   uint32
	last       []byte
}

func NewTaskbarBridge() *TaskbarBridge {
	var seed [8]byte
	_, _ = rand.Read(seed[:])
	generation := binary.LittleEndian.Uint64(seed[:])
	if generation == 0 {
		generation = 1
	}
	b := &TaskbarBridge{generation: generation}
	if key, err := registry.OpenKey(registry.CURRENT_USER, taskbarRegistry, registry.QUERY_VALUE); err == nil {
		value, _, _ := key.GetIntegerValue("AgentQuotaBuild")
		b.expected = uint32(value)
		key.Close()
	}
	return b
}

func className(h uintptr) string {
	var buf [128]uint16
	pGetClassName.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:])
}
func prop(h uintptr, name string) uintptr {
	p, _ := windows.UTF16PtrFromString(name)
	r, _, _ := pGetProp.Call(h, uintptr(unsafe.Pointer(p)))
	return r
}
func windowPID(h uintptr) uint32 {
	var pid uint32
	pGetWindowPID.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}
func rectOf(h uintptr) image.Rectangle {
	var r [4]int32
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r[0])))
	return image.Rect(int(r[0]), int(r[1]), int(r[2]), int(r[3]))
}
func primaryTaskbar() uintptr {
	cls, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(cls)), 0)
	return h
}

type childSearch struct {
	name  string
	found uintptr
}

var currentChildSearch *childSearch
var childCallback = windows.NewCallback(func(h, l uintptr) uintptr {
	search := currentChildSearch
	if search == nil {
		return 0
	}
	if className(h) == search.name {
		search.found = h
		return 0
	}
	return 1
})

func childClass(root uintptr, name string) uintptr {
	if root == 0 {
		return 0
	}
	search := childSearch{name: name}
	previous := currentChildSearch
	currentChildSearch = &search
	pEnumChildWindows.Call(root, childCallback, 0)
	currentChildSearch = previous
	return search.found
}

func (b *TaskbarBridge) Snapshot() TaskbarSnapshot {
	root := primaryTaskbar()
	s := TaskbarSnapshot{Bounds: rectOf(root), DPI: 96}
	tray := childClass(root, "TrayNotifyWnd")
	s.Tray = rectOf(tray)
	// Only the tested bottom Windows 10 taskbar is supported.
	_, screenH := ScreenSize()
	if root == 0 || tray == 0 || childClass(root, "ReBarWindow32") == 0 || s.Bounds.Dx() < s.Bounds.Dy() || s.Bounds.Min.Y < screenH/2 {
		b.forget()
		return s
	}
	valid, _, _ := pIsWindow.Call(b.window)
	if b.window == 0 || valid == 0 || className(b.window) != "AgentQuotaTaskbarHost3" || windowPID(b.window) != b.hostPID {
		b.window = childClass(root, "AgentQuotaTaskbarHost3")
		b.hostPID = windowPID(b.window)
		b.generation++
		if b.generation == 0 {
			b.generation = 1
		}
		b.last = nil
	}
	h := b.window
	ancestor, _, _ := pGetAncestor.Call(h, 2)
	build := uint32(prop(h, "AQ.Build"))
	if h == 0 || ancestor != root || b.hostPID != windowPID(root) || b.expected == 0 || build != b.expected || prop(h, "AQ.Protocol") != taskbarProtocol {
		b.forget()
		return s
	}
	s.Available = true
	s.Build = build
	s.Readout = rectOf(h)
	s.DPI = int(prop(h, "AQ.DPI"))
	if s.DPI < 96 {
		dpi, _, _ := pGetDPIForWindow.Call(h)
		s.DPI = int(dpi)
	}
	s.Fits = prop(h, "AQ.Fits") != 0 && prop(h, "AQ.Session") == uintptr(b.generation) && s.Readout.In(s.Bounds) && s.Readout.Max.X <= s.Tray.Min.X
	s.Hidden = s.Bounds.Min.Y >= screenH-2 || s.Readout.Min.Y >= screenH-2
	return s
}

func taskbarPacket(generation uint64, flags uint32, w, h, dpi int, pixels []byte) ([]byte, error) {
	if generation == 0 || flags&^uint32(7) != 0 || dpi < 96 || dpi > 768 {
		return nil, fmt.Errorf("invalid taskbar header")
	}
	if flags == 2 {
		if w != 0 || h != 0 || len(pixels) != 0 {
			return nil, fmt.Errorf("invalid disconnect")
		}
	} else if flags&2 != 0 || w < 1 || h < 1 || w > 1024 || h > 256 || len(pixels) != w*h*4 {
		return nil, fmt.Errorf("invalid taskbar frame")
	}
	p := make([]byte, 32+len(pixels))
	binary.LittleEndian.PutUint32(p, packetMagic)
	binary.LittleEndian.PutUint32(p[4:], flags)
	binary.LittleEndian.PutUint64(p[8:], generation)
	for i, v := range []uint32{uint32(w), uint32(h), uint32(dpi), taskbarProtocol} {
		binary.LittleEndian.PutUint32(p[16+i*4:], v)
	}
	copy(p[32:], pixels)
	return p, nil
}

func (b *TaskbarBridge) send(packet []byte) bool {
	if b.window == 0 || hwnd == 0 {
		return false
	}
	data := struct {
		tag  uintptr
		size uint32
		data unsafe.Pointer
	}{packetMagic, uint32(len(packet)), unsafe.Pointer(&packet[0])}
	var accepted uintptr
	delivered, _, _ := pSendMessageTimeout.Call(b.window, 0x004a, uintptr(hwnd), uintptr(unsafe.Pointer(&data)), 0x0001|0x0002|0x0020, 150, uintptr(unsafe.Pointer(&accepted)))
	runtime.KeepAlive(packet)
	runtime.KeepAlive(data)
	return delivered != 0 && accepted != 0
}

func (b *TaskbarBridge) Frame(pixels []byte, w, h, dpi int, visible bool) bool {
	if !b.Snapshot().Available {
		return false
	}
	flags := uint32(0)
	if visible {
		flags = 1
	}
	if prop(b.window, "AQ.Session") != uintptr(b.generation) {
		flags |= 4
	}
	packet, err := taskbarPacket(b.generation, flags, w, h, dpi, pixels)
	if err != nil {
		return false
	}
	if bytes.Equal(packet, b.last) {
		return true
	}
	if !b.send(packet) {
		b.window = 0
		b.last = nil
		return false
	}
	b.last = packet
	return true
}

func (b *TaskbarBridge) Disconnect() {
	packet, _ := taskbarPacket(b.generation, 2, 0, 0, 96, nil)
	b.send(packet)
	b.last = nil
}

func decodeTaskbarInput(packed uintptr) (int, image.Point) {
	x := int32((uint64(packed) >> 16) & 0xffffff)
	y := int32((uint64(packed) >> 40) & 0xffffff)
	x = x << 8 >> 8
	y = y << 8 >> 8
	return int(packed & 0xffff), image.Pt(int(x), int(y))
}

func handleTaskbarMessage(msg uint32, w, l uintptr) bool {
	listener, ok := cb.(interface {
		OnTaskbarEvent(uint64, int, image.Point)
	})
	if !ok {
		return false
	}
	if msg == taskbarInput {
		kind, p := decodeTaskbarInput(l)
		if kind == 5 {
			// Forwarded PostMessage input does not get WM_MOUSEMOVE's automatic
			// coalescing. Repainting old positions makes a captured drag trail the
			// pointer; keep only the latest queued move before any control event.
			var next struct {
				window   uintptr
				message  uint32
				w, l     uintptr
				time     uint32
				position point
				private  uint32
			}
			for {
				queued, _, _ := pPeekTaskbarMessage.Call(uintptr(unsafe.Pointer(&next)), uintptr(hwnd), uintptr(taskbarInput), uintptr(taskbarInput), 0)
				if queued == 0 {
					break
				}
				nextKind, nextPoint := decodeTaskbarInput(next.l)
				if nextKind != 5 || next.w != w {
					break
				}
				pPeekTaskbarMessage.Call(uintptr(unsafe.Pointer(&next)), uintptr(hwnd), uintptr(taskbarInput), uintptr(taskbarInput), 1)
				p = nextPoint
			}
		}
		listener.OnTaskbarEvent(uint64(w), kind, p)
		return true
	}
	if msg == taskbarReady || msg == taskbarCreated {
		listener.OnTaskbarEvent(0, 0, image.Point{})
		return true
	}
	return false
}
func (b *TaskbarBridge) Owns(generation uint64) bool {
	return b.window != 0 && generation == b.generation
}
func SetVisible(visible bool) {
	show := uintptr(0)
	if visible {
		show = swShowNoactivate
	}
	pShowWindow.Call(uintptr(hwnd), show)
	windowVisible = visible
}
func CursorPosition() image.Point { x, y := cursorPos(); return image.Pt(x, y) }
func ShowMenu()                   { showMenu() }

type TaskbarManifest struct {
	Build  uint32 `json:"build"`
	DLL    string `json:"dll"`
	SHA256 string `json:"sha256"`
}

// InstallTaskbar registers one versioned per-user module. Explorer's own
// confirmation remains the authority for enabling its toolbar.
func InstallTaskbar(manifestPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var m TaskbarManifest
	if len(data) > 4096 {
		return fmt.Errorf("taskbar manifest too large")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err = json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Build == 0 || filepath.Base(m.DLL) != m.DLL || !strings.HasPrefix(m.DLL, "agentquota-taskbar-") || filepath.Ext(m.DLL) != ".dll" {
		return fmt.Errorf("invalid taskbar manifest")
	}
	path, err := filepath.Abs(filepath.Join(filepath.Dir(manifestPath), m.DLL))
	if err != nil {
		return err
	}
	dll, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(dll)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), m.SHA256) {
		return fmt.Errorf("taskbar DLL hash mismatch")
	}
	if childClass(primaryTaskbar(), "ReBarWindow32") == 0 {
		return fmt.Errorf("native docking requires the Windows 10 horizontal taskbar")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, taskbarRegistry, registry.WRITE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err = key.SetStringValue("", "Agent Quota"); err != nil {
		return err
	}
	if err = key.SetDWordValue("AgentQuotaBuild", m.Build); err != nil {
		return err
	}
	inproc, _, err := registry.CreateKey(key, `InprocServer32`, registry.WRITE)
	if err != nil {
		return err
	}
	err = inproc.SetStringValue("", path)
	if err == nil {
		err = inproc.SetStringValue("ThreadingModel", "Apartment")
	}
	inproc.Close()
	if err != nil {
		return err
	}
	category, _, err := registry.CreateKey(key, `Implemented Categories\{00021492-0000-0000-C000-000000000046}`, registry.WRITE)
	if err != nil {
		return err
	}
	category.Close()
	if err := retirePreviousTaskbarHosts(); err != nil {
		return err
	}
	return taskbarShell(true)
}

func taskbarShell(show bool) error { return taskbarShellClass(show, taskbarCLSID) }
func taskbarShellClass(show bool, class string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return fmt.Errorf("taskbar COM initialization: 0x%x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	cls, _ := windows.GUIDFromString(`{E6442437-6C68-4F52-94DD-2CFED267EFB9}`)
	iid, _ := windows.GUIDFromString(`{6D67E846-5B9C-4DB8-9CBC-DDE12F4254F1}`)
	band, _ := windows.GUIDFromString(class)
	if show {
		if err := discoverTaskbarClass(ole, band); err != nil {
			return err
		}
	}
	var object *struct{ vtable *[7]uintptr }
	hr, _, _ = ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&cls)), 0, 5, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&object)))
	if int32(hr) < 0 {
		return fmt.Errorf("taskbar shell service: 0x%x", hr)
	}
	vt := object.vtable
	objectAddress := uintptr(unsafe.Pointer(object))
	defer syscall.SyscallN(vt[2], objectAddress)
	windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0x2000, 0, 0)
	syscall.SyscallN(vt[6], objectAddress)
	method := 3
	if !show {
		method = 4
	}
	hr, _, _ = syscall.SyscallN(vt[method], objectAddress, uintptr(unsafe.Pointer(&band)))
	runtime.KeepAlive(band)
	if int32(hr) < 0 {
		return fmt.Errorf("taskbar toolbar registration: 0x%x", hr)
	}
	return nil
}

type shellObject struct{ vtable *[9]uintptr }

func (o *shellObject) release() { syscall.SyscallN(o.vtable[2], uintptr(unsafe.Pointer(o))) }
func createShellObject(ole *windows.LazyDLL, cls, iid windows.GUID) (*shellObject, error) {
	var object *shellObject
	hr, _, _ := ole.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&cls)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&object)))
	if int32(hr) < 0 {
		return nil, fmt.Errorf("taskbar component discovery: 0x%x", hr)
	}
	return object, nil
}

// Resolve the registered category through COM before asking the shell to show
// it. A fresh HKCU category can otherwise remain absent from Explorer's cache.
func discoverTaskbarClass(ole *windows.LazyDLL, band windows.GUID) error {
	unknown, _ := windows.GUIDFromString(`{00000000-0000-0000-C000-000000000046}`)
	host, err := createShellObject(ole, band, unknown)
	if err != nil {
		return err
	}
	host.release()
	manager, _ := windows.GUIDFromString(`{0002E005-0000-0000-C000-000000000046}`)
	information, _ := windows.GUIDFromString(`{0002E013-0000-0000-C000-000000000046}`)
	categories, err := createShellObject(ole, manager, information)
	if err != nil {
		return err
	}
	defer categories.release()
	category, _ := windows.GUIDFromString(`{00021492-0000-0000-C000-000000000046}`)
	var enumeration *shellObject
	hr, _, _ := syscall.SyscallN(categories.vtable[5], uintptr(unsafe.Pointer(categories)), 1, uintptr(unsafe.Pointer(&category)), 0, 0, uintptr(unsafe.Pointer(&enumeration)))
	if int32(hr) < 0 || enumeration == nil {
		return fmt.Errorf("taskbar category enumeration: 0x%x", hr)
	}
	defer enumeration.release()
	for i := 0; i < 256; i++ {
		var cls windows.GUID
		var fetched uint32
		hr, _, _ = syscall.SyscallN(enumeration.vtable[3], uintptr(unsafe.Pointer(enumeration)), 1, uintptr(unsafe.Pointer(&cls)), uintptr(unsafe.Pointer(&fetched)))
		if hr != 0 || fetched != 1 {
			break
		}
		if cls == band {
			return nil
		}
	}
	return fmt.Errorf("registered taskbar host is absent from deskband category")
}

func (b *TaskbarBridge) forget() {
	if b.window != 0 && prop(b.window, "AQ.Session") == uintptr(b.generation) {
		b.Disconnect()
	}
	b.window = 0
	b.last = nil
	b.generation++
	if b.generation == 0 {
		b.generation = 1
	}
}

// TaskbarStatus is a credential-free diagnostic of native geometry and flags.
func TaskbarStatus() map[string]any {
	b := NewTaskbarBridge()
	s := b.Snapshot()
	root := primaryTaskbar()
	host := childClass(root, "AgentQuotaTaskbarHost3")
	ownerClass, _ := windows.UTF16PtrFromString("AgentQuota")
	owner, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(ownerClass)), 0)
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(owner, ^uintptr(19))
	shown, _, _ := user32.NewProc("IsWindowVisible").Call(owner)
	foreground, _, _ := user32.NewProc("GetForegroundWindow").Call()
	return map[string]any{"snapshot": s, "host": host, "session": prop(host, "AQ.Session"), "native_fits": prop(host, "AQ.Fits") != 0, "parent_background": prop(host, "AQ.ParentBackground") != 0, "native_protocol": prop(host, "AQ.Protocol"), "native_visible": prop(host, "AQ.Visible") != 0, "owner": owner, "owner_bounds": rectOf(owner), "owner_visible": shown != 0, "owner_exstyle": style, "agent_keys_ready": prop(owner, "AQ.AgentKeys") != 0, "foreground": foreground, "task_buttons": rectOf(childClass(root, "MSTaskListWClass"))}
}

// Each replaced host is retired through its shell interface. A fresh identity
// loads the verified DLL without restarting Explorer or reusing its old factory.
func retirePreviousTaskbarHosts() error {
	for _, class := range []string{"{E2219E46-1471-47B5-BB31-654F89245B93}", "{E2219E46-1471-47B5-BB31-654F89245B92}"} {
		if err := retireTaskbarClass(class); err != nil {
			return err
		}
	}
	return nil
}
func retireTaskbarClass(class string) error {
	path := `Software\Classes\CLSID\` + class
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	title, _, err := key.GetStringValue("")
	key.Close()
	if err != nil || title != "Agent Quota" {
		return fmt.Errorf("unexpected previous taskbar registration")
	}
	if err := taskbarShellClass(false, class); err != nil {
		return err
	}
	for _, suffix := range []string{`\Implemented Categories\{00021492-0000-0000-C000-000000000046}`, `\Implemented Categories`, `\InprocServer32`, ""} {
		if err := registry.DeleteKey(registry.CURRENT_USER, path+suffix); err != nil && err != registry.ErrNotExist {
			return err
		}
	}
	return nil
}
