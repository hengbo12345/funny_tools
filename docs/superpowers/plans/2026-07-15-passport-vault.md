# Passport Vault Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Windows PowerShell script that stores passport/password entries in a portable encrypted JSON vault.

**Architecture:** The implementation is one user-facing script plus focused helper modules and Pester tests. The vault stores JSON plaintext only in memory, then writes an encrypted JSON envelope to disk using PBKDF2-derived keys and authenticated encryption.

**Tech Stack:** PowerShell 7+, .NET cryptography APIs, Pester tests, no runtime network dependency downloads.

## Global Constraints

- Do not implement KeePass `.kdbx` compatibility.
- Store the default vault beside the script as `passport-vault.dat`.
- Support `-VaultPath` to override the default vault location.
- Use PBKDF2-HMAC-SHA256 as the default KDF with 600000 iterations.
- Prefer AES-GCM when available.
- Use AES-CBC with HMAC-SHA256 encrypt-then-MAC only as a fallback.
- Support an optional key file as a second unlock factor.
- Keep Windows-user binding out of the first implementation.
- Hide stored passwords by default and require explicit reveal or copy.
- Show non-sensitive error messages for unlock, version, corruption, and save failures.
- Use cryptographically secure randomness for salts, nonces, IVs, and IDs.

---

## File Structure

- Create `PassportVault.ps1`: command-line entrypoint, parameters, unlock/create flow, menu loop.
- Create `src/PassportVault.Crypto.psm1`: KDF, optional key-file mixing, AES-GCM encryption/decryption, AES-CBC/HMAC fallback.
- Create `src/PassportVault.Store.psm1`: vault envelope load/save, protected header canonicalization, atomic writes.
- Create `src/PassportVault.Entries.psm1`: entry creation, search, update, delete, JSON payload shape.
- Create `tests/PassportVault.Crypto.Tests.ps1`: crypto round-trip, wrong password, tampering tests.
- Create `tests/PassportVault.Store.Tests.ps1`: new/load/save/version/header tests.
- Create `tests/PassportVault.Entries.Tests.ps1`: entry operation tests.

---

### Task 1: Entry Model And Operations

**Files:**
- Create: `src/PassportVault.Entries.psm1`
- Test: `tests/PassportVault.Entries.Tests.ps1`

**Interfaces:**
- Produces: `New-VaultPayload() -> hashtable`
- Produces: `New-VaultEntry([string]$Name, [string]$Username, [string]$Password, [string]$Notes) -> hashtable`
- Produces: `Add-VaultEntry([hashtable]$Vault, [hashtable]$Entry) -> hashtable`
- Produces: `Search-VaultEntries([hashtable]$Vault, [string]$Query) -> object[]`
- Produces: `Update-VaultEntry([hashtable]$Vault, [string]$Id, [hashtable]$Changes) -> hashtable`
- Produces: `Remove-VaultEntry([hashtable]$Vault, [string]$Id) -> hashtable`

- [ ] **Step 1: Write failing entry operation tests**

```powershell
Import-Module "$PSScriptRoot/../src/PassportVault.Entries.psm1" -Force

Describe "PassportVault entry operations" {
    It "creates an empty vault payload" {
        $vault = New-VaultPayload
        $vault.schemaVersion | Should -Be 1
        $vault.entries.Count | Should -Be 0
    }

    It "adds and searches entries without searching passwords" {
        $vault = New-VaultPayload
        $entry = New-VaultEntry -Name "example.com" -Username "alice" -Password "secret-pass" -Notes "travel login"

        $vault = Add-VaultEntry -Vault $vault -Entry $entry

        (Search-VaultEntries -Vault $vault -Query "example").Count | Should -Be 1
        (Search-VaultEntries -Vault $vault -Query "alice").Count | Should -Be 1
        (Search-VaultEntries -Vault $vault -Query "travel").Count | Should -Be 1
        (Search-VaultEntries -Vault $vault -Query "secret-pass").Count | Should -Be 0
    }

    It "updates an entry and changes updatedAt" {
        $vault = New-VaultPayload
        $entry = New-VaultEntry -Name "old" -Username "user" -Password "pw" -Notes ""
        $vault = Add-VaultEntry -Vault $vault -Entry $entry
        $originalUpdatedAt = $vault.entries[0].updatedAt
        Start-Sleep -Milliseconds 5

        $vault = Update-VaultEntry -Vault $vault -Id $entry.id -Changes @{ name = "new"; notes = "changed" }

        $vault.entries[0].name | Should -Be "new"
        $vault.entries[0].notes | Should -Be "changed"
        $vault.entries[0].updatedAt | Should -Not -Be $originalUpdatedAt
    }

    It "removes an entry by id" {
        $vault = New-VaultPayload
        $entry = New-VaultEntry -Name "remove-me" -Username "u" -Password "p" -Notes ""
        $vault = Add-VaultEntry -Vault $vault -Entry $entry

        $vault = Remove-VaultEntry -Vault $vault -Id $entry.id

        $vault.entries.Count | Should -Be 0
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Entries.Tests.ps1 -Output Detailed"`

