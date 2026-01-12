$found = $false

Get-ChildItem bin -Filter *.json -Recurse | ForEach-Object {
    $bytes = [System.IO.File]::ReadAllBytes($_.FullName)
    if ($bytes.Length -ge 3 -and
        $bytes[0] -eq 0xEF -and
        $bytes[1] -eq 0xBB -and
        $bytes[2] -eq 0xBF) {

        Write-Host "❌ BOM found in $($_.FullName)"
        $found = $true
    }
}

if ($found) {
    Write-Host "❌ BOM found ↑ see above ↑"
    exit 1   # ← оставь для CI
} else {
    Write-Host "✅ No BOM found"
}