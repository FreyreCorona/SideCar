//go:build windows

package power

import (
	"log"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	WM_POWERBROADCAST = 0x0218
	PBT_APMSUSPEND    = 0x0004
	PBT_APMRESUME     = 0x0007
)

type windowsPowerMonitor struct {
	hwnd syscall.Handle
}

func newPowerMonitor() Monitor {
	return &windowsPowerMonitor{}
}

func (m *windowsPowerMonitor) Start(callback func(PowerEvent)) error {
	go func() {
		runtime.LockOSThread()

		user32 := syscall.NewLazyDLL("user32.dll")
		createWindow := user32.NewProc("CreateWindowExW")
		defWindowProc := user32.NewProc("DefWindowProcW")
		registerClass := user32.NewProc("RegisterClassW")
		getMessage := user32.NewProc("GetMessageW")
		dispatchMessage := user32.NewProc("DispatchMessageW")

		className, _ := syscall.UTF16PtrFromString("SideCarPowerMonitor")

		type wndClass struct {
			style         uint32
			lpfnWndProc   uintptr
			cbClsExtra    int32
			cbWndExtra    int32
			hInstance     syscall.Handle
			hIcon         syscall.Handle
			hCursor       syscall.Handle
			hbrBackground syscall.Handle
			lpszMenuName  *uint16
			lpszClassName *uint16
		}

		wndProc := syscall.NewCallback(func(hwnd syscall.Handle, msg uint32, wparam, lparam uintptr) uintptr {
			if msg == WM_POWERBROADCAST {
				if wparam == PBT_APMSUSPEND {
					log.Println("PowerMonitor: Windows is suspending")
					callback(Sleep)
				} else if wparam == PBT_APMRESUME {
					log.Println("PowerMonitor: Windows is resuming")
					callback(Wake)
				}
			}
			ret, _, _ := defWindowProc.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
			return ret
		})

		wc := wndClass{
			lpfnWndProc:   wndProc,
			lpszClassName: className,
		}

		registerClass.Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, _ := createWindow.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		)

		m.hwnd = syscall.Handle(hwnd)

		var msg struct {
			hwnd    syscall.Handle
			message uint32
			wParam  uintptr
			lParam  uintptr
			time    uint32
			pt      struct{ x, y int32 }
		}

		for {
			ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if ret == 0 {
				break
			}
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()

	return nil
}

func (m *windowsPowerMonitor) Stop() {
}
