# SPDX-License-Identifier: Apache-2.0
#
# Installs ajent on Windows (docs/adr/003-distribution-channels.md).
# Downloads the release zip for this architecture, verifies its SHA-256
# against checksums.txt, checks the Ed25519 signature through the
# trusted public key with OpenSSL 3 before extraction or execution, installs it and
# records the install method in install.json. Runs on Windows PowerShell
# 5.1 and PowerShell 7. On macOS and Linux, use install.sh instead:
# curl -fsSL https://ajent.social/install.sh | sh
#
# Environment variables (same names as install.sh):
#   AJENT_VERSION              Version to install, e.g. "0.7.0" or "v0.7.0".
#                               Default: the service-pinned signed release.
#   AJENT_INSTALL_DIR          Where to install ajent.exe.
#                               Default: $env:LOCALAPPDATA\Programs\ajent
#   AJENT_CONFIG_DIR           Where to write install.json.
#                               Default: $HOME\.aj
#   AJENT_NO_SETUP             When "1", skip running "ajent setup".
#   AJENT_DOWNLOAD_BASE        Base URL for release assets, for tests and
#                               mirrors. Default:
#                               the service-configured public release host.
#                               AJENT_VERSION is required when this is set.
#   AJENT_RELEASE_PUBKEY_FILE  A PEM file replacing the embedded release
#                               public key. For tests only.

$ErrorActionPreference = 'Stop'
if ($args.Count -gt 0) { throw 'installer arguments are not supported; run ajent setup with your options after installation' }
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function ajentInstallSay {
    param([string]$Message)
    [Console]::Error.WriteLine("ajent-install: $Message")
}

function ajentInstallStripLeadingV {
    param([string]$Value)
    if ($Value.Length -gt 0 -and $Value.Substring(0, 1) -eq 'v') {
        return $Value.Substring(1)
    }
    return $Value
}

function ajentInstallDetectArch {
    $raw = $env:PROCESSOR_ARCHITEW6432
    if (-not $raw) {
        $raw = $env:PROCESSOR_ARCHITECTURE
    }
    switch ($raw) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { throw "unsupported architecture $raw" }
    }
}

function ajentInstallResolveVersion {
    $v = $env:AJENT_VERSION
    if ($v) {
        $v = ajentInstallStripLeadingV $v
    }

    if (-not $v) { $v = '__AJENT_RELEASE_VERSION__' }

    if ($v -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$') {
        throw "could not determine a valid version (got '$v')"
    }

    return $v
}

function ajentInstallVerifyChecksum {
    param([string]$FilePath, [string]$Name, [string]$ChecksumsPath)

    $want = $null
    foreach ($line in Get-Content -LiteralPath $ChecksumsPath) {
        $fields = $line -split '\s+', 2
        if ($fields.Length -lt 2) {
            continue
        }
        $entryName = $fields[1].TrimStart('*', ' ')
        if ($entryName -eq $Name) {
            $want = $fields[0].ToLower()
            break
        }
    }
    if (-not $want) {
        throw "checksum mismatch for $Name"
    }

    $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $FilePath).Hash.ToLower()
    if ($want -ne $got) {
        throw "checksum mismatch for $Name"
    }
}

function ajentInstallVerifySignature {
    param([string]$ChecksumsPath, [string]$SigPath)

    $keyPath = Join-Path (Split-Path -Path $SigPath -Parent) 'release-key.pem'
    if ($env:AJENT_RELEASE_PUBKEY_FILE) {
        Copy-Item -LiteralPath $env:AJENT_RELEASE_PUBKEY_FILE -Destination $keyPath
    } else {
        $releaseKey = @'
__AJENT_RELEASE_PUBLIC_KEY__
'@
        if ($releaseKey -notmatch '^-----BEGIN PUBLIC KEY-----') {
            throw "no release key is embedded in this installer"
        }
        $releaseKey | Set-Content -LiteralPath $keyPath -Encoding ascii
    }
    $openssl = Get-Command -Name 'openssl.exe', 'openssl' -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $openssl) { throw "OpenSSL 3 is required to verify the release before installation" }
    $sigBin = Join-Path (Split-Path -Path $SigPath -Parent) 'sig.bin'
    [IO.File]::WriteAllBytes($sigBin, [Convert]::FromBase64String((Get-Content -LiteralPath $SigPath -Raw).Trim()))
    & $openssl.Source pkeyutl -verify -pubin -inkey $keyPath -rawin -in $ChecksumsPath -sigfile $sigBin | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "release signature does not verify (OpenSSL 3 required)" }
}

