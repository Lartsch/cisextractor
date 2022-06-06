$PSDefaultParameterValues = @{}
$PSDefaultParameterValues += @{'New-RegKey:ErrorAction' = 'SilentlyContinue'}

$name = "cisextractor"
$sourcefolder = "C:\<...>\cisextractor\src"
$buildfolder = "${sourcefolder}\build"
$upxpath = "${sourcefolder}\upx.exe"
$Env:GOARCH = "amd64"

$buildnames =@{
    linux="${name}_linux_amd64";
    darwin="${name}_mac_amd64";
    windows="${name}_win_amd64.exe"
}

$buildnames.GetEnumerator() | % {
    $key =  $($_.key)
    $value = $($_.value)
    $filepath = "${buildfolder}\${value}"
    Write-Host "`nNOW BUILDING: $key - $value"
    Write-Host "Compiling ..."
    $Env:GOOS = $key
    go build -ldflags="-s -w" -o $filepath
    Write-Host "Compressing..."
    & $upxpath -9 -k $filepath
}

Write-Host "`n`nCleaning up...`n"
Get-ChildItem $buildfolder | Foreach-Object {
    if ($_.FullName -match '~$' -or $_.FullName -match '000') {
        rm $_.FullName
    }
}

Write-Host "DONE`n"
