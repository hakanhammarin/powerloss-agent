# PowerLoss Agent

A Go agent for Windows 10/11 that monitors laptop power status, exposes a REST API, and advertises itself on the local network via mDNS/DNS-SD for zero-configuration discovery.

## Features

- Checks if laptop is charging
- Reports State of Charge (SOC)
- Reports computer name, location, and IP addresses
- Simple REST API for monitoring software
- Auto-renames computer when settings file is updated
- **mDNS/DNS-SD advertisement** — automatically discoverable by monitoring_js and any RFC 6762/6763 compatible browser (including OpenThread Border Router DNS-SD)

## Service Discovery (mDNS)

The agent registers itself as a DNS-SD service on startup:

| Field        | Value                                      |
|--------------|--------------------------------------------|
| Service type | `_powerloss._tcp.local`                    |
| Service name | `PowerLoss-Agent-<ComputerName>`           |
| Port         | Configured port (default 8080)             |
| TXT records  | `path=/api/status`, `location=<Location>`, `version=1.0` |

Any tool that speaks mDNS/DNS-SD (Bonjour, Avahi, OpenThread Border Router) can discover running agents without manual IP configuration.

### OpenThread / Thread network compatibility

The `_powerloss._tcp` service type is standard DNS-SD (RFC 6763) over mDNS (RFC 6762). OpenThread Border Routers proxy mDNS records between Thread mesh networks and standard IP networks using the same DNS-SD protocol, so agents on Thread-connected hosts are discoverable by the monitoring_js browser and vice versa.

## API Endpoints

- `GET /api/status` — Get power status and system info
- `GET /api/settings` — Get current settings
- `POST /api/settings/update` — Update settings

### Status response example

```json
{
  "computer_name": "LAPTOP-01",
  "location": "Office A",
  "ip_addresses": ["192.168.1.42"],
  "is_charging": false,
  "soc": 67,
  "timestamp": "2026-06-05T12:00:00Z"
}
```

## Settings File (settings.json)

```json
{
  "computer_name": "MyLaptop",
  "location": "Office",
  "port": 8080
}
```

When `computer_name` is changed (via API or by editing the file), the agent renames the computer in Windows. The `location` value is also published in the mDNS TXT record.

## Install & Run

1. Build (see Build section below) or download the pre-built `powerloss-agent.exe`
2. Place `powerloss-agent.exe` and `settings.json` in the same directory
3. Run `powerloss-agent.exe` (runs in foreground; use Task Scheduler or NSSM for service install)
4. The agent logs its port and registers on mDNS automatically

## Build for Windows

Cross-compile from macOS/Linux:

```bash
GOOS=windows GOARCH=amd64 go build -o powerloss-agent.exe .
```

Or on Windows:

```bat
go build -o powerloss-agent.exe .
```

## Requirements

- Go 1.21+
- Windows 10/11 with PowerShell (for battery status and computer rename)
- No Bonjour/Avahi installation required — mDNS is handled by the embedded `grandcat/zeroconf` library

## Linux

The same source builds for Linux (`GOOS=linux GOARCH=amd64 go build -o powerloss-agent .`). Battery status is read from `/sys/class/power_supply/BAT*`. The computer is not renamed on Linux; a new `computer_name` is only reported by the API.

To emulate a laptop on a machine without a battery, set `POWER_SUPPLY_DIR` to a directory with two files, `status` (`Charging` or `Discharging`) and `capacity` (0 to 100). `powerloss.sh` in `10mila_automation` runs several such agents as systemd services on a lab VM.
