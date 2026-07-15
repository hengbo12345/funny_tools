//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	stdInputHandle  = ^uintptr(10) + 1
	enableEchoInput = 0x0004
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getStdHandle   = kernel32.NewProc("GetStdHandle")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

func readSecretLine(label string) (string, error) {
	fmt.Print(label + ": ")
	handle, _, _ := getStdHandle.Call(stdInputHandle)
	if handle == 0 || handle == ^uintptr(0) {
		return readVisibleLine()
	}
	var mode uint32
	if r1, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode))); r1 != 0 {
		_, _, _ = setConsoleMode.Call(handle, uintptr(mode&^enableEchoInput))
		defer setConsoleMode.Call(handle, uintptr(mode))
		defer fmt.Println()
	}
	return readVisibleLine()
}

func readVisibleLine() (string, error) {
	value, err := readInputLine()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(value, "\r\n"), nil
}
