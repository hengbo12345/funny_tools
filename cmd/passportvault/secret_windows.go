//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"syscall"
	"unsafe"
)

const (
	stdInputHandle                  = ^uintptr(10) + 1
	stdOutputHandle                 = ^uintptr(11) + 1
	enableEchoInput                 = 0x0004
	enableLineInput                 = 0x0002
	enableVirtualTerminalProcessing = 0x0004
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getStdHandle   = kernel32.NewProc("GetStdHandle")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

func init() {
	handle, _, _ := getStdHandle.Call(stdOutputHandle)
	if handle != 0 && handle != ^uintptr(0) {
		var mode uint32
		if r1, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode))); r1 != 0 {
			_, _, _ = setConsoleMode.Call(handle, uintptr(mode|enableVirtualTerminalProcessing))
		}
	}
}

func setTerminalRaw(in io.Reader) (func(), error) {
	file, ok := in.(*os.File)
	if !ok {
		return nil, errors.New("not an os.File")
	}
	handle := syscall.Handle(file.Fd())
	var mode uint32
	r1, _, _ := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 == 0 {
		stdH, _, _ := getStdHandle.Call(stdInputHandle)
		if stdH == 0 || stdH == ^uintptr(0) {
			return nil, errors.New("invalid console handle")
		}
		handle = syscall.Handle(stdH)
		r1, _, _ = getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
		if r1 == 0 {
			return nil, errors.New("could not get console mode")
		}
	}

	rawMode := mode &^ (enableEchoInput | enableLineInput)
	r2, _, _ := setConsoleMode.Call(uintptr(handle), uintptr(rawMode))
	if r2 == 0 {
		return nil, errors.New("could not set console mode")
	}

	restore := func() {
		_, _, _ = setConsoleMode.Call(uintptr(handle), uintptr(mode))
	}
	return restore, nil
}