Expected: FAIL because `src/PassportVault.Entries.psm1` or functions do not exist.

- [ ] **Step 3: Implement entry operations**

```powershell
Set-StrictMode -Version Latest

function New-UtcTimestamp {
    return [DateTime]::UtcNow.ToString("o")
}

function New-VaultPayload {
    return @{
        schemaVersion = 1
        entries = @()
    }
}

function New-VaultEntry {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Username,
        [Parameter(Mandatory)][string]$Password,
        [string]$Notes = ""
    )

    $now = New-UtcTimestamp
    return @{
        id = [Guid]::NewGuid().ToString()
        name = $Name
        username = $Username
        password = $Password
        notes = $Notes
        createdAt = $now
        updatedAt = $now
    }
}

function Add-VaultEntry {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][hashtable]$Entry
    )

    $Vault.entries = @($Vault.entries) + @($Entry)
    return $Vault
}

function Search-VaultEntries {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$Query
    )

    $needle = $Query.ToLowerInvariant()
    return @($Vault.entries | Where-Object {
        ($_.name -as [string]).ToLowerInvariant().Contains($needle) -or
        ($_.username -as [string]).ToLowerInvariant().Contains($needle) -or
        ($_.notes -as [string]).ToLowerInvariant().Contains($needle)
    })
}

function Update-VaultEntry {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$Id,
        [Parameter(Mandatory)][hashtable]$Changes
    )

    foreach ($entry in $Vault.entries) {
        if ($entry.id -eq $Id) {
            foreach ($key in @("name", "username", "password", "notes")) {
                if ($Changes.ContainsKey($key)) {
                    $entry[$key] = [string]$Changes[$key]
                }
            }
            $entry.updatedAt = New-UtcTimestamp
            return $Vault
        }
    }
    throw "Entry not found"
}

function Remove-VaultEntry {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$Id
    )

    $Vault.entries = @($Vault.entries | Where-Object { $_.id -ne $Id })
    return $Vault
}

Export-ModuleMember -Function New-VaultPayload, New-VaultEntry, Add-VaultEntry, Search-VaultEntries, Update-VaultEntry, Remove-VaultEntry
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Entries.Tests.ps1 -Output Detailed"`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/PassportVault.Entries.psm1 tests/PassportVault.Entries.Tests.ps1
git commit -m "feat: add vault entry operations"
```

---

### Task 2: Crypto Module

**Files:**
- Create: `src/PassportVault.Crypto.psm1`
- Test: `tests/PassportVault.Crypto.Tests.ps1`

**Interfaces:**
- Consumes: none.
- Produces: `New-RandomBytes([int]$Length) -> byte[]`
- Produces: `New-KeyMaterial([string]$Password, [byte[]]$Salt, [int]$Iterations, [string]$KeyFilePath) -> hashtable`
- Produces: `Protect-VaultPayload([string]$PlainJson, [string]$Password, [hashtable]$Options) -> hashtable`
- Produces: `Unprotect-VaultPayload([hashtable]$Envelope, [string]$Password, [string]$KeyFilePath) -> string`

- [ ] **Step 1: Write failing crypto tests**

```powershell
Import-Module "$PSScriptRoot/../src/PassportVault.Crypto.psm1" -Force

Describe "PassportVault crypto" {
    It "round-trips JSON with AES-GCM" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "correct horse battery staple" -Options @{}

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "correct horse battery staple"

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.format | Should -Be "PassportVault"
        $envelope.kdf.name | Should -Be "PBKDF2-SHA256"
    }

    It "rejects the wrong password" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "right" -Options @{}

        { Unprotect-VaultPayload -Envelope $envelope -Password "wrong" } | Should -Throw
    }

    It "rejects tampered ciphertext" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "right" -Options @{}
        $bytes = [Convert]::FromBase64String($envelope.ciphertext)
        $bytes[0] = $bytes[0] -bxor 1
        $envelope.ciphertext = [Convert]::ToBase64String($bytes)

        { Unprotect-VaultPayload -Envelope $envelope -Password "right" } | Should -Throw
    }

    It "rejects tampered protected metadata" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "right" -Options @{}
        $envelope.kdf.iterations = 10

        { Unprotect-VaultPayload -Envelope $envelope -Password "right" } | Should -Throw
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`

Expected: FAIL because crypto functions do not exist.

- [ ] **Step 3: Implement AES-GCM crypto**

```powershell
Set-StrictMode -Version Latest

function New-RandomBytes {
    param([Parameter(Mandatory)][int]$Length)
    $bytes = [byte[]]::new($Length)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return $bytes
}

