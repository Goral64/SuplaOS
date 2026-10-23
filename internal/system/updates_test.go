package system

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestParseUpgradable(t *testing.T) {
	out := `Listing...
suplaos/trixie 0.2.0 arm64 [upgradable from: 0.1.0]
linux-image-rpi-v8/trixie 1:6.18.51-1+rpt1 arm64 [upgradable from: 1:6.18.50-1+rpt1]
`
	got := parseUpgradable(out)
	if len(got) != 2 || got[0] != (Package{"suplaos", "0.1.0", "0.2.0"}) || got[1].From != "1:6.18.50-1+rpt1" {
		t.Errorf("parseUpgradable = %+v", got)
	}
}

func TestNewerKernel(t *testing.T) {
	installed := []string{"6.18.50+rpt-rpi-v8", "6.18.51+rpt-rpi-2712", "6.18.50+rpt-rpi-2712"}
	if newerKernel("6.18.50+rpt-rpi-v8", installed) {
		t.Error("Pi 5 kernel counted for Pi 4")
	}
	if !newerKernel("6.18.50+rpt-rpi-2712", installed) {
		t.Error("newer Pi 5 kernel not found")
	}
	if !newerKernel("6.12.48+deb13-amd64", []string{"6.12.48+deb13-amd64", "6.12.57+deb13-amd64"}) {
		t.Error("newer Debian kernel not found")
	}
	if newerKernel("6.12.57+deb13-amd64", []string{"6.12.48+deb13-amd64", "6.12.57+deb13-amd64"}) {
		t.Error("older kernel counted")
	}
}

func TestPackagesBusy(t *testing.T) {
	if file := os.Getenv("SUPLAOS_TEST_LOCK"); file != "" {
		// The other process, like apt, holding the lock.
		f, _ := os.OpenFile(file, os.O_RDWR, 0)
		lk := syscall.Flock_t{Type: syscall.F_WRLCK}
		if err := syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lk); err != nil {
			os.Exit(1)
		}
		os.Stdout.WriteString("ok\n")
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	dpkgLock = filepath.Join(t.TempDir(), "lock-frontend")
	defer func() { dpkgLock = "/var/lib/dpkg/lock-frontend" }()
	if PackagesBusy() {
		t.Error("busy without the file")
	}
	os.WriteFile(dpkgLock, nil, 0o644)
	if PackagesBusy() {
		t.Error("busy without a lock")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPackagesBusy$")
	cmd.Env = append(os.Environ(), "SUPLAOS_TEST_LOCK="+dpkgLock)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	buf := make([]byte, 3)
	if _, err := io.ReadFull(out, buf); err != nil || string(buf) != "ok\n" {
		t.Fatalf("lock holder: %v %q", err, buf)
	}
	if !PackagesBusy() {
		t.Error("lock not seen")
	}
}
