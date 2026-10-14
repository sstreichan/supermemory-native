# Builds the dashboard assets with Bun and enforces the Phase 8 bundle gates.
#
#   .\build.ps1
#
# Requires the Bun runtime (https://bun.sh). The generated index.html and
# local-console.js are committed so Go builds never need Bun.
$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

bun run build
if ($LASTEXITCODE -ne 0) { throw "bun run build failed" }

$size = (Get-Item .\local-console.js).Length
$htmlSize = (Get-Item .\index.html).Length
$limit = 51200 # 50 KB hard gate (Phase 2 target: < 40 KB)

Write-Host "local-console.js: $size bytes"
Write-Host "index.html:       $htmlSize bytes"

if ($size -ge $limit) {
  throw "local-console.js is $size bytes, at or above the $limit byte gate."
}

# A bundle that Bun tree-shook down to its empty IIFE wrapper is tiny but
# useless, so require a real API path to be present as well.
$bundle = Get-Content .\local-console.js -Raw
if ($bundle -notmatch [regex]::Escape('/v3/documents/documents')) {
  throw "local-console.js is missing the /v3/documents/documents API path."
}
if ($bundle -notmatch [regex]::Escape('/v4/search')) {
  throw "local-console.js is missing the /v4/search API path."
}

Write-Host "build ok"
