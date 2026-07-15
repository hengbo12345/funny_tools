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
    It "round-trips JSON with a non-empty key file" {
        $keyFilePath = Join-Path $TestDrive "vault.key"
        [System.IO.File]::WriteAllBytes($keyFilePath, [byte[]](1, 2, 3, 4))
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ KeyFilePath = $keyFilePath }

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "master" -KeyFilePath $keyFilePath

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.factors.keyFile | Should -BeTrue
    }

    It "rejects an empty key file when creating a vault" {
        $keyFilePath = Join-Path $TestDrive "empty.key"
        [System.IO.File]::WriteAllBytes($keyFilePath, [byte[]]::new(0))

        { Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ KeyFilePath = $keyFilePath } } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "requires the configured key file to unlock a key-file vault" {
        $keyFilePath = Join-Path $TestDrive "vault.key"
        [System.IO.File]::WriteAllBytes($keyFilePath, [byte[]](1, 2, 3, 4))
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ KeyFilePath = $keyFilePath }

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "rejects a different key file when unlocking a key-file vault" {
        $keyFilePath = Join-Path $TestDrive "vault.key"
        $wrongKeyFilePath = Join-Path $TestDrive "wrong.key"
        [System.IO.File]::WriteAllBytes($keyFilePath, [byte[]](1, 2, 3, 4))
        [System.IO.File]::WriteAllBytes($wrongKeyFilePath, [byte[]](5, 6, 7, 8))
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ KeyFilePath = $keyFilePath }

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" -KeyFilePath $wrongKeyFilePath } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "rejects an optional key file for a vault that was not configured with one" {
        $keyFilePath = Join-Path $TestDrive "vault.key"
        [System.IO.File]::WriteAllBytes($keyFilePath, [byte[]](1, 2, 3, 4))
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" -KeyFilePath $keyFilePath } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "round-trips JSON with AES-CBC-HMAC fallback" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ ForceCipher = "AES-CBC-HMAC" }

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "master"

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.cipher.name | Should -Be "AES-CBC-HMAC"
        $envelope.cipher.iv | Should -Not -BeNullOrEmpty
        $envelope.cipher.hmac | Should -Not -BeNullOrEmpty
    }

    It "automatically falls back to AES-CBC-HMAC when AES-GCM is unavailable" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ SimulateAesGcmUnavailable = $true }

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "master"

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.cipher.name | Should -Be "AES-CBC-HMAC"
    }

    It "rejects tampered AES-CBC-HMAC protected metadata" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{ ForceCipher = "AES-CBC-HMAC" }
        $envelope.kdf.iterations = 100000

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "normalizes malformed Base64 and envelope properties to a safe error" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}
        $envelope.kdf.salt = "not-base64"

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Could not unlock vault"

        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}
        $envelope.kdf = @{ name = "PBKDF2-SHA256" }

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "bounds PBKDF2 iterations before attempting to unlock" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}
        $envelope.kdf.iterations = 1000001

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Could not unlock vault"
    }

    It "preserves the unsupported version error for unknown formats" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}
        $envelope.cipher.name = "UnknownCipher"

        { Unprotect-VaultPayload -Envelope $envelope -Password "master" } |
            Should -Throw -ExpectedMessage "Unsupported vault version"
    }
}
