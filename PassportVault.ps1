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
