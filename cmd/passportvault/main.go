package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"passportvault/internal/vault"
)

type appConfig struct {
	vaultPath   string
	keyFilePath string
	passwordEnv string
	reveal      bool
}

var stdinReader = bufio.NewReader(os.Stdin)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, command, rest, err := parseArgs(args)
	if err != nil {
		return err
	}
	password, err := readMasterPassword(cfg.passwordEnv)
	if err != nil {
		return err
	}

	switch command {
	case "session":
		return RunInteractiveSession(cfg, password, os.Stdin, os.Stdout)
	case "init":
		return commandInit(cfg, password)
	case "add":
		return commandAdd(cfg, password)
	case "list":
		return commandList(cfg, password)
	case "search":
		if len(rest) != 1 {
			return errors.New("usage: passportvault search <query>")
		}
		return commandSearch(cfg, password, rest[0])
	case "show":
		if len(rest) != 1 {
			return errors.New("usage: passportvault show [-reveal] <name>")
		}
		return commandShow(cfg, password, rest[0])
	case "edit":
		if len(rest) != 1 {
			return errors.New("usage: passportvault edit <name>")
		}
		return commandEdit(cfg, password, rest[0])
	case "delete":
		if len(rest) != 1 {
			return errors.New("usage: passportvault delete <name>")
		}
		return commandDelete(cfg, password, rest[0])
	case "change-password":
		return commandChangePassword(cfg, password)
	default:
		return usageError(command)
	}
}

func parseArgs(args []string) (appConfig, string, []string, error) {
	cfg := appConfig{}
	flags := flag.NewFlagSet("passportvault", flag.ContinueOnError)
	flags.StringVar(&cfg.vaultPath, "vault", "passport-vault-go.dat", "vault file path")
	flags.StringVar(&cfg.keyFilePath, "key-file", "", "optional key file path")
	flags.StringVar(&cfg.passwordEnv, "password-env", "PASSPORTVAULT_PASSWORD", "environment variable containing the master password")
	flags.BoolVar(&cfg.reveal, "reveal", false, "reveal password for show command")
	if err := flags.Parse(args); err != nil {
		return cfg, "", nil, err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return cfg, "session", nil, nil
	}
	return cfg, rest[0], rest[1:], nil
}

func commandInit(cfg appConfig, password string) error {
	confirm, err := promptSecret("Confirm master password")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("passwords do not match")
	}
	if err := vault.SaveNewFile(cfg.vaultPath, vault.New(), password, cfg.keyFilePath); err != nil {
		return err
	}
	fmt.Println("Vault created:", cfg.vaultPath)
	return nil
}

func commandAdd(cfg appConfig, password string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	entry, err := promptEntry(vault.Entry{})
	if err != nil {
		return err
	}
	if err := v.Add(entry); err != nil {
		return err
	}
	if err := vault.SaveFile(cfg.vaultPath, v, password, cfg.keyFilePath); err != nil {
		return err
	}
	fmt.Println("Entry added:", entry.Name)
	return nil
}

func commandList(cfg appConfig, password string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	for _, entry := range v.Search("") {
		fmt.Printf("%s\t%s\n", entry.Name, entry.Username)
	}
	return nil
}

func commandSearch(cfg appConfig, password string, query string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	for _, entry := range v.Search(query) {
		fmt.Printf("%s\t%s\n", entry.Name, entry.Username)
	}
	return nil
}

func commandShow(cfg appConfig, password string, name string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	entry, ok := v.Find(name)
	if !ok {
		return vault.ErrEntryNotFound
	}
	fmt.Println("Name:", entry.Name)
	fmt.Println("Username:", entry.Username)
	if cfg.reveal {
		fmt.Println("Password:", entry.Password)
	} else {
		fmt.Println("Password: ********")
	}
	if entry.Notes != "" {
		fmt.Println("Notes:", entry.Notes)
	}
	return nil
}

