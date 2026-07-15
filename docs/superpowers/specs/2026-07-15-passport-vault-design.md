# Passport Vault PowerShell Script Design

## Purpose

Build a Windows PowerShell script that safely stores passport/password entries in a portable encrypted JSON vault. The tool should be convenient for local command-line use while protecting entries at rest with a KeePass-inspired layered security model.

This project will not implement KeePass `.kdbx` compatibility. It will use similar concepts: a master entry password, optional additional factors, password-based key derivation, authenticated encryption, and a structured encrypted vault file.

## User Flow

1. User runs `PassportVault.ps1`.
2. Script prompts for the entry password with hidden input.
3. If the vault file exists, the script derives keys and attempts to decrypt it.
4. If the vault file does not exist, the script asks the user to confirm the entry password and creates a new empty vault.
5. After unlock, the script shows a menu:
   - List entries
   - Search entries
   - Add entry
   - View entry
   - Modify entry
   - Delete entry
   - Change entry password
   - Exit
6. Entry view hides the stored password by default. The user must explicitly choose reveal or copy.
7. Add, modify, delete, and password-change operations save the full vault back to disk encrypted.
8. On exit, the script clears sensitive variables as much as PowerShell reasonably allows.

## Entry Model

The decrypted payload is JSON. Each entry contains:

```json
{
  "id": "guid",
  "name": "example.com",
  "username": "alice",
  "password": "secret",
  "notes": "",
  "createdAt": "2026-07-15T00:00:00Z",
  "updatedAt": "2026-07-15T00:00:00Z"
}
```

The top-level decrypted JSON contains a vault schema version and an array of entries. Search matches `name`, `username`, and `notes`; it does not search password values.

## Vault File Format

The on-disk file is encrypted data plus metadata, encoded as JSON for inspectable structure:

```json
{
  "format": "PassportVault",
  "version": 1,
  "kdf": {
    "name": "PBKDF2-SHA256",
    "iterations": 600000,
    "salt": "base64"
  },
  "cipher": {
    "name": "AES-GCM",
    "nonce": "base64",
    "tag": "base64"
  },
  "factors": {
    "keyFile": false,
    "windowsUserBinding": false
  },
  "ciphertext": "base64"
}
```

The metadata is not secret, but security-critical metadata is authenticated. The implementation will build a canonical protected header from format, version, kdf, cipher.name, cipher.nonce, and factors. For AES-GCM, that canonical protected header is passed as associated data. For AES-CBC/HMAC fallback, it is included in the HMAC input together with the IV and ciphertext.

## Security Design

The default unlock factor is the entry password. The script will support an optional key file as a second factor. Windows-user binding is reserved for a later enhancement and remains off by default so the vault stays portable.

Default key derivation is PBKDF2-HMAC-SHA256 because it is available in standard PowerShell/.NET. The design allows adding Argon2id later if a reliable local .NET dependency is introduced. The initial implementation should avoid network dependency downloads during normal use.

Preferred encryption is AES-GCM when available in the host PowerShell/.NET runtime. AES-GCM provides confidentiality and authentication in one primitive. If AES-GCM is unavailable, the fallback is AES-CBC with PKCS7 padding plus HMAC-SHA256 using encrypt-then-MAC. The fallback derives independent encryption and MAC keys.

Every save generates fresh random encryption parameters. The implementation must use a cryptographically secure random number generator for salts, nonces, IVs, and generated IDs where relevant.

Wrong passwords, missing key files, unsupported versions, corrupted files, and tampered vault contents must fail cleanly without exposing partial decrypted data. The script must authenticate the protected header and ciphertext before parsing decrypted JSON.

## Convenience Behavior

The script defaults to a vault file beside the script, named `passport-vault.dat`. A `-VaultPath` parameter can override this for backup, sync-folder, or USB-drive use.

Menu commands are numeric for fast terminal use. Search is case-insensitive and returns numbered results that can be opened directly. Password reveal is explicit. Clipboard copy is supported where available and should clear the clipboard after a short delay when the platform supports it.

## Components

- `Read-EntryPassword`: prompt for hidden password input.
- `Get-PlainTextFromSecureString`: convert password only for key derivation, then clear temporary values where possible.
- `New-KeyMaterial`: combine password and optional key-file material, then derive encryption keys.
- `Protect-VaultPayload`: serialize, encrypt, and authenticate decrypted JSON.
- `Unprotect-VaultPayload`: authenticate, decrypt, and parse JSON.
- `Load-Vault`: read the vault file, validate metadata, and decrypt.
- `Save-Vault`: write encrypted vault data atomically.
- `Show-MainMenu`: command loop.
- `Add-Entry`, `Search-Entries`, `Show-Entry`, `Edit-Entry`, `Remove-Entry`: entry operations.
- `Set-VaultPassword`: re-encrypt the vault with a new entry password and optional new factors.

## Error Handling

The script should print clear, non-sensitive error messages:

- `Vault file not found` when loading an explicit missing path.
- `Could not unlock vault` for wrong password, wrong key file, corruption, or tampering.
- `Unsupported vault version` for newer incompatible formats.
- `Could not save vault` when atomic write or replacement fails.

Detailed cryptographic exception messages should not be shown to the user because they may leak implementation details.

## Testing Strategy

Tests should cover:

- Encrypt/decrypt round trip.
- Wrong password fails.
- Tampered ciphertext fails.
- Tampered metadata fails.
- New vault creation.
- Add, search, edit, and delete operations against in-memory vault data.
- Vault version checks.
- AES-GCM path when available.
- AES-CBC/HMAC fallback path if implemented on the current runtime.

Manual verification should include running the script, creating a vault, adding an entry, searching it, revealing it, modifying it, exiting, reopening the script, and confirming the data survives only with the correct entry password.

## Scope Boundaries

In scope:

- One PowerShell script.
- Portable encrypted JSON vault file.
- Entry password plus optional key file security model.
- Local command-line menu workflow.

Out of scope for the first version:

- KeePass `.kdbx` import/export compatibility.
- Cloud sync conflict resolution.
- Browser integration.
- Auto-type.
- Secure desktop mode.
- Multi-user shared vault editing.
- Recovery if the entry password and key file are lost.