function Get-Bytes {
    param([Parameter(Mandatory)][string]$Value)
    return [System.Text.Encoding]::UTF8.GetBytes($Value)
}

function ConvertTo-Base64 {
    param([Parameter(Mandatory)][byte[]]$Bytes)
    return [Convert]::ToBase64String($Bytes)
}

function ConvertFrom-Base64 {
    param([Parameter(Mandatory)][string]$Value)
    return [Convert]::FromBase64String($Value)
}

function Get-KeyFileBytes {
    param([string]$KeyFilePath)
    if ([string]::IsNullOrWhiteSpace($KeyFilePath)) {
        return [byte[]]::new(0)
    }
    if (-not (Test-Path -LiteralPath $KeyFilePath)) {
        throw "Could not unlock vault"
    }
    return [System.IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $KeyFilePath))
}

function New-KeyMaterial {
    param(
        [Parameter(Mandatory)][string]$Password,
        [Parameter(Mandatory)][byte[]]$Salt,
        [Parameter(Mandatory)][int]$Iterations,
        [string]$KeyFilePath
    )

    $passwordBytes = Get-Bytes -Value $Password
    $keyFileBytes = Get-KeyFileBytes -KeyFilePath $KeyFilePath
    $combined = [byte[]]::new($passwordBytes.Length + $keyFileBytes.Length)
    [Buffer]::BlockCopy($passwordBytes, 0, $combined, 0, $passwordBytes.Length)
    if ($keyFileBytes.Length -gt 0) {
        [Buffer]::BlockCopy($keyFileBytes, 0, $combined, $passwordBytes.Length, $keyFileBytes.Length)
    }

    $kdf = [System.Security.Cryptography.Rfc2898DeriveBytes]::new($combined, $Salt, $Iterations, [System.Security.Cryptography.HashAlgorithmName]::SHA256)
    $key = $kdf.GetBytes(32)
    $macKey = $kdf.GetBytes(32)
    $kdf.Dispose()
    [Array]::Clear($combined, 0, $combined.Length)
    [Array]::Clear($passwordBytes, 0, $passwordBytes.Length)

    return @{
        encryptionKey = $key
        macKey = $macKey
    }
}

function ConvertTo-ProtectedHeaderJson {
    param([Parameter(Mandatory)][hashtable]$Envelope)
    $header = [ordered]@{
        format = $Envelope.format
        version = $Envelope.version
        kdf = [ordered]@{
            name = $Envelope.kdf.name
            iterations = $Envelope.kdf.iterations
            salt = $Envelope.kdf.salt
        }
        cipher = [ordered]@{
            name = $Envelope.cipher.name
            nonce = $Envelope.cipher.nonce
        }
        factors = [ordered]@{
            keyFile = [bool]$Envelope.factors.keyFile
            windowsUserBinding = [bool]$Envelope.factors.windowsUserBinding
        }
    }
    return ($header | ConvertTo-Json -Depth 8 -Compress)
}

function Protect-VaultPayload {
    param(
        [Parameter(Mandatory)][string]$PlainJson,
        [Parameter(Mandatory)][string]$Password,
        [hashtable]$Options = @{}
    )

    $iterations = if ($Options.ContainsKey("Iterations")) { [int]$Options.Iterations } else { 600000 }
    $keyFilePath = if ($Options.ContainsKey("KeyFilePath")) { [string]$Options.KeyFilePath } else { $null }
    $salt = New-RandomBytes -Length 32
    $nonce = New-RandomBytes -Length 12
    $keys = New-KeyMaterial -Password $Password -Salt $salt -Iterations $iterations -KeyFilePath $keyFilePath
    $plainBytes = Get-Bytes -Value $PlainJson
    $cipherBytes = [byte[]]::new($plainBytes.Length)
    $tag = [byte[]]::new(16)

    $envelope = [ordered]@{
        format = "PassportVault"
        version = 1
        kdf = [ordered]@{ name = "PBKDF2-SHA256"; iterations = $iterations; salt = ConvertTo-Base64 -Bytes $salt }
        cipher = [ordered]@{ name = "AES-GCM"; nonce = ConvertTo-Base64 -Bytes $nonce; tag = "" }
        factors = [ordered]@{ keyFile = -not [string]::IsNullOrWhiteSpace($keyFilePath); windowsUserBinding = $false }
        ciphertext = ""
    }

    $aad = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $envelope)
    $aes = [System.Security.Cryptography.AesGcm]::new($keys.encryptionKey)
    $aes.Encrypt($nonce, $plainBytes, $cipherBytes, $tag, $aad)
    $aes.Dispose()

    $envelope.cipher.tag = ConvertTo-Base64 -Bytes $tag
    $envelope.ciphertext = ConvertTo-Base64 -Bytes $cipherBytes
    [Array]::Clear($plainBytes, 0, $plainBytes.Length)
    return $envelope
}

