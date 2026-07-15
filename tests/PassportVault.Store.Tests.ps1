Import-Module "$PSScriptRoot/../src/PassportVault.Crypto.psm1" -Force
Import-Module "$PSScriptRoot/../src/PassportVault.Entries.psm1" -Force
Import-Module "$PSScriptRoot/../src/PassportVault.Store.psm1" -Force

Describe "PassportVault store" {
    BeforeEach {
        $script:path = Join-Path $TestDrive "passport-vault.dat"
    }

    It "saves and loads a vault" {
        $vault = New-VaultPayload
        $vault = Add-VaultEntry -Vault $vault -Entry (New-VaultEntry -Name "site" -Username "user" -Password "pass" -Notes "")

        Save-Vault -Vault $vault -VaultPath $script:path -Password "master"
        $loaded = Load-Vault -VaultPath $script:path -Password "master"

        $loaded.schemaVersion | Should -Be 1
        $loaded.entries[0].name | Should -Be "site"
    }

    It "reports whether the vault exists" {
        Test-VaultExists -VaultPath $script:path | Should -BeFalse
        Save-Vault -Vault (New-VaultPayload) -VaultPath $script:path -Password "master"
        Test-VaultExists -VaultPath $script:path | Should -BeTrue
    }

    It "rejects unsupported vault schema after decrypting" {
        $vault = New-VaultPayload
        $vault.schemaVersion = 99
        Save-Vault -Vault $vault -VaultPath $script:path -Password "master"

        { Load-Vault -VaultPath $script:path -Password "master" } | Should -Throw
    }
}
