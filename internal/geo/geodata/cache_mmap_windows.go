//go:build windows

package geodata

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapFile maps the whole file read-only (PAGE_READONLY, FILE_MAP_READ). The
// mapping handle is closed once the view exists; the view keeps the section
// alive until unmapFile.
func mapFile(f *os.File, size int) ([]byte, error) {
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, os.NewSyscallError("CreateFileMapping", err)
	}
	defer windows.CloseHandle(h)
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, os.NewSyscallError("MapViewOfFile", err)
	}
	// addr is the view's base address, outside the Go heap.
	return unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(nil), addr)), size), nil
}

// adviseRandom has no Windows counterpart worth the call: a view faults in
// only the pages queries touch.
func adviseRandom([]byte) {}

func unmapFile(b []byte) {
	_ = windows.UnmapViewOfFile(uintptr(unsafe.Pointer(unsafe.SliceData(b))))
}
