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

function Get-UserFacingErrorMessage {
    param([Parameter(Mandatory)][string]$Message)
    switch ($Message) {
        "Vault file not found" { return "Vault file not found." }
        "Could not unlock vault" { return "Could not unlock vault." }
        "Unsupported vault version" { return "Unsupported vault version." }
        "Could not save vault" { return "Could not save vault." }
        "Passwords do not match." { return "Passwords do not match." }
        "Entry not found" { return "Entry not found." }
        default { return "Operation failed." }
    }
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

function Start-ClipboardClearTimer {
    param(
        [Parameter(Mandatory)][string]$ExpectedValue,
        [int]$DelaySeconds = 30
    )
    if ($null -eq (Get-Command Get-Clipboard -ErrorAction SilentlyContinue) -or
        $null -eq (Get-Command Set-Clipboard -ErrorAction SilentlyContinue)) {
        return $false
    }

    $timer = $null
    $sourceIdentifier = "PassportVault.Clipboard.$([Guid]::NewGuid().ToString('N'))"
    try {
        $timer = [System.Timers.Timer]::new($DelaySeconds * 1000)
        $timer.AutoReset = $false
        Register-ObjectEvent -InputObject $timer -EventName Elapsed -SourceIdentifier $sourceIdentifier -MessageData @{
            expectedValue = $ExpectedValue
            sourceIdentifier = $sourceIdentifier
            timer = $timer
        } -Action {
            try {
                $currentValue = Get-Clipboard -Raw -ErrorAction Stop
                if ($currentValue -ceq $event.MessageData.expectedValue) {
                    Set-Clipboard -Value ([string]::Empty) -ErrorAction Stop
                }
            } catch {
            } finally {
                $event.MessageData.timer.Dispose()
                Unregister-Event -SourceIdentifier $event.MessageData.sourceIdentifier -ErrorAction SilentlyContinue
            }
        } | Out-Null
        $timer.Start()
        return $true
    } catch {
        if ($null -ne $timer) { $timer.Dispose() }
        Unregister-Event -SourceIdentifier $sourceIdentifier -ErrorAction SilentlyContinue
        return $false
    }
}

function Copy-EntryPassword {
    param([Parameter(Mandatory)][string]$Password)
    if ($null -eq (Get-Command Set-Clipboard -ErrorAction SilentlyContinue)) {
        Write-Host "Clipboard is unavailable."
        return
    }
    try {
        Set-Clipboard -Value $Password -ErrorAction Stop
        Write-Host "Password copied to clipboard."
        if (Start-ClipboardClearTimer -ExpectedValue $Password -DelaySeconds 30) {
            Write-Host "Clipboard will clear in 30 seconds if it is unchanged."
        }
    } catch {
        Write-Host "Clipboard is unavailable."
    }
}

function Show-EntryDetails {
    param([Parameter(Mandatory)][hashtable]$Entry)
    Write-Host "Name: $($Entry.name)"
    Write-Host "Username: $($Entry.username)"
    Write-Host "Password: ********"
    Write-Host "Notes: $($Entry.notes)"
    $action = ([string](Read-Host "[R]eveal, [C]opy, or Enter to return")).Trim().ToLowerInvariant()
    switch ($action) {
        "r" { Write-Host "Password: $($Entry.password)" }
        "c" { Copy-EntryPassword -Password $Entry.password }
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
    [object[]]$results = Search-VaultEntries -Vault $Vault -Query $query
    if ($results.Count -eq 0) {
        Write-Host "No matches."
        return
    }
    for ($i = 0; $i -lt $results.Count; $i++) {
        Write-Host ("{0}. {1} ({2})" -f ($i + 1), $results[$i].name, $results[$i].username)
    }

    $choice = Read-Host "Open result number (Enter to return)"
    if ([string]::IsNullOrWhiteSpace($choice)) { return }
    $index = 0
    if ([int]::TryParse($choice, [ref]$index) -and $index -ge 1 -and $index -le $results.Count) {
        Show-EntryDetails -Entry $results[$index - 1]
        return
    }
    Write-Host "Invalid choice."
}

function Edit-EntryInteractive {
    param([Parameter(Mandatory)][hashtable]$Vault)
    $entry = Select-Entry -Vault $Vault
    if ($null -eq $entry) { return $null }
    $name = Read-Host "Name [$($entry.name)]"
    $username = Read-Host "Username [$($entry.username)]"
    $changePassword = Read-Host "Change password? (y/N)"
    $notes = Read-Host "Notes [$($entry.notes)]"
    $changes = @{}
    if (-not [string]::IsNullOrWhiteSpace($name)) { $changes.name = $name }
    if (-not [string]::IsNullOrWhiteSpace($username)) { $changes.username = $username }
    if ($changePassword -eq "y") { $changes.password = Read-EntryPassword -Prompt "New item password" }
    if (-not [string]::IsNullOrWhiteSpace($notes)) { $changes.notes = $notes }
    if ($changes.Count -eq 0) { return $null }
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
            "5" {
                $updatedVault = Edit-EntryInteractive -Vault $Vault
                if ($null -ne $updatedVault) {
                    $Vault = $updatedVault
                    Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $Password -KeyFilePath $KeyFilePath
                }
            }
            "6" {
                $entry = Select-Entry -Vault $Vault
                if ($entry -and (Read-Host "Delete $($entry.name)? (y/N)") -eq "y") {
                    $Vault = Remove-VaultEntry -Vault $Vault -Id $entry.id
                    Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $Password -KeyFilePath $KeyFilePath
                }
            }
            "7" {
                $newPassword = $null
                $confirmPassword = $null
                try {
                    $newPassword = Read-EntryPassword -Prompt "New entry password"
                    $confirmPassword = Read-EntryPassword -Prompt "Confirm new entry password"
                    if ($newPassword -ne $confirmPassword) { Write-Host "Passwords do not match."; break }
                    Save-Vault -Vault $Vault -VaultPath $VaultPath -Password $newPassword -KeyFilePath $KeyFilePath
                    $Password = $newPassword
                } finally {
                    $newPassword = $null
                    $confirmPassword = $null
                }
            }
            "8" { return }
            default { Write-Host "Invalid choice." }
        }
    }
}

$password = $null
$confirm = $null
$vault = $null
$exitCode = 0
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
    Write-Host (Get-UserFacingErrorMessage -Message $_.Exception.Message)
    $exitCode = 1
} finally {
    # PowerShell strings are immutable, but dropping references limits their lifetime.
    $password = $null
    $confirm = $null
    $vault = $null
}
if ($exitCode -ne 0) { exit $exitCode }
