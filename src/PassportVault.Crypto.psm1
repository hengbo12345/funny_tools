Set-StrictMode -Version Latest

# PBKDF2 limits bound attacker-controlled work while retaining the 600000 iteration default.
$script:DefaultPbkdf2Iterations = 600000
$script:MinimumPbkdf2Iterations = 100000
$script:MaximumPbkdf2Iterations = 1000000
$script:CouldNotUnlockVault = "Could not unlock vault"
$script:UnsupportedVaultVersion = "Unsupported vault version"

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

function Clear-Bytes {
    param([byte[]]$Bytes)
    if ($null -ne $Bytes -and $Bytes.Length -gt 0) {
        [Array]::Clear($Bytes, 0, $Bytes.Length)
    }
}

function Get-KeyFileBytes {
    param([string]$KeyFilePath)
    if ([string]::IsNullOrWhiteSpace($KeyFilePath)) {
        return [byte[]]::new(0)
    }
    try {
        $resolvedPath = Resolve-Path -LiteralPath $KeyFilePath -ErrorAction Stop
        $bytes = [System.IO.File]::ReadAllBytes($resolvedPath.Path)
        if ($bytes.Length -eq 0) {
            throw $script:CouldNotUnlockVault
        }
        return $bytes
    } catch {
        throw $script:CouldNotUnlockVault
    }
}

function Get-ValidatedPbkdf2Iterations {
    param([Parameter(Mandatory)][object]$Value)
    $iterations = 0
    if (-not [int]::TryParse([string]$Value, [ref]$iterations)) {
        throw $script:CouldNotUnlockVault
    }
    if ($iterations -lt $script:MinimumPbkdf2Iterations -or $iterations -gt $script:MaximumPbkdf2Iterations) {
        throw $script:CouldNotUnlockVault
    }
    return $iterations
}

function Get-RequiredBase64Bytes {
    param([Parameter(Mandatory)][object]$Value)
    if ($Value -isnot [string]) {
        throw $script:CouldNotUnlockVault
    }
    return ConvertFrom-Base64 -Value $Value
}

function Assert-ByteLength {
    param([Parameter(Mandatory)][byte[]]$Bytes, [Parameter(Mandatory)][int]$ExpectedLength)
    if ($Bytes.Length -ne $ExpectedLength) {
        throw $script:CouldNotUnlockVault
    }
}

function New-KeyMaterial {
    param(
        [Parameter(Mandatory)][string]$Password,
        [Parameter(Mandatory)][byte[]]$Salt,
        [Parameter(Mandatory)][int]$Iterations,
        [string]$KeyFilePath
    )
    $passwordBytes = $null
    $keyFileBytes = $null
    $combined = $null
    $kdf = $null
    try {
        $validatedIterations = Get-ValidatedPbkdf2Iterations -Value $Iterations
        $passwordBytes = Get-Bytes -Value $Password
        $keyFileBytes = Get-KeyFileBytes -KeyFilePath $KeyFilePath
        $combined = [byte[]]::new($passwordBytes.Length + $keyFileBytes.Length)
        [Buffer]::BlockCopy($passwordBytes, 0, $combined, 0, $passwordBytes.Length)
        if ($keyFileBytes.Length -gt 0) {
            [Buffer]::BlockCopy($keyFileBytes, 0, $combined, $passwordBytes.Length, $keyFileBytes.Length)
        }
        $kdf = [System.Security.Cryptography.Rfc2898DeriveBytes]::new(
            $combined, $Salt, $validatedIterations, [System.Security.Cryptography.HashAlgorithmName]::SHA256
        )
        return @{ encryptionKey = $kdf.GetBytes(32); macKey = $kdf.GetBytes(32) }
    } catch {
        throw $script:CouldNotUnlockVault
    } finally {
        if ($null -ne $kdf) { $kdf.Dispose() }
        Clear-Bytes -Bytes $combined
        Clear-Bytes -Bytes $passwordBytes
        Clear-Bytes -Bytes $keyFileBytes
    }
}

function Clear-KeyMaterial {
    param([hashtable]$Keys)
    if ($null -ne $Keys) {
        Clear-Bytes -Bytes $Keys.encryptionKey
        Clear-Bytes -Bytes $Keys.macKey
    }
}

