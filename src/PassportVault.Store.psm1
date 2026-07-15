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

function Remove-VaultBackupIfReplaced {
    param(
        [string]$BackupPath,
        [Parameter(Mandatory)][bool]$ReplacementSucceeded
    )
    if (-not $ReplacementSucceeded -or [string]::IsNullOrWhiteSpace($BackupPath)) {
        return
    }
    try {
        if ([System.IO.File]::Exists($BackupPath)) {
            [System.IO.File]::Delete($BackupPath)
        }
    } catch {}
}

function Save-Vault {
    param(
        [Parameter(Mandatory)][hashtable]$Vault,
        [Parameter(Mandatory)][string]$VaultPath,
        [Parameter(Mandatory)][string]$Password,
        [string]$KeyFilePath
    )

    $tempPath = $null
    $backupPath = $null
    $replacementSucceeded = $false
    try {
        $plainJson = $Vault | ConvertTo-Json -Depth 20 -Compress
        $envelope = Protect-VaultPayload -PlainJson $plainJson -Password $Password -Options @{ KeyFilePath = $KeyFilePath }
        $envelopeJson = $envelope | ConvertTo-Json -Depth 20
        $resolvedVaultPath = [System.IO.Path]::GetFullPath($VaultPath)
        $directory = [System.IO.Path]::GetDirectoryName($resolvedVaultPath)
        if (-not [string]::IsNullOrWhiteSpace($directory)) {
            [System.IO.Directory]::CreateDirectory($directory) | Out-Null
        }

        $fileName = [System.IO.Path]::GetFileName($resolvedVaultPath)
        $tempPath = [System.IO.Path]::Combine($directory, ".$fileName.$([System.Guid]::NewGuid().ToString('N')).tmp")
        $backupPath = [System.IO.Path]::Combine($directory, ".$fileName.$([System.Guid]::NewGuid().ToString('N')).bak")
        [System.IO.File]::WriteAllText($tempPath, $envelopeJson, [System.Text.Encoding]::UTF8)
        if ([System.IO.File]::Exists($resolvedVaultPath)) {
            [System.IO.File]::Replace($tempPath, $resolvedVaultPath, $backupPath, $true)
            $replacementSucceeded = $true
        } else {
            [System.IO.File]::Move($tempPath, $resolvedVaultPath)
        }
    } catch {
        if ($null -ne $tempPath) {
            try {
                if ([System.IO.File]::Exists($tempPath)) { [System.IO.File]::Delete($tempPath) }
            } catch {}
        }
        throw "Could not save vault"
    } finally {
        if ($null -ne $tempPath) {
            try {
                if ([System.IO.File]::Exists($tempPath)) { [System.IO.File]::Delete($tempPath) }
            } catch {}
        }
        Remove-VaultBackupIfReplaced -BackupPath $backupPath -ReplacementSucceeded $replacementSucceeded
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