function Unprotect-VaultPayload {
    param(
        [Parameter(Mandatory)][hashtable]$Envelope,
        [Parameter(Mandatory)][string]$Password,
        [string]$KeyFilePath
    )

    if ($Envelope.format -ne "PassportVault" -or [int]$Envelope.version -ne 1) {
        throw "Unsupported vault version"
    }
    if ($Envelope.kdf.name -ne "PBKDF2-SHA256" -or $Envelope.cipher.name -ne "AES-GCM") {
        throw "Unsupported vault version"
    }

    $salt = ConvertFrom-Base64 -Value $Envelope.kdf.salt
    $nonce = ConvertFrom-Base64 -Value $Envelope.cipher.nonce
    $tag = ConvertFrom-Base64 -Value $Envelope.cipher.tag
    $cipherBytes = ConvertFrom-Base64 -Value $Envelope.ciphertext
    $keys = New-KeyMaterial -Password $Password -Salt $salt -Iterations ([int]$Envelope.kdf.iterations) -KeyFilePath $KeyFilePath
    $plainBytes = [byte[]]::new($cipherBytes.Length)
    $aad = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $Envelope)

    try {
        $aes = [System.Security.Cryptography.AesGcm]::new($keys.encryptionKey)
        $aes.Decrypt($nonce, $cipherBytes, $tag, $plainBytes, $aad)
        $aes.Dispose()
        return [System.Text.Encoding]::UTF8.GetString($plainBytes)
    } catch {
        throw "Could not unlock vault"
    } finally {
        if ($plainBytes) { [Array]::Clear($plainBytes, 0, $plainBytes.Length) }
    }
}

Export-ModuleMember -Function New-RandomBytes, New-KeyMaterial, Protect-VaultPayload, Unprotect-VaultPayload, ConvertTo-ProtectedHeaderJson
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`

Expected: PASS on PowerShell 7+ with AES-GCM support.

- [ ] **Step 5: Commit**

```bash
git add src/PassportVault.Crypto.psm1 tests/PassportVault.Crypto.Tests.ps1
git commit -m "feat: add vault encryption"
```

---


### Task 3: AES-CBC/HMAC Fallback

**Files:**
- Modify: `src/PassportVault.Crypto.psm1`
- Modify: `tests/PassportVault.Crypto.Tests.ps1`

**Interfaces:**
- Consumes: `New-RandomBytes`, `New-KeyMaterial`, `ConvertTo-ProtectedHeaderJson`
- Produces: fallback support inside `Protect-VaultPayload` when `Options.ForceCipher` is `"AES-CBC-HMAC"`
- Produces: fallback support inside `Unprotect-VaultPayload` when `Envelope.cipher.name` is `"AES-CBC-HMAC"`

- [ ] **Step 1: Add failing fallback tests**

```powershell
It "round-trips JSON with AES-CBC-HMAC fallback" {
    $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ ForceCipher = "AES-CBC-HMAC" }

    $plain = Unprotect-VaultPayload -Envelope $envelope -Password "master"

    $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
    $envelope.cipher.name | Should -Be "AES-CBC-HMAC"
    $envelope.cipher.iv | Should -Not -BeNullOrEmpty
    $envelope.cipher.hmac | Should -Not -BeNullOrEmpty
}

It "rejects tampered AES-CBC-HMAC protected metadata" {
    $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ ForceCipher = "AES-CBC-HMAC" }
    $envelope.kdf.iterations = 10

    { Unprotect-VaultPayload -Envelope $envelope -Password "master" } | Should -Throw
}
```

- [ ] **Step 2: Run tests to verify fallback tests fail**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`

Expected: FAIL because `ForceCipher = "AES-CBC-HMAC"` is not implemented.

- [ ] **Step 3: Add fallback helper functions**

```powershell
function Test-FixedTimeEquals {
    param(
        [Parameter(Mandatory)][byte[]]$Left,
        [Parameter(Mandatory)][byte[]]$Right
    )
    return [System.Security.Cryptography.CryptographicOperations]::FixedTimeEquals($Left, $Right)
}

function New-HmacSha256 {
    param(
        [Parameter(Mandatory)][byte[]]$Key,
        [Parameter(Mandatory)][byte[]]$Data
    )
    $hmac = [System.Security.Cryptography.HMACSHA256]::new($Key)
    try {
        return $hmac.ComputeHash($Data)
    } finally {
        $hmac.Dispose()
    }
}

function Join-Bytes {
    param([Parameter(Mandatory)][byte[][]]$Parts)
    $length = ($Parts | Measure-Object -Property Length -Sum).Sum
    $result = [byte[]]::new($length)
    $offset = 0
    foreach ($part in $Parts) {
        [Buffer]::BlockCopy($part, 0, $result, $offset, $part.Length)
        $offset += $part.Length
    }
    return $result
}
```