func commandEdit(cfg appConfig, password string, name string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	current, ok := v.Find(name)
	if !ok {
		return vault.ErrEntryNotFound
	}
	replacement, err := promptEntry(current)
	if err != nil {
		return err
	}
	if err := v.Update(name, replacement); err != nil {
		return err
	}
	if err := vault.SaveFile(cfg.vaultPath, v, password, cfg.keyFilePath); err != nil {
		return err
	}
	fmt.Println("Entry updated:", replacement.Name)
	return nil
}

func commandDelete(cfg appConfig, password string, name string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	if !confirm("Delete " + name + "?") {
		fmt.Println("Canceled.")
		return nil
	}
	if err := v.Remove(name); err != nil {
		return err
	}
	if err := vault.SaveFile(cfg.vaultPath, v, password, cfg.keyFilePath); err != nil {
		return err
	}
	fmt.Println("Entry deleted:", name)
	return nil
}

func commandChangePassword(cfg appConfig, password string) error {
	v, err := loadExisting(cfg, password)
	if err != nil {
		return err
	}
	next, err := promptSecret("New master password")
	if err != nil {
		return err
	}
	confirmNext, err := promptSecret("Confirm new master password")
	if err != nil {
		return err
	}
	if next != confirmNext {
		return errors.New("passwords do not match")
	}
	if err := vault.SaveFile(cfg.vaultPath, v, next, cfg.keyFilePath); err != nil {
		return err
	}
	fmt.Println("Master password changed.")
	return nil
}

func loadExisting(cfg appConfig, password string) (*vault.Vault, error) {
	v, err := vault.LoadFile(cfg.vaultPath, password, cfg.keyFilePath)
	if errors.Is(err, vault.ErrVaultNotFound) {
		return nil, fmt.Errorf("%w: run init first", err)
	}
	return v, err
}

func promptEntry(current vault.Entry) (vault.Entry, error) {
	name, err := promptLineDefault("Name", current.Name)
	if err != nil {
		return vault.Entry{}, err
	}
	username, err := promptLineDefault("Username", current.Username)
	if err != nil {
		return vault.Entry{}, err
	}
	passwordPrompt := "Password"
	if current.Password != "" {
		passwordPrompt = "Password (leave blank to keep current)"
	}
	password, err := promptSecret(passwordPrompt)
	if err != nil {
		return vault.Entry{}, err
	}
	if password == "" {
		password = current.Password
	}
	notes, err := promptLineDefault("Notes", current.Notes)
	if err != nil {
		return vault.Entry{}, err
	}
	return vault.Entry{Name: name, Username: username, Password: password, Notes: notes}, nil
}

func readMasterPassword(envName string) (string, error) {
	if envName != "" {
		if value := os.Getenv(envName); value != "" {
			return value, nil
		}
	}
	return promptSecret("Master password")
}

func promptLineDefault(label string, current string) (string, error) {
	prompt := label
	if current != "" {
		prompt += " [" + current + "]"
	}
	fmt.Print(prompt + ": ")
	value, err := readInputLine()
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return current, nil
	}
	return value, nil
}

func readInputLine() (string, error) {
	return stdinReader.ReadString('\n')
}

func promptSecret(label string) (string, error) {
	return readSecretLine(label)
}

func confirm(label string) bool {
	fmt.Print(label + " (y/N): ")
	value, err := readInputLine()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "y")
}

func usageError(command string) error {
	if command == "" {
		return errors.New(usageText())
	}
	return fmt.Errorf("unknown command %q\n\n%s", command, usageText())
}

func usageText() string {
	return `Usage:
  passportvault [flags]
  passportvault [flags] session
  passportvault [flags] init
  passportvault [flags] add
  passportvault [flags] list
  passportvault [flags] search <query>
  passportvault [flags] show [-reveal] <name>
  passportvault [flags] edit <name>
  passportvault [flags] delete <name>
  passportvault [flags] change-password

Flags:
  -vault <path>          Vault file path, default passport-vault-go.dat
  -key-file <path>       Optional key file path
  -password-env <name>   Env var containing master password, default PASSPORTVAULT_PASSWORD
  -reveal                Reveal password for show command`
}
