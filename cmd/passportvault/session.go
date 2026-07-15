package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"passportvault/internal/vault"
)

type session struct {
	cfg      appConfig
	password string
	reader   *bufio.Reader
	out      io.Writer
	vault    *vault.Vault
}

func RunInteractiveSession(cfg appConfig, password string, input io.Reader, output io.Writer) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	s := &session{
		cfg:      cfg,
		password: password,
		reader:   bufio.NewReader(input),
		out:      output,
		vault:    v,
	}
	return s.loop()
}

func (s *session) loop() error {
	for {
		fmt.Fprintln(s.out)
		fmt.Fprintln(s.out, "PassportVault")
		fmt.Fprintln(s.out, "1. List entries")
		fmt.Fprintln(s.out, "2. Search entries")
		fmt.Fprintln(s.out, "3. Add entry")
		fmt.Fprintln(s.out, "4. View entry")
		fmt.Fprintln(s.out, "5. Edit entry")
		fmt.Fprintln(s.out, "6. Delete entry")
		fmt.Fprintln(s.out, "7. Change master password")
		fmt.Fprintln(s.out, "8. Quit")
		choice, err := s.prompt("Choose")
		if err != nil {
			return err
		}
		switch strings.ToLower(choice) {
		case "1", "list", "l":
			if err := s.listEntries(s.vault.Search("")); err != nil {
				return err
			}
		case "2", "search", "s":
			if err := s.searchEntries(); err != nil {
				return err
			}
		case "3", "add", "a":
			if err := s.addEntry(); err != nil {
				return err
			}
		case "4", "view", "v":
			if err := s.viewEntryByName(); err != nil {
				return err
			}
		case "5", "edit", "e":
			if err := s.editEntryByName(); err != nil {
				return err
			}
		case "6", "delete", "d":
			if err := s.deleteEntryByName(); err != nil {
				return err
			}
		case "7", "change-password", "password":
			if err := s.changePassword(); err != nil {
				return err
			}
		case "8", "quit", "q", "exit":
			return nil
		default:
			fmt.Fprintln(s.out, "Invalid choice.")
		}
	}
}