- [ ] **Step 4: Extend `Protect-VaultPayload` for fallback**

Add this branch before the AES-GCM encryption block:

```powershell
if ($Options.ContainsKey("ForceCipher") -and $Options.ForceCipher -eq "AES-CBC-HMAC") {
    $iv = New-RandomBytes -Length 16
    $envelope = [ordered]@{
        format = "PassportVault"
        version = 1
        kdf = [ordered]@{ name = "PBKDF2-SHA256"; iterations = $iterations; salt = ConvertTo-Base64 -Bytes $salt }
        cipher = [ordered]@{ name = "AES-CBC-HMAC"; iv = ConvertTo-Base64 -Bytes $iv; hmac = "" }
        factors = [ordered]@{ keyFile = -not [string]::IsNullOrWhiteSpace($keyFilePath); windowsUserBinding = $false }
        ciphertext = ""
    }
    $aes = [System.Security.Cryptography.Aes]::Create()
    $aes.Mode = [System.Security.Cryptography.CipherMode]::CBC
    $aes.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
    $aes.Key = $keys.encryptionKey
    $aes.IV = $iv
    $encryptor = $aes.CreateEncryptor()
    $cipherBytes = $encryptor.TransformFinalBlock($plainBytes, 0, $plainBytes.Length)
    $encryptor.Dispose()
    $aes.Dispose()

    $headerBytes = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $envelope)
    $macInput = Join-Bytes -Parts @($headerBytes, $iv, $cipherBytes)
    $mac = New-HmacSha256 -Key $keys.macKey -Data $macInput
    $envelope.cipher.hmac = ConvertTo-Base64 -Bytes $mac
    $envelope.ciphertext = ConvertTo-Base64 -Bytes $cipherBytes
    [Array]::Clear($plainBytes, 0, $plainBytes.Length)
    return $envelope
}
```

- [ ] **Step 5: Extend `ConvertTo-ProtectedHeaderJson` and `Unprotect-VaultPayload` for fallback**

In `ConvertTo-ProtectedHeaderJson`, create a cipher header before `$header` and use it in the protected header:

```powershell
$cipherHeader = if ($Envelope.cipher.name -eq "AES-CBC-HMAC") {
    [ordered]@{ name = $Envelope.cipher.name; iv = $Envelope.cipher.iv }
} else {
    [ordered]@{ name = $Envelope.cipher.name; nonce = $Envelope.cipher.nonce }
}

$header = [ordered]@{
    format = $Envelope.format
    version = $Envelope.version
    kdf = [ordered]@{
        name = $Envelope.kdf.name
        iterations = $Envelope.kdf.iterations
        salt = $Envelope.kdf.salt
    }
    cipher = $cipherHeader
    factors = [ordered]@{
        keyFile = [bool]$Envelope.factors.keyFile
        windowsUserBinding = [bool]$Envelope.factors.windowsUserBinding
    }
}
```

In `Unprotect-VaultPayload`, replace the cipher validation with this version:

```powershell
if ($Envelope.kdf.name -ne "PBKDF2-SHA256") {
    throw "Unsupported vault version"
}
if ($Envelope.cipher.name -notin @("AES-GCM", "AES-CBC-HMAC")) {
    throw "Unsupported vault version"
}
```

Then add this branch in `Unprotect-VaultPayload` after key derivation and before AES-GCM handling:

```powershell
if ($Envelope.cipher.name -eq "AES-CBC-HMAC") {
    $iv = ConvertFrom-Base64 -Value $Envelope.cipher.iv
    $cipherBytes = ConvertFrom-Base64 -Value $Envelope.ciphertext
    $expectedMac = ConvertFrom-Base64 -Value $Envelope.cipher.hmac
    $headerBytes = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $Envelope)
    $macInput = Join-Bytes -Parts @($headerBytes, $iv, $cipherBytes)
    $actualMac = New-HmacSha256 -Key $keys.macKey -Data $macInput
    if (-not (Test-FixedTimeEquals -Left $expectedMac -Right $actualMac)) {
        throw "Could not unlock vault"
    }

    try {
        $aes = [System.Security.Cryptography.Aes]::Create()
        $aes.Mode = [System.Security.Cryptography.CipherMode]::CBC
        $aes.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
        $aes.Key = $keys.encryptionKey
        $aes.IV = $iv
        $decryptor = $aes.CreateDecryptor()
        $plainBytes = $decryptor.TransformFinalBlock($cipherBytes, 0, $cipherBytes.Length)
        $decryptor.Dispose()
        $aes.Dispose()
        return [System.Text.Encoding]::UTF8.GetString($plainBytes)
    } catch {
        throw "Could not unlock vault"
    } finally {
        if ($plainBytes) { [Array]::Clear($plainBytes, 0, $plainBytes.Length) }
    }
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Crypto.Tests.ps1 -Output Detailed"`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/PassportVault.Crypto.psm1 tests/PassportVault.Crypto.Tests.ps1
git commit -m "feat: add fallback vault cipher"
```

---

### Task 4: Vault Store Load And Save

**Files:**
- Create: `src/PassportVault.Store.psm1`
- Test: `tests/PassportVault.Store.Tests.ps1`

**Interfaces:**
- Consumes: `Protect-VaultPayload`, `Unprotect-VaultPayload`, `New-VaultPayload`
- Produces: `Save-Vault([hashtable]$Vault, [string]$VaultPath, [string]$Password, [string]$KeyFilePath) -> void`
- Produces: `Load-Vault([string]$VaultPath, [string]$Password, [string]$KeyFilePath) -> hashtable`
- Produces: `Test-VaultExists([string]$VaultPath) -> bool`

- [ ] **Step 1: Write failing store tests**

```powershell
Import-Module "$PSScriptRoot/../src/PassportVault.Crypto.psm1" -Force
Import-Module "$PSScriptRoot/../src/PassportVault.Entries.psm1" -Force
Import-Module "$PSScriptRoot/../src/PassportVault.Store.psm1" -Force

