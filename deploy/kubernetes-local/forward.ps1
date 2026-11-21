param([switch]$CheckOnly)

$ErrorActionPreference = 'Stop'
$logRoot = Join-Path $env:LOCALAPPDATA 'socialnet\port-forward'
New-Item -ItemType Directory -Path $logRoot -Force | Out-Null
$statusLog = Join-Path $logRoot 'status.log'
$lastStatus = ''

function Write-ForwardStatus([string]$Message) {
    if ($Message -eq $script:lastStatus) { return }
    if ((Test-Path -LiteralPath $statusLog) -and (Get-Item -LiteralPath $statusLog).Length -gt 1MB) {
        Move-Item -LiteralPath $statusLog -Destination ($statusLog + '.previous') -Force
    }
    Add-Content -LiteralPath $statusLog -Value ('{0:o} {1}' -f (Get-Date), $Message) -Encoding UTF8
    $script:lastStatus = $Message
}

function Test-KubernetesReady([string]$Executable) {
    $info = New-Object System.Diagnostics.ProcessStartInfo
    $info.FileName = $Executable
    $info.Arguments = '--context docker-desktop --request-timeout=5s get --raw=/readyz'
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $probe = New-Object System.Diagnostics.Process
    $probe.StartInfo = $info
    try {
        [void]$probe.Start()
        $stdout = $probe.StandardOutput.ReadToEndAsync()
        $stderr = $probe.StandardError.ReadToEndAsync()
        if (-not $probe.WaitForExit(10000)) {
            $probe.Kill()
            $probe.WaitForExit()
            return $false
        }
        return ($probe.ExitCode -eq 0 -and $stdout.Result.Trim() -eq 'ok')
    } finally {
        $probe.Dispose()
    }
}

# Wait quietly for Docker Desktop/Kubernetes instead of spawning a failing
# console every three seconds. Keep the address stable when a Pod is replaced.
while ($true) {
    $forward = $null
    try {
        $kubectlCommand = Get-Command kubectl.exe -ErrorAction SilentlyContinue
        if (-not $kubectlCommand) {
            Write-ForwardStatus 'Waiting: kubectl.exe is not available on PATH.'
            if ($CheckOnly) { exit 2 }
            Start-Sleep -Seconds 15
            continue
        }
        if (-not (Test-KubernetesReady $kubectlCommand.Source)) {
            Write-ForwardStatus 'Waiting: docker-desktop Kubernetes API is not ready.'
            if ($CheckOnly) { exit 2 }
            Start-Sleep -Seconds 15
            continue
        }
        if ($CheckOnly) {
            Write-Output 'Kubernetes API is ready.'
            exit 0
        }
        Write-ForwardStatus 'Starting socialnet port-forward on 127.0.0.1:8080.'
        $info = New-Object System.Diagnostics.ProcessStartInfo
        $info.FileName = $kubectlCommand.Source
        $info.Arguments = '--context docker-desktop -n socialnet port-forward --address 127.0.0.1 service/socialnet 8080:80'
        $info.UseShellExecute = $false
        $info.CreateNoWindow = $true
        $info.RedirectStandardOutput = $true
        $info.RedirectStandardError = $true
        $forward = New-Object System.Diagnostics.Process
        $forward.StartInfo = $info
        [void]$forward.Start()
        $outFile = [System.IO.File]::Create((Join-Path $logRoot 'forward.out.log'))
        $errFile = [System.IO.File]::Create((Join-Path $logRoot 'forward.err.log'))
        try {
            $outCopy = $forward.StandardOutput.BaseStream.CopyToAsync($outFile)
            $errCopy = $forward.StandardError.BaseStream.CopyToAsync($errFile)
            $forward.WaitForExit()
            $outCopy.GetAwaiter().GetResult()
            $errCopy.GetAwaiter().GetResult()
        } finally {
            $outFile.Dispose()
            $errFile.Dispose()
        }
        Write-ForwardStatus 'Port-forward stopped; waiting before reconnecting.'
    } catch {
        Write-ForwardStatus ('Retrying after error: ' + $_.Exception.Message)
        if ($CheckOnly) { exit 2 }
    } finally {
        if ($null -ne $forward) {
            if (-not $forward.HasExited) { $forward.Kill() }
            $forward.Dispose()
        }
    }
    Start-Sleep -Seconds 15
}
