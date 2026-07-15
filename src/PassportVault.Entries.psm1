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
