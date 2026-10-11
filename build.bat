@echo off
echo Building PowerLoss Agent for Windows...
go build -o powerloss-agent.exe main.go
if %ERRORLEVEL% EQU 0 (
    echo Build successful: powerloss-agent.exe
) else (
    echo Build failed
)
