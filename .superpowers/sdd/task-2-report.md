# Task 2 Report: Crypto Module

## What Was Implemented

- Added `src/PassportVault.Crypto.psm1` with the required public functions:
  `New-RandomBytes`, `New-KeyMaterial`, `Protect-VaultPayload`, and
  `Unprotect-VaultPayload`.
- Implemented AES-GCM vault payload encryption using a 32-byte random salt,
  12-byte nonce, 16-byte authentication tag, and PBKDF2-SHA256 with 600000
  default iterations.
- Implemented password plus optional key-file key material derivation and the
  required PassportVault version 1 envelope.
- Authenticated format, version, KDF, cipher nonce, and factor metadata as
  AES-GCM additional authenticated data.
- Added the four specified Pester tests for round-tripping, wrong passwords,
  ciphertext tampering, and protected metadata tampering.
- Committed the implementation as `33d6635 feat: add vault encryption`.

## Testing And Results

Attempted RED phase command after writing the tests:

```text
pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"
```

Result:

```text
/bin/bash: line 1: pwsh: command not found
```

Attempted the same GREEN phase command after implementation. It produced the
same environment failure with exit code 127. Pester tests were not executed,
so no passing test claim is made.

Static verification completed successfully:

```text
git diff --check
git diff --cached --check
git show --check --stat --oneline HEAD
```

All three commands exited 0 without whitespace errors.

## TDD Evidence / Blocker

The Pester tests were created before the crypto module. The exact required RED
command was run before implementation and could not start because `pwsh` is
not installed or available on `PATH`. The exact GREEN command was run after
implementation and was blocked by the same missing executable.

## Files Changed

- `src/PassportVault.Crypto.psm1`
- `tests/PassportVault.Crypto.Tests.ps1`
- `.superpowers/sdd/task-2-report.md`

## Self-Review Findings

- Public function names and parameter types match the Task 2 contract.
- The envelope uses the required format (`PassportVault`), version (`1`), KDF
  (`PBKDF2-SHA256`), cipher (`AES-GCM`), default iteration count (`600000`),
  salt length (`32`), nonce length (`12`), and tag length (`16`).
- The protected header contains format, version, KDF name/iterations/salt,
  cipher name/nonce, and both factor flags; it is used as AES-GCM AAD for both
  encryption and decryption.
- The committed patch contains only the two task-owned source and test files.
- No static-review defects were found. Runtime PowerShell/Pester verification
  remains outstanding.

## Concerns

- PowerShell 7+ and Pester are unavailable in this container because `pwsh`
  is not installed. Run the specified Pester command in a PowerShell 7+
  environment with AES-GCM and Pester support before relying on runtime
  verification.

## Review Fixes (Task 2)

### Findings Fixed

- Added AES-CBC with HMAC-SHA256 encrypt-then-MAC support via `ForceCipher = "AES-CBC-HMAC"` and authenticated CBC envelope metadata.
- Rejected empty key files, required a configured key file at unlock, and rejected an optional key file for vaults not configured to use one.
- Validated envelope factor policy, Base64 fields, metadata types and lengths, and PBKDF2 iteration bounds before key derivation.
- Normalized corrupt metadata, key-file failures, KDF failures, and authentication failures to `Could not unlock vault`, while preserving `Unsupported vault version` for unsupported format, version, KDF, or cipher.
- Kept the exact default PBKDF2 iteration count at `600000`; documented supported bounds of `100000` through `1000000` in module constants.

### Test Commands And Results

- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"` failed with exit code 127: `/bin/bash: line 1: pwsh: command not found`.
- `git diff --check` completed successfully before commit.

### Files Changed

- `src/PassportVault.Crypto.psm1`
- `tests/PassportVault.Crypto.Tests.ps1`
- `.superpowers/sdd/task-2-report.md`

### Remaining Concerns

- PowerShell 7+ and Pester are unavailable in this container, so the new runtime tests could not be executed here.

### Post-Commit Static Verification

- `git diff --check`, `git diff --cached --check`, and `git show --check --stat --oneline HEAD` all completed with exit code 0.
