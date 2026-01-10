#!/usr/bin/env pwsh
cd "D:\projects\singnbox-launch"

# Test just the subscription package (no UI)
Write-Host "Running VLESS tests..." -ForegroundColor Cyan
$output = & go test ./core/config/subscription -v -run "TestParseNode_VLESS" -timeout 30s 2>&1
Write-Host $output
Write-Host "Exit code: $LASTEXITCODE" -ForegroundColor Yellow

if ($LASTEXITCODE -eq 0) {
    Write-Host "`n✅ VLESS tests PASSED!" -ForegroundColor Green
} else {
    Write-Host "`n❌ VLESS tests FAILED!" -ForegroundColor Red
}

exit $LASTEXITCODE
