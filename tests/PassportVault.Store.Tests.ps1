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

    It "overwrites an existing vault" {
        $firstVault = New-VaultPayload
        $firstVault = Add-VaultEntry -Vault $firstVault -Entry (New-VaultEntry -Name "first" -Username "user" -Password "pass" -Notes "")
        Save-Vault -Vault $firstVault -VaultPath $script:path -Password "master"

        $secondVault = New-VaultPayload
        $secondVault = Add-VaultEntry -Vault $secondVault -Entry (New-VaultEntry -Name "second" -Username "user" -Password "pass" -Notes "")
        Save-Vault -Vault $secondVault -VaultPath $script:path -Password "master"

        $loaded = Load-Vault -VaultPath $script:path -Password "master"

        $loaded.entries | Should -HaveCount 1
        $loaded.entries[0].name | Should -Be "second"
    }

    It "sanitizes encryption failures while saving" {
        Mock -CommandName Protect-VaultPayload -ModuleName PassportVault.Store -MockWith {
            throw "encryption failed"
        }

        { Save-Vault -Vault (New-VaultPayload) -VaultPath $script:path -Password "master" } | Should -Throw "Could not save vault"
    }

    It "preserves the backup when replacement fails during Save-Vault" {
        InModuleScope PassportVault.Store -Parameters @{ VaultPath = $script:path } {
            param($VaultPath)
            $script:replacementBackupPath = $null
            [System.IO.File]::WriteAllText($VaultPath, "existing vault")
            Mock -CommandName Invoke-VaultFileReplacement -MockWith {
                param($SourcePath, $DestinationPath, $BackupPath)
                $script:replacementBackupPath = $BackupPath
                [System.IO.File]::WriteAllText($BackupPath, "original vault")
                throw "replacement failed"
            }

            { Save-Vault -Vault @{ schemaVersion = 1; entries = @() } -VaultPath $VaultPath -Password "master" } |
                Should -Throw -ExpectedMessage "Could not save vault"
            [System.IO.File]::Exists($script:replacementBackupPath) | Should -BeTrue
        }
    }

    It "rejects unsupported vault schema after decrypting" {
        $vault = New-VaultPayload
        $vault.schemaVersion = 99
        Save-Vault -Vault $vault -VaultPath $script:path -Password "master"

        { Load-Vault -VaultPath $script:path -Password "master" } | Should -Throw
    }
}
