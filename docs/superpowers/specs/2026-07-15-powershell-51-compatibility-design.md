# PassportVault PowerShell 5.1 Compatibility Design

## Goal

Make PassportVault usable from Windows PowerShell 5.1 without requiring PowerShell 7.

## Scope

- Support creating, saving, and loading vaults that use `AES-CBC-HMAC`.
- Keep the existing encrypted JSON envelope format.
- Avoid runtime network downloads and third-party crypto dependencies.
- Do not add migration support for vaults created with `AES-GCM`.
- Do not require PowerShell 5.1 to read `AES-GCM` vaults.

## Compatibility Strategy

PowerShell 5.1 runs on .NET Framework, so the default execution path must avoid .NET Core-only APIs. New vaults will default to `AES-CBC-HMAC`, which already provides authenticated encryption through AES-CBC plus HMAC-SHA256.

The crypto module will provide local compatibility helpers for APIs that are unavailable in .NET Framework:

- Random bytes through `RandomNumberGenerator.Create().GetBytes()`.
- Constant-time byte comparison through a manual XOR accumulator.
- PBKDF2-HMAC-SHA256 through a compatible implementation if the runtime constructor is unavailable.

AES-GCM code may remain present for runtimes that support it, but it will not be selected by default and it is not part of the PowerShell 5.1 compatibility promise.

## User-Facing Behavior

The README will list PowerShell 5.1+ as supported and use `powershell.exe` examples. It will document that PowerShell 5.1 uses `AES-CBC-HMAC` and cannot open existing `AES-GCM` vaults.

## Testing

Tests will cover the compatibility helpers and the default vault round trip. AES-GCM-specific tests can remain guarded by runtime support checks, but the default cipher expectation should become `AES-CBC-HMAC`.
