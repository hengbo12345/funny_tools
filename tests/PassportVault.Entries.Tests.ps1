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
