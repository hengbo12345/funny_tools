# Final Fix Report

## Findings Fixed

- Byte-array helper outputs now use scalar array returns, including empty arrays, with typed receiving variables and exact `System.Byte[]` regression assertions.
- Password and key-file factors now use a domain marker, a big-endian password-length prefix, password bytes, and a SHA-256 key-file digest. The boundary-collision regression covers `a` + `bc` versus `ab` + `c`.
- Atomic replacement now tracks confirmed `File.Replace` success and deletes a backup only after success. A testable helper verifies that backups survive the failure state.
- `Search-VaultEntries` preserves its declared `object[]` contract for zero and one result.
- Numbered search results can be opened directly. Entry details keep passwords masked by default and offer explicit reveal or clipboard copy.
- Clipboard copy conditionally schedules a 30-second clear and clears only when the clipboard is unchanged.
- Default supported hosts assert AES-GCM selection. The fallback test mocks the GCM-construction seam to throw an actual `PlatformNotSupportedException`, exercising the production catch path.
- README cipher guidance now describes AES-GCM as preferred and AES-CBC/HMAC-SHA256 as fallback.
- Top-level cleanup drops `$password`, `$confirm`, and `$vault` references in `finally`; password-change confirmation references are also dropped locally.

## Tests/Commands Attempted and Results

- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Store.Tests.ps1 -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Entries.Tests.ps1 -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `pwsh -NoProfile -Command "Invoke-Pester -Path tests -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- PowerShell parser command over `PassportVault.ps1`, `src`, and `tests`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `powershell -NoProfile -Command "$PSVersionTable.PSVersion"`: exit 127, `/bin/bash: line 1: powershell: command not found`.
- `git diff --check`: exit 0 before staging and after commit.
- `git diff --cached --check`: exit 0 before and after the code commit.
- Targeted static scans found no remaining `SimulateAesGcmUnavailable`, unwrapped `return $bytes`, direct empty-byte-array return, or direct backup deletion pattern.
- `git show --check --stat --oneline HEAD` after code commit `bde207a`: exit 0.

Runtime tests and parser validation were not executed because neither PowerShell executable is installed.

## Files Changed

- `src/PassportVault.Crypto.psm1`
- `tests/PassportVault.Crypto.Tests.ps1`
- `src/PassportVault.Store.psm1`
- `tests/PassportVault.Store.Tests.ps1`
- `src/PassportVault.Entries.psm1`
- `tests/PassportVault.Entries.Tests.ps1`
- `PassportVault.ps1`
- `README.md`
- `.superpowers/sdd/final-fix-report.md`

## Remaining Concerns

- Pester and PowerShell parser results remain unknown until the branch is run on a host with `pwsh` and Pester.
- The new framed KDF input intentionally derives different keys from the previous branch implementation. Vault files created by pre-fix commits cannot be unlocked without a migration path.

## Whole-Branch Re-Review Fixes

### Findings Fixed

- The AES-GCM protected-metadata test now uses the valid bounded iteration count `600001`, forces AES-GCM, and asserts the normalized authentication failure.
- `Save-Vault` now calls the internal `Invoke-VaultFileReplacement` wrapper; the regression test writes a backup, forces that production-path replacement to fail, asserts `Could not save vault`, and verifies the backup remains.
- Clipboard timers retain returned event jobs, remove completed jobs before scheduling another timer, and remove outstanding jobs, event subscriptions, and timers during top-level cleanup. Event `MessageData` holds a SHA-256 digest instead of the plaintext password.
- README documents Pester 5+, key-file existence, non-empty and unchanged requirements, separate backup guidance, and a creation example.

### Commands And Results

- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Store.Tests.ps1 -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `pwsh -NoProfile -Command "Invoke-Pester -Path tests -Output Detailed"`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- PowerShell parser command over `PassportVault.ps1`, `src`, and `tests`: exit 127, `/bin/bash: line 1: pwsh: command not found`.
- `git diff --check`: exit 0 before staging.
- `git diff --cached --check`: exit 0 before staging.
- Static scans confirmed `600001`, the replacement wrapper invocation, event-job removal, digest-only event metadata, Pester 5+, and key-file lifecycle documentation.

### Files Changed

- `tests/PassportVault.Crypto.Tests.ps1`
- `src/PassportVault.Store.psm1`
- `tests/PassportVault.Store.Tests.ps1`
- `PassportVault.ps1`
- `README.md`
- `.superpowers/sdd/final-fix-report.md`

### Remaining Concerns

### Post-Commit Static Verification

- `git diff --check`: exit 0.
- `git diff --cached --check`: exit 0.
- `git show --check --stat --oneline HEAD`: exit 0 for `fe5be3e fix: address passport vault re-review`.
