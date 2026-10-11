//go:build !windows

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

func setTerminalRaw(in io.Reader) (func(), error) {
	file, ok := in.(*os.File)
	if !ok {
		return nil, errors.New("not an os.File")
	}
	info, err := file.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		return nil, errors.New("not a terminal device")
	}

	cmd := exec.Command("stty", "-g")
	cmd.Stdin = file
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(out))

	setCmd := exec.Command("stty", "-echo", "-icanon", "min", "1")
	setCmd.Stdin = file
	if err := setCmd.Run(); err != nil {
		return nil, err
	}

	restore := func() {
		restoreCmd := exec.Command("stty", state)
		restoreCmd.Stdin = file
		_ = restoreCmd.Run()
	}
	return restore, nil
}
