//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func readSecretLine(label string) (string, error) {
	fmt.Print(label + ": ")
	_ = exec.Command("stty", "-echo").Run()
	defer exec.Command("stty", "echo").Run()
	defer fmt.Println()

	value, err := readInputLine()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(value, "\r\n"), nil
}