function Test-FixedTimeEquals {
    param([Parameter(Mandatory)][byte[]]$Left, [Parameter(Mandatory)][byte[]]$Right)
    return [System.Security.Cryptography.CryptographicOperations]::FixedTimeEquals($Left, $Right)
}

function New-HmacSha256 {
    param([Parameter(Mandatory)][byte[]]$Key, [Parameter(Mandatory)][byte[]]$Data)
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

function ConvertTo-ProtectedHeaderJson {
    param([Parameter(Mandatory)][hashtable]$Envelope)
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
    return ($header | ConvertTo-Json -Depth 8 -Compress)
}

function New-VaultEnvelope {
    param(
        [Parameter(Mandatory)][int]$Iterations,
        [Parameter(Mandatory)][byte[]]$Salt,
        [Parameter(Mandatory)][string]$CipherName,
        [Parameter(Mandatory)][byte[]]$InitializationBytes,
        [Parameter(Mandatory)][bool]$UsesKeyFile
    )
    $cipher = if ($CipherName -eq "AES-CBC-HMAC") {
        [ordered]@{ name = $CipherName; iv = ConvertTo-Base64 -Bytes $InitializationBytes; hmac = "" }
    } else {
        [ordered]@{ name = $CipherName; nonce = ConvertTo-Base64 -Bytes $InitializationBytes; tag = "" }
    }
    return [ordered]@{
        format = "PassportVault"
        version = 1
        kdf = [ordered]@{ name = "PBKDF2-SHA256"; iterations = $Iterations; salt = ConvertTo-Base64 -Bytes $Salt }
        cipher = $cipher
        factors = [ordered]@{ keyFile = $UsesKeyFile; windowsUserBinding = $false }
        ciphertext = ""
    }
}

function Protect-VaultPayload {
    param(
        [Parameter(Mandatory)][string]$PlainJson,
        [Parameter(Mandatory)][string]$Password,
        [hashtable]$Options = @{}
    )
    $keys = $null
    $plainBytes = $null
    try {
        $iterations = if ($Options.ContainsKey("Iterations")) {
            Get-ValidatedPbkdf2Iterations -Value $Options.Iterations
        } else {
            $script:DefaultPbkdf2Iterations
        }
        $keyFilePath = if ($Options.ContainsKey("KeyFilePath")) { [string]$Options.KeyFilePath } else { $null }
        $usesKeyFile = -not [string]::IsNullOrWhiteSpace($keyFilePath)
        $forceCipherSpecified = $Options.ContainsKey("ForceCipher")
        $cipherName = if ($forceCipherSpecified) { [string]$Options.ForceCipher } else { "AES-GCM" }
        if (-not $forceCipherSpecified -and $Options.ContainsKey("SimulateAesGcmUnavailable") -and [bool]$Options.SimulateAesGcmUnavailable) {
            $cipherName = "AES-CBC-HMAC"
        }
        if ($cipherName -notin @("AES-GCM", "AES-CBC-HMAC")) {
            throw $script:UnsupportedVaultVersion
        }
        $salt = New-RandomBytes -Length 32
        $initializationBytes = if ($cipherName -eq "AES-CBC-HMAC") {
            New-RandomBytes -Length 16
        } else {
            New-RandomBytes -Length 12
        }
        $keys = New-KeyMaterial -Password $Password -Salt $salt -Iterations $iterations -KeyFilePath $keyFilePath
        $plainBytes = Get-Bytes -Value $PlainJson
        $envelope = New-VaultEnvelope -Iterations $iterations -Salt $salt -CipherName $cipherName -InitializationBytes $initializationBytes -UsesKeyFile $usesKeyFile

        if ($cipherName -eq "AES-CBC-HMAC") {
            $aes = $null
            $encryptor = $null
            try {
                $aes = [System.Security.Cryptography.Aes]::Create()
                $aes.Mode = [System.Security.Cryptography.CipherMode]::CBC
                $aes.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
                $aes.Key = $keys.encryptionKey
                $aes.IV = $initializationBytes
                $encryptor = $aes.CreateEncryptor()
                $cipherBytes = $encryptor.TransformFinalBlock($plainBytes, 0, $plainBytes.Length)
            } finally {
                if ($null -ne $encryptor) { $encryptor.Dispose() }
                if ($null -ne $aes) { $aes.Dispose() }
            }
            $headerBytes = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $envelope)
            $macInput = Join-Bytes -Parts @($headerBytes, $initializationBytes, $cipherBytes)
            $mac = New-HmacSha256 -Key $keys.macKey -Data $macInput
            $envelope.cipher.hmac = ConvertTo-Base64 -Bytes $mac
            $envelope.ciphertext = ConvertTo-Base64 -Bytes $cipherBytes
            return $envelope
        }

        $cipherBytes = [byte[]]::new($plainBytes.Length)
        $tag = [byte[]]::new(16)
        $aad = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $envelope)
        $aes = $null
        try {
            $aes = [System.Security.Cryptography.AesGcm]::new($keys.encryptionKey)
            $aes.Encrypt($initializationBytes, $plainBytes, $cipherBytes, $tag, $aad)
        } catch [System.PlatformNotSupportedException] {
            if ($forceCipherSpecified) {
                throw
            }
            $initializationBytes = New-RandomBytes -Length 16
            $envelope = New-VaultEnvelope -Iterations $iterations -Salt $salt -CipherName "AES-CBC-HMAC" -InitializationBytes $initializationBytes -UsesKeyFile $usesKeyFile
            $cbc = $null
            $encryptor = $null
            try {
                $cbc = [System.Security.Cryptography.Aes]::Create()
                $cbc.Mode = [System.Security.Cryptography.CipherMode]::CBC
                $cbc.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
                $cbc.Key = $keys.encryptionKey
                $cbc.IV = $initializationBytes
                $encryptor = $cbc.CreateEncryptor()
                $cipherBytes = $encryptor.TransformFinalBlock($plainBytes, 0, $plainBytes.Length)
            } finally {
                if ($null -ne $encryptor) { $encryptor.Dispose() }
                if ($null -ne $cbc) { $cbc.Dispose() }
            }
            $headerBytes = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $envelope)
            $macInput = Join-Bytes -Parts @($headerBytes, $initializationBytes, $cipherBytes)
            $mac = New-HmacSha256 -Key $keys.macKey -Data $macInput
            $envelope.cipher.hmac = ConvertTo-Base64 -Bytes $mac
            $envelope.ciphertext = ConvertTo-Base64 -Bytes $cipherBytes
            return $envelope
        } finally {
            if ($null -ne $aes) { $aes.Dispose() }
        }
        $envelope.cipher.tag = ConvertTo-Base64 -Bytes $tag
        $envelope.ciphertext = ConvertTo-Base64 -Bytes $cipherBytes
        return $envelope
    } finally {
        Clear-Bytes -Bytes $plainBytes
        Clear-KeyMaterial -Keys $keys
    }
}

