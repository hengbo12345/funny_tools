# Passport Vault

Portable encrypted JSON vault for passport/password entries.

This branch implements a new Go-based vault format. It does not migrate or read vaults created by the earlier PowerShell implementation.

## Requirements

- Go 1.22+ to build from source
- No runtime network access or third-party runtime dependencies

## Build

```powershell
go build -o passportvault.exe .\cmd\passportvault
```

Cross-compile examples:

```powershell
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o dist\passportvault-windows-amd64.exe .\cmd\passportvault
$env:GOOS = "linux";   $env:GOARCH = "amd64"; go build -o dist\passportvault-linux-amd64 .\cmd\passportvault
$env:GOOS = "darwin";  $env:GOARCH = "arm64"; go build -o dist\passportvault-darwin-arm64 .\cmd\passportvault
```

## Usage

```powershell
.\passportvault.exe init
.\passportvault.exe add
.\passportvault.exe list
.\passportvault.exe search github
.\passportvault.exe show github
.\passportvault.exe -reveal show github
.\passportvault.exe edit github
.\passportvault.exe delete github
.\passportvault.exe change-password
```

By default, the vault is stored as `passport-vault-go.dat` in the current directory.

Use `-vault` to choose a file:

```powershell
.\passportvault.exe -vault D:\Backup\passport-vault-go.dat init
```

Use `-key-file` to require a separate key file:

```powershell
.\passportvault.exe -key-file .\vault.key init
.\passportvault.exe -key-file .\vault.key add
```

The key file must already exist, be non-empty, and remain unchanged for the lifetime of the vault. Back it up separately from the vault file.

Generate a key file:

```powershell
$key = New-Object 'byte[]' 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $rng.GetBytes($key)
    [System.IO.File]::WriteAllBytes(".\vault.key", $key)
} finally {
    $rng.Dispose()
    [Array]::Clear($key, 0, $key.Length)
}
```

For scripts, set the master password through an environment variable instead of an interactive prompt:

```powershell
$env:PASSPORTVAULT_PASSWORD = "correct horse battery staple"
.\passportvault.exe list
Remove-Item Env:\PASSPORTVAULT_PASSWORD
```

## Security Notes

- The file is not KeePass `.kdbx` compatible.
- The Go vault format is new and intentionally does not migrate old PowerShell vaults.
- Default KDF is PBKDF2-HMAC-SHA256 with 600000 iterations.
- Encryption is AES-256-GCM with authenticated vault metadata.
- Password prompts are hidden on Windows and Unix-like terminals.
- Losing the master password or required key file means the vault cannot be recovered.

## Future Plan

The code is split so `internal/vault` owns encryption, file format, and entry operations, while `cmd/passportvault` owns CLI behavior. A future TUI can reuse the same vault package without changing the file format.
