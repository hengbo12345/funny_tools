# Passport Vault

Portable encrypted JSON vault for passport/password entries.

## Requirements

- Windows PowerShell 5.1+ or PowerShell 7+
- Pester 5+ for tests

## Usage

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\PassportVault.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\PassportVault.ps1 -VaultPath "D:\Backup\passport-vault.dat"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\PassportVault.ps1 -VaultPath ".\passport-vault.dat" -KeyFilePath ".\vault.key"
```

When run without `-VaultPath`, the vault is stored beside the script as `passport-vault.dat`.

To debug a generic `Operation failed.` message, rerun with `-ShowErrorDetails`:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\PassportVault.ps1 -ShowErrorDetails
```

The vault is encrypted on disk. The entry password and optional key file are required to unlock it. Losing them means the vault cannot be recovered.

When using `-KeyFilePath`, the key file must already exist, be non-empty, and remain unchanged for the lifetime of the vault. Back it up separately from the vault file.

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

## Security Notes

- The file is not KeePass `.kdbx` compatible.
- Default KDF is PBKDF2-HMAC-SHA256 with 600000 iterations.
- New vaults use AES-CBC with HMAC-SHA256 for authenticated encryption so they work on Windows PowerShell 5.1.
- AES-GCM vaults created by older PowerShell 7-only builds are not supported in Windows PowerShell 5.1; this release does not include migration support.
- Passwords are hidden by default in the menu.
