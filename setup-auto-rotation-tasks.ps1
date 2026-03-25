# Sets up Windows Task Scheduler jobs for auto-rotation
# Run this script once as Administrator

$ProjectDir = "C:\Users\Dan\Desktop\Front Office Dynasty Sports\fantasy-baseball-go"

# 1. Weekly task: Sunday 4 PM Pacific
$weeklyAction = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$ProjectDir\auto-rotation-weekly.ps1`"" `
    -WorkingDirectory $ProjectDir

$weeklyTrigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Sunday -At "4:00PM"

$weeklySettings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable

Register-ScheduledTask `
    -TaskName "FODS - Auto Rotation Weekly" `
    -Action $weeklyAction `
    -Trigger $weeklyTrigger `
    -Settings $weeklySettings `
    -Description "Auto-fill Colorado Rockies pitching rotation from MLB probables every Sunday at 4 PM Pacific" `
    -Force

Write-Host "Created: FODS - Auto Rotation Weekly (Sunday 4 PM)" -ForegroundColor Green

# 2. Daily task: Mon-Sat 9 AM Pacific
$dailyAction = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$ProjectDir\auto-rotation-daily.ps1`"" `
    -WorkingDirectory $ProjectDir

$dailyTrigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday,Tuesday,Wednesday,Thursday,Friday,Saturday -At "9:00AM"

$dailySettings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable

Register-ScheduledTask `
    -TaskName "FODS - Auto Rotation Daily Check" `
    -Action $dailyAction `
    -Trigger $dailyTrigger `
    -Settings $dailySettings `
    -Description "Check and update today's pitching rotation from MLB probables Mon-Sat at 9 AM Pacific" `
    -Force

Write-Host "Created: FODS - Auto Rotation Daily Check (Mon-Sat 9 AM)" -ForegroundColor Green
Write-Host ""
Write-Host "Tasks created! View in Task Scheduler or run:" -ForegroundColor Cyan
Write-Host "  Get-ScheduledTask | Where-Object TaskName -like 'FODS*'"
