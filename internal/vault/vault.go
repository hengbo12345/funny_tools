package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	formatName        = "PassportVaultGo"
	formatVersion     = 1
	schemaVersion     = 1
	defaultIterations = 600000
)

var (
	ErrVaultExists        = errors.New("vault already exists")
	ErrVaultNotFound      = errors.New("vault file not found")
	ErrCouldNotUnlock     = errors.New("could not unlock vault")
	ErrUnsupportedVault   = errors.New("unsupported vault format")
	ErrEntryNotFound      = errors.New("entry not found")
	ErrEntryAlreadyExists = errors.New("entry already exists")
)

type Vault struct {
	SchemaVersion int     `json:"schemaVersion"`
	Entries       []Entry `json:"entries"`
}

type Entry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type envelope struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	KDF        kdfInfo           `json:"kdf"`
	Cipher     cipherInfo        `json:"cipher"`
	Factors    factorsInfo       `json:"factors"`
	Ciphertext string            `json:"ciphertext"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type kdfInfo struct {
	Name       string `json:"name"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
}

type cipherInfo struct {
	Name  string `json:"name"`
	Nonce string `json:"nonce"`
}

type factorsInfo struct {
	KeyFile bool `json:"keyFile"`
}

func New() *Vault {
	return &Vault{
		SchemaVersion: schemaVersion,
		Entries:       []Entry{},
	}
}

func (v *Vault) Add(entry Entry) error {
	entry.Name = strings.TrimSpace(entry.Name)
	if entry.Name == "" {
		return errors.New("entry name is required")
	}
	if _, ok := v.Find(entry.Name); ok {
		return ErrEntryAlreadyExists
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if entry.ID == "" {
		id, err := randomID()
		if err != nil {
			return err
		}
		entry.ID = id
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	v.Entries = append(v.Entries, entry)
	sortEntries(v.Entries)
	return nil
}

func (v *Vault) Find(name string) (Entry, bool) {
	for _, entry := range v.Entries {
		if strings.EqualFold(entry.Name, name) {
			return entry, true
		}
	}
	return Entry{}, false
}

func (v *Vault) Search(query string) []Entry {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		out := append([]Entry(nil), v.Entries...)
		sortEntries(out)
		return out
	}
	var out []Entry
	for _, entry := range v.Entries {
		if strings.Contains(strings.ToLower(entry.Name), needle) ||
			strings.Contains(strings.ToLower(entry.Username), needle) ||
			strings.Contains(strings.ToLower(entry.Notes), needle) {
			out = append(out, entry)
		}
	}
	sortEntries(out)
	return out
}

func (v *Vault) Update(name string, replacement Entry) error {
	for i := range v.Entries {
		if strings.EqualFold(v.Entries[i].Name, name) {
			replacement.Name = strings.TrimSpace(replacement.Name)
			if replacement.Name == "" {
				return errors.New("entry name is required")
			}
			replacement.ID = v.Entries[i].ID
			replacement.CreatedAt = v.Entries[i].CreatedAt
			replacement.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			v.Entries[i] = replacement
			sortEntries(v.Entries)
			return nil
		}
	}
	return ErrEntryNotFound
}

func (v *Vault) Remove(name string) error {
	for i := range v.Entries {
		if strings.EqualFold(v.Entries[i].Name, name) {
			v.Entries = append(v.Entries[:i], v.Entries[i+1:]...)
			return nil
		}
	}
	return ErrEntryNotFound
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func SaveNewFile(path string, v *Vault, password string, keyFilePath string) error {
	if Exists(path) {
		return ErrVaultExists
	}
	return SaveFile(path, v, password, keyFilePath)
}

func SaveFile(path string, v *Vault, password string, keyFilePath string) error {
	if v == nil {
		v = New()
	}
	if v.SchemaVersion == 0 {
		v.SchemaVersion = schemaVersion
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	env, err := protect(plain, password, keyFilePath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileReplace(path, data)
}

func LoadFile(path string, password string, keyFilePath string) (*Vault, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrVaultNotFound
	}
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, ErrCouldNotUnlock
	}
	plain, err := unprotect(env, password, keyFilePath)
	if err != nil {
		return nil, err
	}
	var v Vault
	if err := json.Unmarshal(plain, &v); err != nil {
		return nil, ErrCouldNotUnlock
	}
	if v.SchemaVersion != schemaVersion {
		return nil, ErrUnsupportedVault
	}
	if v.Entries == nil {
		v.Entries = []Entry{}
	}
	sortEntries(v.Entries)
	return &v, nil
}

func protect(plain []byte, password string, keyFilePath string) (envelope, error) {
	salt, err := randomBytes(32)
	if err != nil {
		return envelope{}, err
	}
	nonce, err := randomBytes(12)
	if err != nil {
		return envelope{}, err
	}
	keyFileDigest, usesKeyFile, err := keyFileDigest(keyFilePath)
	if err != nil {
		return envelope{}, err
	}
	key := deriveKey(password, keyFileDigest, salt, defaultIterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return envelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return envelope{}, err
	}
	env := envelope{
		Format:  formatName,
		Version: formatVersion,
		KDF: kdfInfo{
			Name:       "PBKDF2-HMAC-SHA256",
			Iterations: defaultIterations,
			Salt:       base64.StdEncoding.EncodeToString(salt),
		},
		Cipher: cipherInfo{
			Name:  "AES-256-GCM",
			Nonce: base64.StdEncoding.EncodeToString(nonce),
		},
		Factors: factorsInfo{KeyFile: usesKeyFile},
	}
	aad, err := protectedHeader(env)
	if err != nil {
		return envelope{}, err
	}
	env.Ciphertext = base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, aad))
	return env, nil
}

