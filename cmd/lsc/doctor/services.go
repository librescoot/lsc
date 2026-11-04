package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func diagnosticCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// Enabled continuous services are expected to run. Socket/timer activation,
// conditions and oneshots must not turn normal inactivity into a fault.
func inactiveServices() ([]string, error) {
	return readInactiveServices(diagnosticCommand)
}

func readInactiveServices(run func(string, ...string) ([]byte, error)) ([]string, error) {
	out, err := run("systemctl", "list-unit-files", "--type=service", "--state=enabled,enabled-runtime", "--no-legend", "--no-pager")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ".service") || strings.Contains(fields[0], "@.") {
			continue
		}
		if fields[1] == "enabled" || fields[1] == "enabled-runtime" {
			names = append(names, fields[0])
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no enabled services reported")
	}
	args := []string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,SubState,Type,TriggeredBy,ConditionResult,AssertResult,RemainAfterExit"}
	args = append(args, names...)
	out, err = run("systemctl", args...)
	if err != nil {
		return nil, err
	}
	units := parseServiceProperties(string(out))
	var inactive []string
	for _, name := range names {
		properties, ok := units[name]
		if !ok || properties["LoadState"] != "loaded" || properties["ActiveState"] == "" || properties["Type"] == "" {
			return nil, fmt.Errorf("incomplete systemd state for %s", name)
		}
		if properties["ActiveState"] != "inactive" || properties["Type"] == "oneshot" ||
			properties["TriggeredBy"] != "" || properties["ConditionResult"] == "no" ||
			properties["AssertResult"] == "no" || properties["RemainAfterExit"] == "yes" {
			continue
		}
		inactive = append(inactive, name)
	}
	sort.Strings(inactive)
	return inactive, nil
}

func parseServiceProperties(output string) map[string]map[string]string {
	units := make(map[string]map[string]string)
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		properties := make(map[string]string)
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				properties[key] = value
			}
		}
		if id := properties["Id"]; id != "" {
			units[id] = properties
		}
	}
	return units
}
