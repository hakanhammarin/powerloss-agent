package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

type Settings struct {
	ComputerName string `json:"computer_name"`
	Location     string `json:"location"`
	Port         int    `json:"port"`
}

type PowerStatus struct {
	ComputerName string   `json:"computer_name"`
	Location     string   `json:"location"`
	IPAddresses  []string `json:"ip_addresses"`
	IsCharging   bool     `json:"is_charging"`
	SOC          int      `json:"soc"`
	Timestamp    string   `json:"timestamp"`
}

var (
	settings     Settings
	settingsLock sync.RWMutex
	settingsFile = "settings.json"
)

func loadSettings() error {
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			settings = Settings{ComputerName: "", Location: "Unknown", Port: 8080}
			return saveSettings()
		}
		return err
	}
	return json.Unmarshal(data, &settings)
}

func saveSettings() error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsFile, data, 0644)
}

func getComputerName() string {
	settingsLock.RLock()
	defer settingsLock.RUnlock()
	if settings.ComputerName != "" {
		return settings.ComputerName
	}
	name, _ := os.Hostname()
	return name
}

func getIPAddresses() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ips = append(ips, ipnet.IP.String())
				}
			}
		}
	}
	return ips
}

// linuxPowerStatus reads the kernel's battery files. POWER_SUPPLY_DIR points it at a
// directory holding fake "status" and "capacity" files, to emulate a laptop on a machine without a battery.
func linuxPowerStatus() (bool, int) {
	dir := os.Getenv("POWER_SUPPLY_DIR")
	if dir == "" {
		found, _ := filepath.Glob("/sys/class/power_supply/BAT*")
		if len(found) == 0 {
			return false, 0
		}
		dir = found[0]
	}
	status, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return false, 0
	}
	capacity, _ := os.ReadFile(filepath.Join(dir, "capacity"))
	soc, _ := strconv.Atoi(strings.TrimSpace(string(capacity)))
	// "Charging", "Full" and "Not charging" all mean mains power is connected.
	return strings.TrimSpace(string(status)) != "Discharging", soc
}

func getPowerStatus() (bool, int) {
	if runtime.GOOS == "linux" {
		return linuxPowerStatus()
	}
	if runtime.GOOS != "windows" {
		return false, 0
	}

	cmd := exec.Command("powershell", "-Command", "(Get-WmiObject -Class Win32_Battery).BatteryStatus;(Get-WmiObject -Class Win32_Battery).EstimatedChargeRemaining")
	output, err := cmd.Output()
	if err != nil {
		return false, 0
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return false, 0
	}

	isCharging := strings.TrimSpace(lines[0]) == "2"
	var soc int
	fmt.Sscanf(lines[1], "%d", &soc)

	return isCharging, soc
}

func renameComputer(newName string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("rename only supported on Windows")
	}

	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Rename-Computer -NewName \"%s\" -Force", newName))
	return cmd.Run()
}

func watchSettings() {
	info, err := os.Stat(settingsFile)
	if err != nil {
		return
	}
	modTime := info.ModTime()

	for {
		time.Sleep(2 * time.Second)
		info, err := os.Stat(settingsFile)
		if err != nil {
			continue
		}
		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
			oldName := getComputerName()

			loadSettings()

			settingsLock.RLock()
			newName := settings.ComputerName
			settingsLock.RUnlock()

			if newName != "" && newName != oldName {
				log.Printf("Renaming computer from %s to %s", oldName, newName)
				if err := renameComputer(newName); err != nil {
					log.Printf("Failed to rename computer: %v", err)
				}
			}
		}
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	isCharging, soc := getPowerStatus()

	settingsLock.RLock()
	status := PowerStatus{
		ComputerName: getComputerName(),
		Location:     settings.Location,
		IPAddresses:  getIPAddresses(),
		IsCharging:   isCharging,
		SOC:          soc,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	settingsLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	settingsLock.RLock()
	defer settingsLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func updateSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newSettings Settings
	if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	settingsLock.Lock()
	oldName := settings.ComputerName
	settings = newSettings
	settingsLock.Unlock()

	saveSettings()

	if newSettings.ComputerName != "" && newSettings.ComputerName != oldName {
		log.Printf("Renaming computer from %s to %s", oldName, newSettings.ComputerName)
		if err := renameComputer(newSettings.ComputerName); err != nil {
			log.Printf("Failed to rename computer: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newSettings)
}

func main() {
	if err := loadSettings(); err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}

	settingsLock.RLock()
	port := settings.Port
	settingsLock.RUnlock()

	if port == 0 {
		port = 8080
	}

	go watchSettings()

	http.HandleFunc("/api/status", statusHandler)
	http.HandleFunc("/api/settings", settingsHandler)
	http.HandleFunc("/api/settings/update", updateSettingsHandler)

	settingsLock.RLock()
	location := settings.Location
	settingsLock.RUnlock()

	txtRecords := []string{
		"path=/api/status",
		"location=" + location,
		"version=1.0",
	}
	server, _ := zeroconf.Register("PowerLoss-Agent-"+getComputerName(), "_powerloss._tcp", "local.", port, txtRecords, nil)
	defer server.Shutdown()

	log.Printf("Agent running on port %d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
