# Create or refresh the SuplaOS-dev virtual machine in Hyper-V and start it.
# Usually run by tools/hyperv.sh, which converts the image to VHDX first.
#
#   hyperv.ps1 -Dir F:\Hyper-V\SuplaOS [-NewDisk suplaos-dev.new.vhdx]
#              [-VMName SuplaOS-dev] [-SwitchName "..."] [-DiskSize 16GB]
#
# Without -SwitchName the first external switch is used, so the VM gets
# an address from the router like a real device.
param(
    [Parameter(Mandatory = $true)][string]$Dir,
    [string]$NewDisk = "",
    [string]$VMName = "SuplaOS-dev",
    [string]$SwitchName = "",
    [UInt64]$DiskSize = 16GB,
    [UInt64]$Memory = 2GB,
    [int]$Cpus = 2
)

$ErrorActionPreference = "Stop"

$principal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    # Hyper-V cmdlets need admin rights: run again elevated and wait for it.
    $argList = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$PSCommandPath`"",
                 "-Dir", "`"$Dir`"", "-VMName", "`"$VMName`"",
                 "-DiskSize", $DiskSize, "-Memory", $Memory, "-Cpus", $Cpus)
    if ($NewDisk) { $argList += @("-NewDisk", "`"$NewDisk`"") }
    if ($SwitchName) { $argList += @("-SwitchName", "`"$SwitchName`"") }
    # WaitForExit() instead of -Wait: -Wait would also wait for vmconnect
    # started by the elevated script.
    $p = Start-Process powershell.exe -Verb RunAs -ArgumentList $argList -PassThru
    $p.WaitForExit()
    exit $p.ExitCode
}

try {
    $disk = Join-Path $Dir "$VMName.vhdx"

    $vm = Get-VM -Name $VMName -ErrorAction SilentlyContinue
    if ($vm -and $vm.State -ne "Off") {
        Write-Host "Stopping $VMName"
        Stop-VM -Name $VMName -TurnOff -Force
    }

    if ($NewDisk) {
        Write-Host "Replacing disk $disk"
        Move-Item -Force (Join-Path $Dir $NewDisk) $disk
        Resize-VHD -Path $disk -SizeBytes $DiskSize
    }
    if (-not (Test-Path $disk)) { throw "No disk $disk" }

    if (-not $vm) {
        if (-not $SwitchName) {
            $SwitchName = (Get-VMSwitch -SwitchType External | Select-Object -First 1).Name
            if (-not $SwitchName) { throw "No external virtual switch, pass -SwitchName" }
        }
        Write-Host "Creating $VMName (switch: $SwitchName)"
        $vm = New-VM -Name $VMName -Generation 2 -MemoryStartupBytes $Memory `
                     -VHDPath $disk -SwitchName $SwitchName -Path $Dir
        Set-VM -Name $VMName -ProcessorCount $Cpus -StaticMemory `
               -AutomaticCheckpointsEnabled $false -AutomaticStopAction ShutDown
        # GRUB in the image is not signed.
        Set-VMFirmware -VMName $VMName -EnableSecureBoot Off `
                       -FirstBootDevice (Get-VMHardDiskDrive -VMName $VMName)
    }

    Start-VM -Name $VMName
    Write-Host "$VMName started. Panel: https://suplaos.local:8443"
    # The console shows the IP address too.
    Start-Process vmconnect.exe -ArgumentList "localhost", "`"$VMName`""
}
catch {
    Write-Host $_ -ForegroundColor Red
    Read-Host "Press Enter to close"
    exit 1
}