func unprotect(env envelope, password string, keyFilePath string) ([]byte, error) {
	if env.Format != formatName || env.Version != formatVersion {
		return nil, ErrUnsupportedVault
	}
	if env.KDF.Name != "PBKDF2-HMAC-SHA256" || env.Cipher.Name != "AES-256-GCM" {
		return nil, ErrUnsupportedVault
	}
	salt, err := base64.StdEncoding.DecodeString(env.KDF.Salt)
	if err != nil || len(salt) != 32 {
		return nil, ErrCouldNotUnlock
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Cipher.Nonce)
	if err != nil || len(nonce) != 12 {
		return nil, ErrCouldNotUnlock
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	keyFileDigest, usesKeyFile, err := keyFileDigest(keyFilePath)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	if env.Factors.KeyFile != usesKeyFile {
		return nil, ErrCouldNotUnlock
	}
	key := deriveKey(password, keyFileDigest, salt, env.KDF.Iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	aad, err := protectedHeader(env)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrCouldNotUnlock
	}
	return plain, nil
}

func protectedHeader(env envelope) ([]byte, error) {
	header := struct {
		Format  string      `json:"format"`
		Version int         `json:"version"`
		KDF     kdfInfo     `json:"kdf"`
		Cipher  cipherInfo  `json:"cipher"`
		Factors factorsInfo `json:"factors"`
	}{
		Format:  env.Format,
		Version: env.Version,
		KDF:     env.KDF,
		Cipher:  env.Cipher,
		Factors: env.Factors,
	}
	return json.Marshal(header)
}

func deriveKey(password string, keyFileDigest []byte, salt []byte, iterations int, length int) []byte {
	passwordBytes := []byte(password)
	frame := bytes.NewBuffer(nil)
	frame.WriteString("PassportVaultGo-KDF-v1")
	_ = binary.Write(frame, binary.BigEndian, uint32(len(passwordBytes)))
	frame.Write(passwordBytes)
	frame.Write(keyFileDigest)
	return pbkdf2SHA256(frame.Bytes(), salt, iterations, length)
}

func pbkdf2SHA256(password []byte, salt []byte, iterations int, length int) []byte {
	hashLen := sha256.Size
	numBlocks := (length + hashLen - 1) / hashLen
	out := make([]byte, 0, numBlocks*hashLen)
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var blockBytes [4]byte
		binary.BigEndian.PutUint32(blockBytes[:], uint32(block))
		mac.Write(blockBytes[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 2; i <= iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length]
}

func keyFileDigest(path string) ([]byte, bool, error) {
	if strings.TrimSpace(path) == "" {
		sum := sha256.Sum256(nil)
		return sum[:], false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, false, errors.New("key file is empty")
	}
	sum := sha256.Sum256(data)
	return sum[:], true, nil
}

func randomBytes(length int) ([]byte, error) {
	out := make([]byte, length)
	_, err := io.ReadFull(rand.Reader, out)
	return out, err
}

func randomID() (string, error) {
	bytes, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func writeFileReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace vault file: %w", err)
	}
	return nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}
