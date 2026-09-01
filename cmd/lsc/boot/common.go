package boot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

const (
	defaultMenderConf  = "/etc/mender/mender.conf"
	fallbackMenderConf = "/var/lib/mender/mender.conf"
)

var partNumRe = regexp.MustCompile(`([0-9]+)$`)

type slotInfo struct {
	Label  string
	Device string
	Num    int
}

type menderLayout struct {
	A slotInfo
	B slotInfo
}

type bootState struct {
	Layout           menderLayout
	CurrentNum       int
	NextNum          int
	UpgradeAvailable string
	BootCount        string
	BootLimit        string
}

func loadLayout() (menderLayout, error) {
	var conf struct {
		RootfsPartA string
		RootfsPartB string
	}

	for _, path := range []string{fallbackMenderConf, defaultMenderConf} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &conf); err != nil {
			return menderLayout{}, fmt.Errorf("parsing %s: %w", path, err)
		}
		if conf.RootfsPartA != "" && conf.RootfsPartB != "" {
			break
		}
	}

	if conf.RootfsPartA == "" || conf.RootfsPartB == "" {
		return menderLayout{}, fmt.Errorf("RootfsPartA/RootfsPartB not found in mender.conf")
	}

	a, err := slotFromDevice("A", conf.RootfsPartA)
	if err != nil {
		return menderLayout{}, err
	}
	b, err := slotFromDevice("B", conf.RootfsPartB)
	if err != nil {
		return menderLayout{}, err
	}
	return menderLayout{A: a, B: b}, nil
}

func slotFromDevice(label, dev string) (slotInfo, error) {
	m := partNumRe.FindStringSubmatch(dev)
	if m == nil {
		return slotInfo{}, fmt.Errorf("cannot extract partition number from %q", dev)
	}
	var num int
	if _, err := fmt.Sscanf(m[1], "%d", &num); err != nil {
		return slotInfo{}, fmt.Errorf("parsing partition number %q: %w", m[1], err)
	}
	return slotInfo{Label: label, Device: dev, Num: num}, nil
}

// currentPartNum returns the partition number of the currently mounted rootfs
// by comparing the device ID of "/" with the rdev of each slot device.
func currentPartNum(layout menderLayout) (int, error) {
	var rootStat syscall.Stat_t
	if err := syscall.Stat("/", &rootStat); err != nil {
		return 0, fmt.Errorf("stat /: %w", err)
	}
	for _, s := range []slotInfo{layout.A, layout.B} {
		var pStat syscall.Stat_t
		if err := syscall.Stat(s.Device, &pStat); err != nil {
			continue
		}
		if uint64(pStat.Rdev) == uint64(rootStat.Dev) {
			return s.Num, nil
		}
	}
	return 0, fmt.Errorf("current rootfs does not match either A (%s) or B (%s)", layout.A.Device, layout.B.Device)
}

func (l menderLayout) slotByNum(num int) (slotInfo, bool) {
	switch num {
	case l.A.Num:
		return l.A, true
	case l.B.Num:
		return l.B, true
	}
	return slotInfo{}, false
}

func (l menderLayout) other(num int) (slotInfo, error) {
	switch num {
	case l.A.Num:
		return l.B, nil
	case l.B.Num:
		return l.A, nil
	}
	return slotInfo{}, fmt.Errorf("partition %d is neither A (%d) nor B (%d)", num, l.A.Num, l.B.Num)
}

func fwPrintenv(key string) (string, error) {
	out, err := exec.Command("fw_printenv", "-n", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

var removeFwSetenvScript = os.Remove

// fwSetenvBatch writes multiple env vars in one fw_setenv script invocation,
// keeping the U-Boot environment update atomic. Some fw_setenv implementations
// silently ignore `-s -`, so pass an actual, securely-created script file.
func fwSetenvBatch(vars map[string]string) (retErr error) {
	f, err := os.CreateTemp("", "lsc-fw_setenv-*")
	if err != nil {
		return fmt.Errorf("create fw_setenv script: %w", err)
	}
	path := f.Name()
	defer func() {
		if err := removeFwSetenvScript(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remove fw_setenv script: %w", err))
		}
	}()

	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var script strings.Builder
	for _, key := range keys {
		script.WriteString(key)
		script.WriteString("=")
		script.WriteString(vars[key])
		script.WriteString("\n")
	}
	if _, err := f.WriteString(script.String()); err != nil {
		writeErr := fmt.Errorf("write fw_setenv script: %w", err)
		if closeErr := f.Close(); closeErr != nil {
			return errors.Join(writeErr, fmt.Errorf("close fw_setenv script: %w", closeErr))
		}
		return writeErr
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close fw_setenv script: %w", err)
	}

	out, err := exec.Command("fw_setenv", "-s", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("fw_setenv: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readBootState() (bootState, error) {
	layout, err := loadLayout()
	if err != nil {
		return bootState{}, err
	}
	cur, err := currentPartNum(layout)
	if err != nil {
		return bootState{}, err
	}

	next, _ := fwPrintenv("mender_boot_part")
	upg, _ := fwPrintenv("upgrade_available")
	bc, _ := fwPrintenv("bootcount")
	bl, _ := fwPrintenv("bootlimit")

	nextNum := 0
	if next != "" {
		// A malformed value here matches the same tolerance as the
		// fwPrintenv calls above: fall back to the zero value rather
		// than fail the whole status read over one bootloader field.
		_, _ = fmt.Sscanf(next, "%d", &nextNum)
	}

	return bootState{
		Layout:           layout,
		CurrentNum:       cur,
		NextNum:          nextNum,
		UpgradeAvailable: upg,
		BootCount:        bc,
		BootLimit:        bl,
	}, nil
}

func labelFor(layout menderLayout, num int) string {
	if s, ok := layout.slotByNum(num); ok {
		return s.Label
	}
	return "?"
}

func confirmPrompt(msg string) bool {
	fmt.Printf("%s [y/N]: ", msg)
	var r string
	// An unreadable answer (EOF, closed stdin) leaves r empty, which
	// falls through to the same "not confirmed" result as an explicit no.
	_, _ = fmt.Scanln(&r)
	r = strings.ToLower(strings.TrimSpace(r))
	return r == "y" || r == "yes"
}
