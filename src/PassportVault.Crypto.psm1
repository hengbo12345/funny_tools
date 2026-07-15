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
