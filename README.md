# Passport Vault

Portable encrypted JSON vault for passport/password entries.

## Requirements

- PowerShell 7+
- Pester for tests

## Usage

```powershell
pwsh ./PassportVault.ps1
pwsh ./PassportVault.ps1 -VaultPath "D:\Backup\passport-vault.dat"
pwsh ./PassportVault.ps1 -VaultPath ".\passport-vault.dat" -KeyFilePath ".\vault.key"
```

The vault is encrypted on disk. The entry password and optional key file are required to unlock it. Losing them means the vault cannot be recovered.

## Security Notes

- The file is not KeePass `.kdbx` compatible.
- Default KDF is PBKDF2-HMAC-SHA256 with 600000 iterations.
- Default cipher is AES-GCM on PowerShell 7+.
- Passwords are hidden by default in the menu.
