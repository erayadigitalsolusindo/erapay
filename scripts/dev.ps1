<#
.SYNOPSIS
  Perintah harian EraPay untuk Windows PowerShell (setara Makefile).
.EXAMPLE
  .\scripts\dev.ps1 help
  .\scripts\dev.ps1 up
#>
param([Parameter(Position = 0)][string]$Task = 'help', [Parameter(Position = 1)][string]$MigrateArg = 'up')

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Compose = Join-Path $Root 'infra/compose/docker-compose.yml'

# Muat .env ke environment proses ini (tanpa mencetak isinya).
function Import-DotEnv {
    $file = Join-Path $Root '.env'
    if (-not (Test-Path $file)) { throw ".env tidak ditemukan. Jalankan: Copy-Item .env.example .env" }
    foreach ($line in Get-Content $file) {
        if ($line -match '^\s*#' -or $line -notmatch '=') { continue }
        $name, $value = $line -split '=', 2
        [Environment]::SetEnvironmentVariable($name.Trim(), $value.Trim(), 'Process')
    }
}

# Hentikan skrip bila perintah eksternal (go, npm, docker) gagal.
function Invoke-Checked([scriptblock]$Cmd) {
    & $Cmd
    if ($LASTEXITCODE -ne 0) { throw "Perintah gagal (exit $LASTEXITCODE)" }
}

switch ($Task) {
    'up'      { Invoke-Checked { docker compose -f $Compose up -d } }
    'down'    { Invoke-Checked { docker compose -f $Compose down } }
    'install' { Push-Location (Join-Path $Root 'apps/web'); try { Invoke-Checked { npm install } } finally { Pop-Location } }
    'api'     {
        Import-DotEnv
        Push-Location (Join-Path $Root 'services/api')
        try { Invoke-Checked { go run ./cmd/api } } finally { Pop-Location }
    }
    'web'     { Push-Location (Join-Path $Root 'apps/web'); try { Invoke-Checked { npm run dev } } finally { Pop-Location } }
    'test'    {
        Push-Location (Join-Path $Root 'services/api'); try { Invoke-Checked { go test ./... } } finally { Pop-Location }
        Push-Location (Join-Path $Root 'apps/web');     try { Invoke-Checked { npm test } }       finally { Pop-Location }
    }
    'check'   {
        Push-Location (Join-Path $Root 'services/api')
        try {
            $unformatted = gofmt -l .
            if ($unformatted) { throw "File Go belum diformat:`n$unformatted" }
            Invoke-Checked { go vet ./... }
        } finally { Pop-Location }
        Push-Location (Join-Path $Root 'apps/web'); try { Invoke-Checked { npm run check } } finally { Pop-Location }
    }
    'migrate' { Import-DotEnv; Push-Location (Join-Path $Root 'services/api'); try { Invoke-Checked { go run ./cmd/migrate $MigrateArg } } finally { Pop-Location } }
    default {
        @'
Perintah: .\scripts\dev.ps1 <nama>
  up       Nyalakan PostgreSQL, Redis, MinIO (Docker Desktop harus berjalan)
  down     Matikan layanan Docker
  install  Pasang dependensi frontend
  api      Jalankan Go API di :8080 (butuh up dan file .env)
  web      Jalankan SvelteKit dev server di :5173
  test     Jalankan semua test (Go + frontend)
  check    Cek format/tipe (Go + frontend)
  migrate  Migrasi DB (up, down, status; default up)
'@ | Write-Host
    }
}
