Import-Module "$PSScriptRoot/../src/PassportVault.Crypto.psm1" -Force

$script:AesGcmSupported = $false
$aesGcmProbe = $null
try {
    $aesGcmProbe = [System.Security.Cryptography.AesGcm]::new([byte[]]::new(32))
    $script:AesGcmSupported = $true
} catch [System.PlatformNotSupportedException] {
    $script:AesGcmSupported = $false
} finally {
    if ($null -ne $aesGcmProbe) { $aesGcmProbe.Dispose() }
}

Describe "PassportVault crypto" {
    It "round-trips JSON and selects AES-GCM when supported" {
        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "correct horse battery staple" -Options @{}

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "correct horse battery staple"

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.format | Should -Be "PassportVault"
        $envelope.kdf.name | Should -Be "PBKDF2-SHA256"
        if ($script:AesGcmSupported) {
            $envelope.cipher.name | Should -Be "AES-GCM"
        }
    }

    It "returns byte arrays as single objects from byte-array helpers" {
        $random = New-RandomBytes -Length 16
        $emptyRandom = New-RandomBytes -Length 0

        $random.GetType().FullName | Should -Be "System.Byte[]"
        $random.Length | Should -Be 16
        $emptyRandom.GetType().FullName | Should -Be "System.Byte[]"
        $emptyRandom.Length | Should -Be 0

        InModuleScope PassportVault.Crypto {
            $utf8 = Get-Bytes -Value "abc"
            $decoded = ConvertFrom-Base64 -Value "AQI="
            $emptyDecoded = ConvertFrom-Base64 -Value ""
            $keyFile = Get-KeyFileBytes
            $required = Get-RequiredBase64Bytes -Value "AwQ="
            $digest = New-HmacSha256 -Key ([byte[]](1, 2, 3)) -Data ([byte[]](4, 5, 6))
            $sha256 = Get-Sha256Digest -Bytes ([byte[]](1, 2, 3))
            $length = ConvertTo-BigEndianUInt32Bytes -Value 3
            $frame = New-FactorFrame -PasswordBytes ([byte[]](1, 2, 3)) -KeyFileBytes ([byte[]](4, 5, 6))
            $joined = Join-Bytes -Parts ([byte[][]]@([byte[]](1), [byte[]](2)))

            $utf8.GetType().FullName | Should -Be "System.Byte[]"
            $decoded.GetType().FullName | Should -Be "System.Byte[]"
            $emptyDecoded.GetType().FullName | Should -Be "System.Byte[]"
            $keyFile.GetType().FullName | Should -Be "System.Byte[]"
            $required.GetType().FullName | Should -Be "System.Byte[]"
            $digest.GetType().FullName | Should -Be "System.Byte[]"
            $sha256.GetType().FullName | Should -Be "System.Byte[]"
            $length.GetType().FullName | Should -Be "System.Byte[]"
            $frame.GetType().FullName | Should -Be "System.Byte[]"
            $joined.GetType().FullName | Should -Be "System.Byte[]"
            $emptyDecoded.Length | Should -Be 0
            $keyFile.Length | Should -Be 0
        }
    }

    It "returns encryption and MAC keys as byte arrays" {
        $keys = New-KeyMaterial -Password "master" -Salt ([byte[]]::new(32)) -Iterations 100000

        $keys.encryptionKey.GetType().FullName | Should -Be "System.Byte[]"
        $keys.macKey.GetType().FullName | Should -Be "System.Byte[]"
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

    It "frames password and key-file factors without boundary collisions" {
        $leftKeyFilePath = Join-Path $TestDrive "left.key"
        $rightKeyFilePath = Join-Path $TestDrive "right.key"
        [System.IO.File]::WriteAllBytes($leftKeyFilePath, [System.Text.Encoding]::UTF8.GetBytes("bc"))
        [System.IO.File]::WriteAllBytes($rightKeyFilePath, [System.Text.Encoding]::UTF8.GetBytes("c"))
        $salt = [byte[]]::new(32)

        $left = New-KeyMaterial -Password "a" -Salt $salt -Iterations 100000 -KeyFilePath $leftKeyFilePath
        $right = New-KeyMaterial -Password "ab" -Salt $salt -Iterations 100000 -KeyFilePath $rightKeyFilePath

        [Convert]::ToBase64String($left.encryptionKey) |
            Should -Not -Be ([Convert]::ToBase64String($right.encryptionKey))
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
        Mock -CommandName New-AesGcm -ModuleName PassportVault.Crypto -MockWith {
            throw [System.PlatformNotSupportedException]::new("AES-GCM unavailable")
        }

        $envelope = Protect-VaultPayload -PlainJson '{"schemaVersion":1,"entries":[]}' -Password "master" -Options @{}

        $plain = Unprotect-VaultPayload -Envelope $envelope -Password "master"

        $plain | Should -Be '{"schemaVersion":1,"entries":[]}'
        $envelope.cipher.name | Should -Be "AES-CBC-HMAC"
        Should -Invoke -CommandName New-AesGcm -ModuleName PassportVault.Crypto -Times 1 -Exactly
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
