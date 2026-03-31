//go:build windows

package tray

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/yashau/ganoid/internal/client"
	"github.com/yashau/ganoid/internal/event"
)

// ─── Win32 constants ─────────────────────────────────────────────────────────

const (
	wmUser         = 0x0400
	wmTrayCallback = wmUser + 1 // sent by Shell_NotifyIcon back to our window

	trayUID = 1

	// Menu item IDs (must be non-zero; disabled items use 0 or are never returned).
	idOpenDashboard = 100
	idQuit          = 101
	idProfileBase   = 200 // profile items: idProfileBase + slice index

	// Shell_NotifyIcon operations.
	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	// NOTIFYICONDATA uFlags.
	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	// LoadImage constants.
	imageIcon      = 1
	lrLoadFromFile = 0x10
	lrDefaultSize  = 0x40

	// TrackPopupMenu flags.
	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020
	tpmReturnCmd   = 0x0100
	tpmNoNotify    = 0x0080

	// AppendMenu flags.
	mfString    = 0x00000000
	mfGrayed    = 0x00000001
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008

	// Tray icon mouse messages (low word of lParam in wmTrayCallback).
	wmRButtonUp    = 0x0205
	wmLButtonDblCk = 0x0203

	// HWND_MESSAGE — message-only window parent.
	hwndMessage = ^uintptr(2) // (HWND)(-3)
)

// ─── Win32 structs ────────────────────────────────────────────────────────────

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// notifyIconData matches NOTIFYICONDATAW (Vista+ layout).
type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type pointL struct{ X, Y int32 }

type winMsg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      pointL
}

// ─── Lazy-loaded Win32 procs ──────────────────────────────────────────────────

var (
	modUser32  = windows.NewLazySystemDLL("user32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = modUser32.NewProc("DefWindowProcW")
	procGetMessageW         = modUser32.NewProc("GetMessageW")
	procTranslateMessage    = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = modUser32.NewProc("DispatchMessageW")
	procPostMessageW        = modUser32.NewProc("PostMessageW")
	procPostQuitMessage     = modUser32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu      = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu         = modUser32.NewProc("DestroyMenu")
	procGetCursorPos        = modUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procLoadImageW          = modUser32.NewProc("LoadImageW")
	procDestroyIcon         = modUser32.NewProc("DestroyIcon")

	procShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW = modKernel32.NewProc("GetModuleHandleW")
)

// ─── Package-level tray state (one instance per process) ─────────────────────

var (
	gHolder    *client.Holder
	gHWnd      windows.HWND
	gIconH     uintptr    // HICON
	gTempIcon  string     // temp .ico file to delete on exit
	gProfileIDs []string  // indexed by (menuID - idProfileBase), set fresh each time menu is shown
	gWndProcCB uintptr    // syscall.NewCallback result — created once at init
)

func init() {
	gWndProcCB = syscall.NewCallback(trayWndProc)
}

// ─── Window procedure ────────────────────────────────────────────────────────

func trayWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch uint32(message) {
	case wmTrayCallback:
		notif := uint32(lParam) & 0xffff
		switch notif {
		case wmRButtonUp:
			showContextMenu(windows.HWND(hwnd))
		case wmLButtonDblCk:
			c := gHolder.Get()
			if c != nil {
				OpenBrowser(c.DashboardURL())
			}
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// ─── Run (blocks until Quit) ──────────────────────────────────────────────────

// Run creates the system tray icon and runs the Win32 message loop.
// Blocks until the user clicks Quit. rebuildCh is drained but not acted on
// since the context menu is built live from the API on every right-click.
func Run(h *client.Holder, rebuildCh <-chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	gHolder = h

	// Load icon.
	iconH, iconTemp, err := loadTrayIcon()
	if err == nil {
		gIconH = iconH
		gTempIcon = iconTemp
		defer func() {
			procDestroyIcon.Call(gIconH)
			os.Remove(gTempIcon)
		}()
	}

	// Register window class.
	className, _ := windows.UTF16PtrFromString("GanoidTrayWnd")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	wcx := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   gWndProcCB,
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))

	// Create message-only window.
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,           // no title
		0,           // no style
		0, 0, 0, 0,  // position & size irrelevant
		hwndMessage, // message-only, no taskbar entry
		0, hInst, 0,
	)
	gHWnd = windows.HWND(hwnd)

	// Register tray icon.
	nid := makeNID(nifMessage | nifIcon | nifTip)
	nid.UCallbackMessage = wmTrayCallback
	nid.HIcon = gIconH
	copy(nid.SzTip[:], windows.StringToUTF16("Ganoid — Tailscale profile manager"))
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))

	defer func() {
		nd := makeNID(0)
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nd)))
	}()

	// Status poller — keeps the tooltip current.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go statusPoller(ctx, gHWnd, h)

	// Drain rebuildCh; menu is always built live.
	go func() {
		for {
			select {
			case <-rebuildCh:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Win32 message loop.
	var m winMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) { // WM_QUIT or error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// ─── Context menu ─────────────────────────────────────────────────────────────

func showContextMenu(hwnd windows.HWND) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	// Query live data.
	c := gHolder.Get()
	statusLabel := "Status: ganoidd not running"
	var activeID string
	gProfileIDs = nil

	if c != nil {
		reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		status, err := c.Status(reqCtx)
		cancel()
		if err == nil && status != nil {
			state := status.Tailscale.BackendState
			switch state {
			case "Not installed":
				statusLabel = "Status: Tailscale not installed"
			default:
				statusLabel = fmt.Sprintf("Status: %s (%s)", state, status.ActiveProfile.Name)
			}
		} else {
			statusLabel = "Status: ganoidd unreachable"
		}

		pCtx, pCancel := context.WithTimeout(context.Background(), 3*time.Second)
		store, err := c.Profiles(pCtx)
		pCancel()
		if err == nil && store != nil {
			activeID = store.ActiveProfileID
			for _, p := range store.Profiles {
				gProfileIDs = append(gProfileIDs, p.ID)
				label := "  " + p.Name
				flags := uint32(mfString)
				if p.ID == activeID {
					label = "\u2713 " + p.Name
					flags |= mfGrayed | mfChecked
				}
				menuID := uint32(idProfileBase + len(gProfileIDs) - 1)
				appendMenu(hMenu, flags, menuID, label)
			}
		}
	} else {
		appendMenu(hMenu, mfString|mfGrayed, 0, statusLabel)
		statusLabel = "" // already added
	}

	if statusLabel != "" {
		appendMenu(hMenu, mfString|mfGrayed, 0, statusLabel)
	}

	if len(gProfileIDs) > 0 {
		appendMenu(hMenu, mfSeparator, 0, "")
	}

	appendMenu(hMenu, mfString, idOpenDashboard, "Open Dashboard")
	appendMenu(hMenu, mfSeparator, 0, "")
	appendMenu(hMenu, mfString, idQuit, "Quit")

	// Show at cursor position; SetForegroundWindow prevents "stuck" menus.
	var pt pointL
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))

	cmd, _, _ := procTrackPopupMenu.Call(
		hMenu,
		tpmLeftAlign|tpmBottomAlign|tpmReturnCmd|tpmNoNotify,
		uintptr(pt.X), uintptr(pt.Y),
		0, uintptr(hwnd), 0,
	)

	handleMenuCmd(uint32(cmd), hwnd)
}

