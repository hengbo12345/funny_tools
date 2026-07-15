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
