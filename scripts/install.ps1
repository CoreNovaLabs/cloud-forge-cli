param(
  [string]$Version = $env:CLOUD_FORGE_VERSION,
  [string]$Repo = $env:CLOUD_FORGE_REPO,
  [string]$InstallDir = $env:CLOUD_FORGE_INSTALL_DIR
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Repo)) {
  $Repo = "CoreNovaLabs/cloud-forge-cli"
}
if ([string]::IsNullOrWhiteSpace($Version)) {
  $Version = "latest"
}
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
  $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\CloudForge"
}

if ($PSVersionTable.PSEdition -eq "Desktop") {
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}

function Resolve-CloudForgeVersion {
  param([string]$RequestedVersion, [string]$Repository)

  if ($RequestedVersion -ne "latest") {
    if ($RequestedVersion.StartsWith("v")) {
      return $RequestedVersion
    }
    return "v$RequestedVersion"
  }

  $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest" -Headers @{
    "User-Agent" = "cloud-forge-installer"
  }
  return $release.tag_name
}

function Get-CloudForgeArch {
  $arch = $env:PROCESSOR_ARCHITECTURE
  if ($arch -eq "AMD64") {
    return "amd64"
  }
  if ($arch -eq "ARM64") {
    return "arm64"
  }
  throw "Unsupported Windows architecture: $arch. Download a release manually: https://github.com/$Repo/releases"
}

function Add-InstallDirToUserPath {
  param([string]$PathToAdd)

  $currentUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @()
  if (-not [string]::IsNullOrWhiteSpace($currentUserPath)) {
    $parts = $currentUserPath -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  }
  if ($parts -notcontains $PathToAdd) {
    $newUserPath = (($parts + $PathToAdd) -join ";")
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    Write-Host "Added $PathToAdd to your user PATH. Open a new terminal to use it there."
  }

  $sessionParts = $env:Path -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  if ($sessionParts -notcontains $PathToAdd) {
    $env:Path = "$PathToAdd;$env:Path"
  }
}

function Invoke-CloudForgeDownload {
  param([string]$Uri, [string]$OutFile)

  $maxAttempts = 5
  for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
    try {
      Invoke-WebRequest -Uri $Uri -OutFile $OutFile -Headers @{
        "User-Agent" = "cloud-forge-installer"
      }
      return
    }
    catch {
      if ($attempt -ge $maxAttempts) {
        throw
      }
      Write-Warning "Download failed; retrying ($attempt/$maxAttempts): $($_.Exception.Message)"
      Start-Sleep -Seconds ([Math]::Min(10, $attempt * 2))
    }
  }
}

$resolvedVersion = Resolve-CloudForgeVersion -RequestedVersion $Version -Repository $Repo
if ([string]::IsNullOrWhiteSpace($resolvedVersion)) {
  throw "Could not resolve latest release for $Repo"
}

$versionNumber = $resolvedVersion.TrimStart("v")
$arch = Get-CloudForgeArch
$archiveName = "cloud-forge_${versionNumber}_windows_${arch}.zip"
$downloadUrl = "https://github.com/$Repo/releases/download/$resolvedVersion/$archiveName"
$tempDir = Join-Path ([IO.Path]::GetTempPath()) ("cloud-forge-install-" + [Guid]::NewGuid().ToString("N"))
$archivePath = Join-Path $tempDir $archiveName

try {
  New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

  Write-Host "Installing cloud-forge $resolvedVersion for windows_$arch"
  Invoke-CloudForgeDownload -Uri $downloadUrl -OutFile $archivePath
  Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force

  $sourceExe = Join-Path $tempDir "cloud-forge.exe"
  if (-not (Test-Path $sourceExe)) {
    throw "Downloaded archive did not contain cloud-forge.exe"
  }

  $targetExe = Join-Path $InstallDir "cloud-forge.exe"
  Copy-Item -Path $sourceExe -Destination $targetExe -Force
  Unblock-File -Path $targetExe -ErrorAction SilentlyContinue
  Add-InstallDirToUserPath -PathToAdd $InstallDir

  Write-Host "Installed cloud-forge to $targetExe"
  & $targetExe version
}
finally {
  if (Test-Path $tempDir) {
    Remove-Item -Recurse -Force $tempDir
  }
}