Describe "PassportVault store" {
    BeforeEach {
        $script:path = Join-Path $TestDrive "passport-vault.dat"
    }

    It "saves and loads a vault" {
        $vault = New-VaultPayload
        $vault = Add-VaultEntry -Vault $vault -Entry (New-VaultEntry -Name "site" -Username "user" -Password "pass" -Notes "")

        Save-Vault -Vault $vault -VaultPath $script:path -Password "master"
        $loaded = Load-Vault -VaultPath $script:path -Password "master"

        $loaded.schemaVersion | Should -Be 1
        $loaded.entries[0].name | Should -Be "site"
    }

    It "reports whether the vault exists" {
        Test-VaultExists -VaultPath $script:path | Should -BeFalse
        Save-Vault -Vault (New-VaultPayload) -VaultPath $script:path -Password "master"
        Test-VaultExists -VaultPath $script:path | Should -BeTrue
    }

    It "rejects unsupported vault schema after decrypting" {
        $vault = New-VaultPayload
        $vault.schemaVersion = 99
        Save-Vault -Vault $vault -VaultPath $script:path -Password "master"

        { Load-Vault -VaultPath $script:path -Password "master" } | Should -Throw
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Store.Tests.ps1 -Output Detailed"`

Expected: FAIL because store functions do not exist.

- [ ] **Step 3: Implement store module**

```powershell
Set-StrictMode -Version Latest

Import-Module "$PSScriptRoot/PassportVault.Crypto.psm1" -Force
Import-Module "$PSScriptRoot/PassportVault.Entries.psm1" -Force

function Test-VaultExists {
    param([Parameter(Mandatory)][string]$VaultPath)
    return [System.IO.File]::Exists($VaultPath)
}

function ConvertTo-Hashtable {
    param([Parameter(Mandatory)]$InputObject)
    if ($null -eq $InputObject) { return $null }
    if ($InputObject -is [System.Collections.IDictionary]) {
        $hash = @{}
        foreach ($key in $InputObject.Keys) {
            $hash[$key] = ConvertTo-Hashtable -InputObject $InputObject[$key]
        }
        return $hash
    }
    if ($InputObject -is [System.Collections.IEnumerable] -and $InputObject -isnot [string]) {
        return @($InputObject | ForEach-Object { ConvertTo-Hashtable -InputObject $_ })
    }
    if ($InputObject -is [pscustomobject]) {
        $hash = @{}
        foreach ($prop in $InputObject.PSObject.Properties) {
            $hash[$prop.Name] = ConvertTo-Hashtable -InputObject $prop.Value
        }
        return $hash
    }
    return $InputObject
}

function Save-Vault {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$VaultPath,
        [Parameter(Mandatory)][string]$Password,
        [string]$KeyFilePath
    )

    $plainJson = $Vault | ConvertTo-Json -Depth 20 -Compress
    $envelope = Protect-VaultPayload -PlainJson $plainJson -Password $Password -Options @{ KeyFilePath = $KeyFilePath }
    $envelopeJson = $envelope | ConvertTo-Json -Depth 20
    $directory = Split-Path -Parent $VaultPath
    if (-not [string]::IsNullOrWhiteSpace($directory)) {
        New-Item -ItemType Directory -Force -Path $directory | Out-Null
    }
    $tempPath = "$VaultPath.tmp"
    try {
        [System.IO.File]::WriteAllText($tempPath, $envelopeJson, [System.Text.Encoding]::UTF8)
        Move-Item -LiteralPath $tempPath -Destination $VaultPath -Force
    } catch {
        if (Test-Path -LiteralPath $tempPath) { Remove-Item -LiteralPath $tempPath -Force }
        throw "Could not save vault"
    }
}

function Load-Vault {
    param(
        [Parameter(Mandatory)][string]$VaultPath,
        [Parameter(Mandatory)][string]$Password,
        [string]$KeyFilePath
    )

    if (-not (Test-VaultExists -VaultPath $VaultPath)) {
        throw "Vault file not found"
    }
    try {
        $envelope = ConvertTo-Hashtable -InputObject ([System.IO.File]::ReadAllText($VaultPath) | ConvertFrom-Json)
        $plainJson = Unprotect-VaultPayload -Envelope $envelope -Password $Password -KeyFilePath $KeyFilePath
        $vault = ConvertTo-Hashtable -InputObject ($plainJson | ConvertFrom-Json)
        if ([int]$vault.schemaVersion -ne 1) {
            throw "Unsupported vault version"
        }
        $vault.entries = @($vault.entries)
        return $vault
    } catch {
        if ($_.Exception.Message -eq "Vault file not found" -or $_.Exception.Message -eq "Unsupported vault version") {
            throw
        }
        throw "Could not unlock vault"
    }
}

Export-ModuleMember -Function Save-Vault, Load-Vault, Test-VaultExists, ConvertTo-Hashtable
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests/PassportVault.Store.Tests.ps1 -Output Detailed"`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/PassportVault.Store.psm1 tests/PassportVault.Store.Tests.ps1
git commit -m "feat: add vault persistence"
```

---

### Task 5: Interactive Script

**Files:**
- Create: `PassportVault.ps1`

**Interfaces:**
- Consumes: all module functions from Tasks 1-4.
- Produces: runnable CLI with `-VaultPath` and `-KeyFilePath`.

- [ ] **Step 1: Create the script entrypoint**

```powershell
param(
    [string]$VaultPath = (Join-Path $PSScriptRoot "passport-vault.dat"),
    [string]$KeyFilePath
)

Set-StrictMode -Version Latest

Import-Module "$PSScriptRoot/src/PassportVault.Entries.psm1" -Force
Import-Module "$PSScriptRoot/src/PassportVault.Crypto.psm1" -Force
Import-Module "$PSScriptRoot/src/PassportVault.Store.psm1" -Force

function Read-EntryPassword {
    param([string]$Prompt = "Entry password")
    $secure = Read-Host -Prompt $Prompt -AsSecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
}

function Read-RequiredValue {
    param([Parameter(Mandatory)][string]$Prompt)
    do {
        $value = Read-Host -Prompt $Prompt
    } while ([string]::IsNullOrWhiteSpace($value))
    return $value
}

function Select-Entry {
    param([Parameter(Mandatory)][hashtable]$Vault)
    if ($Vault.entries.Count -eq 0) {
        Write-Host "No entries."
        return $null
    }
    for ($i = 0; $i -lt $Vault.entries.Count; $i++) {
        Write-Host ("{0}. {1} ({2})" -f ($i + 1), $Vault.entries[$i].name, $Vault.entries[$i].username)
    }
    $choice = Read-Host "Choose entry number"
    $index = 0
    if ([int]::TryParse($choice, [ref]$index) -and $index -ge 1 -and $index -le $Vault.entries.Count) {
        return $Vault.entries[$index - 1]
    }
    Write-Host "Invalid choice."
    return $null
}

function Show-EntryDetails {
    param([Parameter(Mandatory)][hashtable]$Entry)
    Write-Host "Name: $($Entry.name)"
    Write-Host "Username: $($Entry.username)"
    Write-Host "Password: ********"
    Write-Host "Notes: $($Entry.notes)"
    $action = Read-Host "Reveal password? (y/N)"
    if ($action -eq "y") {
        Write-Host "Password: $($Entry.password)"
    }
}

function Add-EntryInteractive {
    param([Parameter(Mandatory)][hashtable]$Vault)
    $name = Read-RequiredValue -Prompt "Name"
    $username = Read-Host "Username"
    $password = Read-EntryPassword -Prompt "Item password"
    $notes = Read-Host "Notes"
    return Add-VaultEntry -Vault $Vault -Entry (New-VaultEntry -Name $name -Username $username -Password $password -Notes $notes)
}

function Search-EntriesInteractive {
    param([Parameter(Mandatory)][hashtable]$Vault)
    $query = Read-Host "Search"
    $results = @(Search-VaultEntries -Vault $Vault -Query $query)
    if ($results.Count -eq 0) {
        Write-Host "No matches."
        return
    }
    for ($i = 0; $i -lt $results.Count; $i++) {
        Write-Host ("{0}. {1} ({2})" -f ($i + 1), $results[$i].name, $results[$i].username)
    }
}

function Edit-EntryInteractive {
    param([Parameter(Mandatory)][hashtable]$Vault)
    $entry = Select-Entry -Vault $Vault
    if ($null -eq $entry) { return $Vault }
    $name = Read-Host "Name [$($entry.name)]"
    $username = Read-Host "Username [$($entry.username)]"
    $changePassword = Read-Host "Change password? (y/N)"
    $notes = Read-Host "Notes [$($entry.notes)]"
    $changes = @{}
    if (-not [string]::IsNullOrWhiteSpace($name)) { $changes.name = $name }
    if (-not [string]::IsNullOrWhiteSpace($username)) { $changes.username = $username }
    if ($changePassword -eq "y") { $changes.password = Read-EntryPassword -Prompt "New item password" }
    if (-not [string]::IsNullOrWhiteSpace($notes)) { $changes.notes = $notes }
    if ($changes.Count -eq 0) { return $Vault }
    return Update-VaultEntry -Vault $Vault -Id $entry.id -Changes $changes
}

function Show-MainMenu {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$Password
    )
    while ($true) {
        Write-Host ""
        Write-Host "1. List entries"
        Write-Host "2. Search entries"
        Write-Host "3. Add entry"
        Write-Host "4. View entry"
        Write-Host "5. Modify entry"
        Write-Host "6. Delete entry"
        Write-Host "7. Change entry password"
        Write-Host "8. Exit"
        $choice = Read-Host "Choose"
        switch ($choice) {
            "1" { $Vault.entries | ForEach-Object { Write-Host "$($_.name) ($($_.username))" } }
            "2" { Search-EntriesInteractive -Vault $Vault }
            "3" { $Vault = Add-EntryInteractive -Vault $Vault; Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $Password -KeyFilePath $KeyFilePath }
            "4" { $entry = Select-Entry -Vault $Vault; if ($entry) { Show-EntryDetails -Entry $entry } }
            "5" { $Vault = Edit-EntryInteractive -Vault $Vault; Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $Password -KeyFilePath $KeyFilePath }
            "6" {
                $entry = Select-Entry -Vault $Vault
                if ($entry -and (Read-Host "Delete $($entry.name)? (y/N)") -eq "y") {
                    $Vault = Remove-VaultEntry -Vault $Vault -Id $entry.id
                    Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $Password -KeyFilePath $KeyFilePath
                }
            }
            "7" {
                $newPassword = Read-EntryPassword -Prompt "New entry password"
                $confirm = Read-EntryPassword -Prompt "Confirm new entry password"
                if ($newPassword -ne $confirm) { Write-Host "Passwords do not match."; break }
                Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $newPassword -KeyFilePath $KeyFilePath
                $Password = $newPassword
            }
            "8" { return }
            default { Write-Host "Invalid choice." }
        }
    }
}

try {
    $password = Read-EntryPassword
    if (Test-VaultExists -VaultPath $VaultPath) {
        $vault = Load-Vault -VaultPath $VaultPath -Password $password -KeyFilePath $KeyFilePath
    } else {
        $confirm = Read-EntryPassword -Prompt "Confirm entry password"
        if ($password -ne $confirm) { throw "Passwords do not match." }
        $vault = New-VaultPayload
        Save-Vault -Vault $vault -VaultPath $VaultPath -Password $password -KeyFilePath $KeyFilePath
    }
    Show-MainMenu -Vault $vault -Password $password
} catch {
    Write-Host $_.Exception.Message
    exit 1
}
```

- [ ] **Step 2: Run parser check**

Run: `pwsh -NoProfile -Command '$null = [System.Management.Automation.PSParser]::Tokenize((Get-Content -Raw PassportVault.ps1), [ref]$null); "parsed"'`

Expected: prints `parsed`.

- [ ] **Step 3: Commit**

```bash
git add PassportVault.ps1
git commit -m "feat: add interactive passport vault script"
```

---

### Task 6: Full Verification And Documentation

**Files:**
- Create: `README.md`
- Modify: `PassportVault.ps1`
- Modify: module files only if verification finds defects.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: documented usage and verified working script.

- [ ] **Step 1: Write README**

````markdown
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
````

- [ ] **Step 2: Run all automated tests**

Run: `pwsh -NoProfile -Command "Invoke-Pester -Path tests -Output Detailed"`

Expected: PASS.

- [ ] **Step 3: Run manual smoke test**

Run: `pwsh ./PassportVault.ps1 -VaultPath ./tmp-smoke-vault.dat`

Manual expected result:
- Create a new vault with a test entry password.
- Add entry `example.com`.
- Search for `example`.
- View entry and reveal password only after choosing reveal.
- Exit.
- Reopen with the same password and confirm entry exists.
- Reopen with a wrong password and confirm unlock fails.

- [ ] **Step 4: Remove smoke-test vault**

Run: `Remove-Item ./tmp-smoke-vault.dat -Force`

Expected: file is removed.

- [ ] **Step 5: Commit**

```bash
git add README.md PassportVault.ps1 src tests
git commit -m "docs: document passport vault usage"
```