function Unprotect-VaultPayload {
    param(
        [Parameter(Mandatory)][hashtable]$Envelope,
        [Parameter(Mandatory)][string]$Password,
        [string]$KeyFilePath
    )
    $keys = $null
    $plainBytes = $null
    try {
        if ($Envelope.format -isnot [string]) {
            throw $script:CouldNotUnlockVault
        }
        if ($Envelope.format -ne "PassportVault") {
            throw $script:UnsupportedVaultVersion
        }
        $version = 0
        if (-not [int]::TryParse([string]$Envelope.version, [ref]$version)) {
            throw $script:CouldNotUnlockVault
        }
        if ($version -ne 1) {
            throw $script:UnsupportedVaultVersion
        }
        if ($Envelope.kdf.name -isnot [string]) {
            throw $script:CouldNotUnlockVault
        }
        if ($Envelope.kdf.name -ne "PBKDF2-SHA256") {
            throw $script:UnsupportedVaultVersion
        }
        if ($Envelope.cipher.name -isnot [string]) {
            throw $script:CouldNotUnlockVault
        }
        if ($Envelope.cipher.name -notin @("AES-GCM", "AES-CBC-HMAC")) {
            throw $script:UnsupportedVaultVersion
        }
        if ($Envelope.factors.keyFile -isnot [bool] -or $Envelope.factors.windowsUserBinding -isnot [bool]) {
            throw $script:CouldNotUnlockVault
        }
        if ($Envelope.factors.windowsUserBinding) {
            throw $script:CouldNotUnlockVault
        }
        $requiresKeyFile = $Envelope.factors.keyFile
        $hasKeyFilePath = -not [string]::IsNullOrWhiteSpace($KeyFilePath)
        if (($requiresKeyFile -and -not $hasKeyFilePath) -or (-not $requiresKeyFile -and $hasKeyFilePath)) {
            throw $script:CouldNotUnlockVault
        }

        $iterations = Get-ValidatedPbkdf2Iterations -Value $Envelope.kdf.iterations
        $salt = Get-RequiredBase64Bytes -Value $Envelope.kdf.salt
        Assert-ByteLength -Bytes $salt -ExpectedLength 32
        $keyPathForDerivation = if ($requiresKeyFile) { $KeyFilePath } else { $null }
        $keys = New-KeyMaterial -Password $Password -Salt $salt -Iterations $iterations -KeyFilePath $keyPathForDerivation
        $cipherBytes = Get-RequiredBase64Bytes -Value $Envelope.ciphertext
        $aad = Get-Bytes -Value (ConvertTo-ProtectedHeaderJson -Envelope $Envelope)

        if ($Envelope.cipher.name -eq "AES-CBC-HMAC") {
            $iv = Get-RequiredBase64Bytes -Value $Envelope.cipher.iv
            $expectedMac = Get-RequiredBase64Bytes -Value $Envelope.cipher.hmac
            Assert-ByteLength -Bytes $iv -ExpectedLength 16
            Assert-ByteLength -Bytes $expectedMac -ExpectedLength 32
            if ($cipherBytes.Length -eq 0 -or $cipherBytes.Length % 16 -ne 0) {
                throw $script:CouldNotUnlockVault
            }
            $macInput = Join-Bytes -Parts @($aad, $iv, $cipherBytes)
            $actualMac = New-HmacSha256 -Key $keys.macKey -Data $macInput
            if (-not (Test-FixedTimeEquals -Left $expectedMac -Right $actualMac)) {
                throw $script:CouldNotUnlockVault
            }
            $aes = $null
            $decryptor = $null
            try {
                $aes = [System.Security.Cryptography.Aes]::Create()
                $aes.Mode = [System.Security.Cryptography.CipherMode]::CBC
                $aes.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
                $aes.Key = $keys.encryptionKey
                $aes.IV = $iv
                $decryptor = $aes.CreateDecryptor()
                $plainBytes = $decryptor.TransformFinalBlock($cipherBytes, 0, $cipherBytes.Length)
                return [System.Text.Encoding]::UTF8.GetString($plainBytes)
            } finally {
                if ($null -ne $decryptor) { $decryptor.Dispose() }
                if ($null -ne $aes) { $aes.Dispose() }
            }
        }

        $nonce = Get-RequiredBase64Bytes -Value $Envelope.cipher.nonce
        $tag = Get-RequiredBase64Bytes -Value $Envelope.cipher.tag
        Assert-ByteLength -Bytes $nonce -ExpectedLength 12
        Assert-ByteLength -Bytes $tag -ExpectedLength 16
        $plainBytes = [byte[]]::new($cipherBytes.Length)
        $aes = [System.Security.Cryptography.AesGcm]::new($keys.encryptionKey)
        try {
            $aes.Decrypt($nonce, $cipherBytes, $tag, $plainBytes, $aad)
        } finally {
            $aes.Dispose()
        }
        return [System.Text.Encoding]::UTF8.GetString($plainBytes)
    } catch {
        if ($_.Exception.Message -eq $script:UnsupportedVaultVersion) {
            throw $script:UnsupportedVaultVersion
        }
        throw $script:CouldNotUnlockVault
    } finally {
        Clear-Bytes -Bytes $plainBytes
        Clear-KeyMaterial -Keys $keys
    }
}

Export-ModuleMember -Function New-RandomBytes, New-KeyMaterial, Protect-VaultPayload, Unprotect-VaultPayload, ConvertTo-ProtectedHeaderJson
