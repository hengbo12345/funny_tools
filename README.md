# Passport Vault

Portable encrypted JSON vault for passport/password entries.

## Requirements

- PowerShell 7+
- Pester 5+ for tests

## Usage

```powershell
pwsh ./PassportVault.ps1
pwsh ./PassportVault.ps1 -VaultPath "D:\Backup\passport-vault.dat"
pwsh ./PassportVault.ps1 -VaultPath ".\passport-vault.dat" -KeyFilePath ".\vault.key"
```

When run without `-VaultPath`, the vault is stored beside the script as `passport-vault.dat`.

The vault is encrypted on disk. The entry password and optional key file are required to unlock it. Losing them means the vault cannot be recovered.

When using `-KeyFilePath`, the key file must already exist, be non-empty, and remain unchanged for the lifetime of the vault. Back it up separately from the vault file.

```powershell
$key = [byte[]]::new(32)
[System.Security.Cryptography.RandomNumberGenerator]::Fill($key)
[System.IO.File]::WriteAllBytes(".\vault.key", $key)
[Array]::Clear($key, 0, $key.Length)
```

## Security Notes

- The file is not KeePass `.kdbx` compatible.
- Default KDF is PBKDF2-HMAC-SHA256 with 600000 iterations.
- AES-GCM is preferred when the host supports it; AES-CBC with HMAC-SHA256 is the authenticated fallback when AES-GCM is unavailable.
- Passwords are hidden by default in the menu.