func (s *session) listEntries(entries []vault.Entry) error {
	if len(entries) == 0 {
		fmt.Fprintln(s.out, "No entries.")
		return nil
	}
	for i, entry := range entries {
		fmt.Fprintf(s.out, "%d. %s\t%s\n", i+1, entry.Name, entry.Username)
	}
	choice, err := s.prompt("Open number, or Enter to return")
	if err != nil {
		return err
	}
	if choice == "" {
		return nil
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(entries) {
		fmt.Fprintln(s.out, "Invalid choice.")
		return nil
	}
	return s.showEntry(entries[index-1])
}

func (s *session) searchEntries() error {
	query, err := s.prompt("Search")
	if err != nil {
		return err
	}
	return s.listEntries(s.vault.Search(query))
}

func (s *session) addEntry() error {
	entry, err := s.promptEntry(vault.Entry{})
	if err != nil {
		return err
	}
	if err := s.vault.Add(entry); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Entry added:", entry.Name)
	return nil
}

func (s *session) viewEntryByName() error {
	entry, ok, err := s.selectEntry("Entry name or number")
	if err != nil || !ok {
		return err
	}
	return s.showEntry(entry)
}

func (s *session) showEntry(entry vault.Entry) error {
	for {
		fmt.Fprintln(s.out, "Name:", entry.Name)
		fmt.Fprintln(s.out, "Username:", entry.Username)
		fmt.Fprintln(s.out, "Password: ********")
		if entry.Notes != "" {
			fmt.Fprintln(s.out, "Notes:", entry.Notes)
		}
		action, err := s.prompt("[R]eveal  [C]opy password  [E]dit  [D]elete  Enter=back")
		if err != nil {
			return err
		}
		switch strings.ToLower(action) {
		case "":
			return nil
		case "r", "reveal":
			fmt.Fprintln(s.out, "Password:", entry.Password)
		case "c", "copy":
			if err := copyToClipboard(entry.Password); err != nil {
				fmt.Fprintln(s.out, "Clipboard unavailable:", err)
			} else {
				fmt.Fprintln(s.out, "Password copied to clipboard.")
			}
		case "e", "edit":
			if err := s.editEntry(entry.Name); err != nil {
				return err
			}
			var ok bool
			entry, ok = s.vault.Find(entry.Name)
			if !ok {
				return nil
			}
		case "d", "delete":
			if err := s.deleteEntry(entry.Name); err != nil {
				return err
			}
			return nil
		default:
			fmt.Fprintln(s.out, "Invalid choice.")
		}
	}
}

func (s *session) editEntryByName() error {
	entry, ok, err := s.selectEntry("Entry name or number")
	if err != nil || !ok {
		return err
	}
	return s.editEntry(entry.Name)
}

func (s *session) editEntry(name string) error {
	current, ok := s.vault.Find(name)
	if !ok {
		return vault.ErrEntryNotFound
	}
	replacement, err := s.promptEntry(current)
	if err != nil {
		return err
	}
	if err := s.vault.Update(name, replacement); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Entry updated:", replacement.Name)
	return nil
}

func (s *session) deleteEntryByName() error {
	entry, ok, err := s.selectEntry("Entry name or number")
	if err != nil || !ok {
		return err
	}
	return s.deleteEntry(entry.Name)
}

func (s *session) deleteEntry(name string) error {
	if !s.confirm("Delete " + name + "?") {
		fmt.Fprintln(s.out, "Canceled.")
		return nil
	}
	if err := s.vault.Remove(name); err != nil {
		return err
	}
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Entry deleted:", name)
	return nil
}

func (s *session) changePassword() error {
	next, err := s.promptSecret("New master password")
	if err != nil {
		return err
	}
	confirmNext, err := s.promptSecret("Confirm new master password")
	if err != nil {
		return err
	}
	if next != confirmNext {
		return errors.New("passwords do not match")
	}
	s.password = next
	if err := s.save(); err != nil {
		return err
	}
	fmt.Fprintln(s.out, "Master password changed.")
	return nil
}

func (s *session) selectEntry(label string) (vault.Entry, bool, error) {
	entries := s.vault.Search("")
	if len(entries) == 0 {
		fmt.Fprintln(s.out, "No entries.")
		return vault.Entry{}, false, nil
	}
	for i, entry := range entries {
		fmt.Fprintf(s.out, "%d. %s\t%s\n", i+1, entry.Name, entry.Username)
	}
	choice, err := s.prompt(label)
	if err != nil {
		return vault.Entry{}, false, err
	}
	if choice == "" {
		return vault.Entry{}, false, nil
	}
	if index, err := strconv.Atoi(choice); err == nil {
		if index >= 1 && index <= len(entries) {
			return entries[index-1], true, nil
		}
		fmt.Fprintln(s.out, "Invalid choice.")
		return vault.Entry{}, false, nil
	}
	entry, ok := s.vault.Find(choice)
	if !ok {
		fmt.Fprintln(s.out, "Entry not found.")
		return vault.Entry{}, false, nil
	}
	return entry, true, nil
}

func (s *session) promptEntry(current vault.Entry) (vault.Entry, error) {
	name, err := s.promptDefault("Name", current.Name)
	if err != nil {
		return vault.Entry{}, err
	}
	username, err := s.promptDefault("Username", current.Username)
	if err != nil {
		return vault.Entry{}, err
	}
	passwordPrompt := "Password"
	if current.Password != "" {
		passwordPrompt = "Password (leave blank to keep current)"
	}
	password, err := s.promptSecret(passwordPrompt)
	if err != nil {
		return vault.Entry{}, err
	}
	if password == "" {
		password = current.Password
	}
	notes, err := s.promptDefault("Notes", current.Notes)
	if err != nil {
		return vault.Entry{}, err
	}
	return vault.Entry{Name: name, Username: username, Password: password, Notes: notes}, nil
}

func (s *session) promptDefault(label string, current string) (string, error) {
	prompt := label
	if current != "" {
		prompt += " [" + current + "]"
	}
	value, err := s.prompt(prompt)
	if err != nil {
		return "", err
	}
	if value == "" {
		return current, nil
	}
	return value, nil
}

func (s *session) promptSecret(label string) (string, error) {
	return s.prompt(label)
}

func (s *session) prompt(label string) (string, error) {
	fmt.Fprint(s.out, label+": ")
	value, err := s.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (s *session) confirm(label string) bool {
	value, err := s.prompt(label + " (y/N)")
	if err != nil {
		return false
	}
	return strings.EqualFold(value, "y")
}

func (s *session) save() error {
	return vault.SaveFile(s.cfg.vaultPath, s.vault, s.password, s.cfg.keyFilePath)
}

func copyToClipboard(value string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return errors.New("no clipboard command found")
		}
	}
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}
