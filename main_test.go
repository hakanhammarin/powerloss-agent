package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLinuxPowerStatus(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip()
	}
	dir := t.TempDir()
	t.Setenv("POWER_SUPPLY_DIR", dir)
	for _, c := range []struct {
		status   string
		capacity string
		charging bool
		soc      int
	}{{"Charging\n", "80\n", true, 80}, {"Discharging\n", "42\n", false, 42}, {"Full\n", "100\n", true, 100}} {
		os.WriteFile(filepath.Join(dir, "status"), []byte(c.status), 0644)
		os.WriteFile(filepath.Join(dir, "capacity"), []byte(c.capacity), 0644)
		if charging, soc := linuxPowerStatus(); charging != c.charging || soc != c.soc {
			t.Errorf("%q %q: got %v %d", c.status, c.capacity, charging, soc)
		}
	}
}