function ajentInstallPathContains {
    param([string]$PathList, [string]$Dir)

    if (-not $PathList) {
        return $false
    }
    $normalizedDir = $Dir.TrimEnd('\')
    foreach ($entry in ($PathList -split ';')) {
        if ($entry -and ($entry.TrimEnd('\') -ieq $normalizedDir)) {
            return $true
        }
    }
    return $false
}

function Install-Ajent {
    $arch = ajentInstallDetectArch
    $v = ajentInstallResolveVersion

    $base = $env:AJENT_DOWNLOAD_BASE
    if (-not $base) {
        $base = '__AJENT_RELEASE_DOWNLOAD_BASE__'
    }
    $name = "ajent_${v}_windows_${arch}.zip"

    $tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ("ajent-" + [guid]::NewGuid()))
    try {
        $zip = Join-Path $tmp.FullName $name
        $checksums = Join-Path $tmp.FullName 'checksums.txt'
        $sig = Join-Path $tmp.FullName 'checksums.txt.sig'

        Invoke-WebRequest -UseBasicParsing -Uri "$base/v$v/$name" -OutFile $zip
        Invoke-WebRequest -UseBasicParsing -Uri "$base/v$v/checksums.txt" -OutFile $checksums
        Invoke-WebRequest -UseBasicParsing -Uri "$base/v$v/checksums.txt.sig" -OutFile $sig

        ajentInstallVerifySignature -ChecksumsPath $checksums -SigPath $sig
        ajentInstallVerifyChecksum -FilePath $zip -Name $name -ChecksumsPath $checksums

        $extractDir = Join-Path $tmp.FullName 'extracted'
        Expand-Archive -Path $zip -DestinationPath $extractDir -Force
        $newExe = Join-Path $extractDir 'ajent.exe'
        if (-not (Test-Path -LiteralPath $newExe)) {
            throw "ajent.exe was not found in the downloaded archive"
        }


        $dir = $env:AJENT_INSTALL_DIR
        if (-not $dir) {
            $dir = Join-Path $env:LOCALAPPDATA 'Programs\ajent'
        }
        New-Item -ItemType Directory -Force -Path $dir | Out-Null

        $exe = Join-Path $dir 'ajent.exe'
        $oldExe = Join-Path $dir 'ajent.exe.old'
        if (Test-Path -LiteralPath $oldExe) {
            Remove-Item -LiteralPath $oldExe -Force
        }
        if (Test-Path -LiteralPath $exe) {
            Rename-Item -LiteralPath $exe -NewName 'ajent.exe.old'
        }
        Move-Item -LiteralPath $newExe -Destination $exe -Force

        $currentPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not (ajentInstallPathContains -PathList $currentPath -Dir $dir)) {
            $newPath = $dir
            if ($currentPath) {
                $newPath = "$currentPath;$dir"
            }
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
            $env:Path = "$dir;$env:Path"
        }

        $cfg = $env:AJENT_CONFIG_DIR
        if (-not $cfg) {
            $cfg = Join-Path $HOME '.aj'
        }
        New-Item -ItemType Directory -Force -Path $cfg | Out-Null

        $record = [ordered]@{
            method       = 'script'
            version      = $v
            path         = $exe
            installed_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd'T'HH:mm:ss'Z'")
        }
        ($record | ConvertTo-Json -Compress) | Set-Content -LiteralPath (Join-Path $cfg 'install.json') -Encoding ascii -NoNewline

        ajentInstallSay "ajent $v installed to $exe"

        if ($env:AJENT_NO_SETUP -ne '1' -and [Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
            & $exe setup
        } else {
            ajentInstallSay 'Next: run "ajent setup" to connect your coding agents.'
        }
    }
    finally {
        Remove-Item -LiteralPath $tmp.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Install-Ajent
