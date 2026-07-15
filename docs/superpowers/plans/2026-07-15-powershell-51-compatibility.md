# PowerShell 5.1 Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PassportVault usable from Windows PowerShell 5.1 for AES-CBC-HMAC vaults, without AES-GCM migration support.

**Architecture:** Keep the existing vault format and module layout. Move default protection to AES-CBC-HMAC and add compatibility helpers for .NET Framework runtime gaps.

**Tech Stack:** PowerShell 5.1+, .NET Framework-compatible cryptography APIs, Pester 5+ tests.

## Global Constraints

- Support creating, saving, and loading vaults that use `AES-CBC-HMAC`.
- Keep the existing encrypted JSON envelope format.
- Avoid runtime network downloads and third-party crypto dependencies.
- Do not add migration support for vaults created with `AES-GCM`.
- Do not require PowerShell 5.1 to read `AES-GCM` vaults.

---

### Task 1: Runtime-Compatible Crypto Helpers

**Files:**
- Modify: `tests/PassportVault.Crypto.Tests.ps1`
- Modify: `src/PassportVault.Crypto.psm1`

**Interfaces:**
- Produces: `New-RandomBytes -Length <int>` returns `byte[]`.
- Produces: `Test-FixedTimeEquals -Left <byte[]> -Right <byte[]>` returns `bool`.
- Produces: `New-KeyMaterial -Password <string> -Salt <byte[]> -Iterations <int> [-KeyFilePath <string>]` derives PBKDF2-HMAC-SHA256 keys without requiring .NET Core constructors.

- [ ] **Step 1: Write failing tests**

Add crypto tests that assert default protection selects `AES-CBC-HMAC`, fixed-time comparison returns false for different lengths, and key derivation works without relying on AES-GCM.

- [ ] **Step 2: Run tests to verify failure**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`
Expected: FAIL because the current default is AES-GCM and helper behavior still depends on .NET Core APIs.

- [ ] **Step 3: Implement helpers**

Replace `RandomNumberGenerator]::Fill()` with `RandomNumberGenerator.Create().GetBytes()`, replace `CryptographicOperations.FixedTimeEquals()` with a manual XOR accumulator, and add a PBKDF2-HMAC-SHA256 fallback that works on .NET Framework.

- [ ] **Step 4: Run tests to verify pass**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`
Expected: PASS.

### Task 2: Default CBC-HMAC Behavior and Documentation

**Files:**
- Modify: `src/PassportVault.Crypto.psm1`
- Modify: `README.md`

**Interfaces:**
- Produces: `Protect-VaultPayload -Options @{}` creates an envelope whose `cipher.name` is `AES-CBC-HMAC`.
- Preserves: `Protect-VaultPayload -Options @{ ForceCipher = "AES-GCM" }` for runtimes that support AES-GCM.

- [ ] **Step 1: Write failing default-cipher assertion**

Update the default round-trip test to expect `AES-CBC-HMAC`.

- [ ] **Step 2: Run tests to verify failure**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`
Expected: FAIL before the default cipher change.

- [ ] **Step 3: Change default and docs**

Set the default cipher to `AES-CBC-HMAC`. Update README requirements, examples, key-file generation, and security notes to describe PowerShell 5.1 support and no AES-GCM migration/read support in 5.1.

- [ ] **Step 4: Run full tests**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests -Output Detailed"`
Expected: PASS.
