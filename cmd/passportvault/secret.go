package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// isAllowedPasswordRune returns true only for standard printable ASCII characters
// between 32 (' ') and 126 ('~').
// Control characters and non-ASCII Unicode characters (such as Chinese, Japanese, Korean,
// full-width symbols, emojis, etc.) are disallowed.
func isAllowedPasswordRune(r rune) bool {
	return r >= 32 && r <= 126
}

// validatePassword returns an error if the password contains any disallowed character.
func validatePassword(password string) error {
	for _, r := range password {
		if !isAllowedPasswordRune(r) {
			return errors.New("password contains disallowed characters: only standard ASCII printable characters are allowed (Chinese, Japanese, and other non-standard characters are not supported)")
		}
	}
	return nil
}

func readSecretLine(label string) (string, error) {
	return readSecret(label, os.Stdin, os.Stdout)
}

func readSecret(label string, in io.Reader, out io.Writer) (string, error) {
	if label != "" {
		fmt.Fprint(out, label+": ")
	}

	restore, err := setTerminalRaw(in)
	if err != nil {
		return readSecretFallback(in, out)
	}

	defer func() {
		restore()
		fmt.Fprintln(out)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	doneChan := make(chan struct{})
	defer close(doneChan)

	go func() {
		select {
		case <-sigChan:
			restore()
			fmt.Fprintln(out)
			os.Exit(130)
		case <-doneChan:
		}
	}()

	return readMaskedLoop(label, in, out)
}

func readMaskedLoop(label string, in io.Reader, out io.Writer) (string, error) {
	var password []rune
	buf := make([]byte, 1)
	disallowedAttempted := false

	for {
		n, err := in.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) && len(password) > 0 {
				break
			}
			return "", err
		}
		if n == 0 {
			continue
		}
		b := buf[0]

		// Enter key
		if b == '\r' || b == '\n' {
			if len(password) == 0 && disallowedAttempted {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "Note: Chinese, Japanese, and non-standard characters are not allowed.")
				if label != "" {
					fmt.Fprint(out, label+": ")
				}
				disallowedAttempted = false
				continue
			}
			break
		}

		// Ctrl+C
		if b == 3 {
			return "", errors.New("interrupted")
		}

		// Ctrl+D
		if b == 4 {
			if len(password) == 0 {
				return "", io.EOF
			}
			break
		}

		// Backspace: ASCII DEL (127) or BS (8)
		if b == 127 || b == 8 {
			if len(password) > 0 {
				password = password[:len(password)-1]
				fmt.Fprint(out, "\b \b")
			}
			continue
		}

		// Ctrl+U: erase whole line
		if b == 21 {
			for range password {
				fmt.Fprint(out, "\b \b")
			}
			password = nil
			continue
		}

		// Escape sequence (arrows, delete, etc.)
		if b == 27 {
			if file, ok := in.(*os.File); ok {
				_ = file.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				seq := make([]byte, 8)
				sn, _ := file.Read(seq)
				_ = file.SetReadDeadline(time.Time{})
				if sn >= 2 && seq[0] == '[' && seq[1] == '3' {
					// Delete key acts like backspace
					if len(password) > 0 {
						password = password[:len(password)-1]
						fmt.Fprint(out, "\b \b")
					}
				}
			}
			continue
		}

		// Disallowed characters:
		// Any byte < 32 or > 126 (including all multi-byte UTF-8 bytes >= 128 for Chinese, Japanese, etc.)
		if b < 32 || b > 126 {
			disallowedAttempted = true
			continue
		}

		// Allowed printable ASCII character
		password = append(password, rune(b))
		fmt.Fprint(out, "*")
	}

	return string(password), nil
}

func readSecretFallback(in io.Reader, out io.Writer) (string, error) {
	line, err := readLineFrom(in)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if err := validatePassword(line); err != nil {
		return "", err
	}
	return line, nil
}

func readLineFrom(in io.Reader) (string, error) {
	if in == os.Stdin {
		line, err := readInputLine()
		return strings.TrimRight(line, "\r\n"), err
	}
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := in.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			line = append(line, b[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				break
			}
			return "", err
		}
	}
	return strings.TrimRight(string(line), "\r"), nil
}