func handleMenuCmd(cmd uint32, hwnd windows.HWND) {
	switch cmd {
	case 0:
		// Cancelled — nothing to do.
	case idOpenDashboard:
		c := gHolder.Get()
		if c != nil {
			OpenBrowser(c.DashboardURL())
		}
	case idQuit:
		procPostQuitMessage.Call(0)
	default:
		idx := int(cmd) - idProfileBase
		if idx >= 0 && idx < len(gProfileIDs) {
			profileID := gProfileIDs[idx]
			c := gHolder.Get()
			if c == nil {
				return
			}
			go func() {
				switchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				done := make(chan struct{})
				c.SwitchProfile(switchCtx, profileID,
					func(ev event.SwitchEvent) {},
					func() { close(done) },
					func(err error) { close(done) },
				)
				<-done
			}()
		}
	}
}

// ─── Status poller ────────────────────────────────────────────────────────────

func statusPoller(ctx context.Context, hwnd windows.HWND, h *client.Holder) {
	update := func() {
		c := h.Get()
		var tip string
		if c == nil {
			tip = "Ganoid — ganoidd not running"
		} else {
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			status, err := c.Status(reqCtx)
			cancel()
			if err == nil && status != nil {
				state := status.Tailscale.BackendState
				switch state {
				case "Not installed":
					tip = "Ganoid — Tailscale not installed"
				default:
					tip = fmt.Sprintf("Ganoid — %s (%s)", state, status.ActiveProfile.Name)
				}
			} else {
				tip = "Ganoid — ganoidd unreachable"
			}
		}
		nid := makeNID(nifTip)
		copy(nid.SzTip[:], windows.StringToUTF16(limitRunes(tip, 127)))
		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	}

	update()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			update()
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func makeNID(flags uint32) notifyIconData {
	nid := notifyIconData{
		HWnd:   gHWnd,
		UID:    trayUID,
		UFlags: flags,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	return nid
}

func appendMenu(hMenu uintptr, flags, id uint32, text string) {
	if flags&mfSeparator != 0 {
		procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
		return
	}
	ptr, _ := windows.UTF16PtrFromString(text)
	procAppendMenuW.Call(hMenu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(ptr)))
}

func limitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func loadTrayIcon() (hicon uintptr, tempPath string, err error) {
	f, err := os.CreateTemp("", "ganoid-*.ico")
	if err != nil {
		return 0, "", fmt.Errorf("create temp icon: %w", err)
	}
	tempPath = f.Name()
	if _, err = f.Write(Icon()); err != nil {
		f.Close()
		os.Remove(tempPath)
		return 0, "", fmt.Errorf("write temp icon: %w", err)
	}
	f.Close()

	pathPtr, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		os.Remove(tempPath)
		return 0, "", err
	}

	h, _, lerr := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		imageIcon,
		0, 0, // let Windows pick the size
		lrLoadFromFile|lrDefaultSize,
	)
	if h == 0 {
		os.Remove(tempPath)
		return 0, "", fmt.Errorf("LoadImage: %w", lerr)
	}
	return h, tempPath, nil
}
