package selfcheck

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aceberg/WatchYourLAN/internal/arp"
	"github.com/aceberg/WatchYourLAN/internal/models"
)

const statusOK = "ok"
const statusWarn = "warn"
const statusError = "error"

// Run executes lightweight runtime diagnostics for the scanner.
func Run(ifaces string) models.SelfCheck {
	var result models.SelfCheck

	arpScanPath := checkArpScan(&result)
	checkArpScanPermission(&result, arpScanPath)
	checkInterfaces(&result, ifaces)
	checkLastScanErrors(&result)

	result.OK = true
	for _, item := range result.Items {
		if item.Status == statusError {
			result.OK = false
			break
		}
	}

	return result
}

func addItem(result *models.SelfCheck, name, status, detail, fix string) {
	result.Items = append(result.Items, models.SelfCheckItem{
		Name:   name,
		Status: status,
		Detail: detail,
		Fix:    fix,
	})
}

func checkArpScan(result *models.SelfCheck) string {
	path, err := exec.LookPath("arp-scan")
	if err != nil {
		addItem(result, "arp-scan", statusError, "arp-scan is not available in PATH", "sudo apt install arp-scan")
		return ""
	}

	addItem(result, "arp-scan", statusOK, path, "")
	return path
}

func checkArpScanPermission(result *models.SelfCheck, arpScanPath string) {
	if arpScanPath == "" {
		return
	}

	if os.Geteuid() == 0 {
		addItem(result, "arp-scan permission", statusOK, "WatchYourLAN is running as root", "")
		return
	}

	output, err := exec.Command("getcap", arpScanPath).CombinedOutput()
	caps := strings.TrimSpace(string(output))
	if err != nil && caps == "" {
		addItem(result, "arp-scan permission", statusWarn, "cannot read file capabilities: "+err.Error(), "getcap "+arpScanPath)
		return
	}

	if hasCapNetRaw(caps) {
		addItem(result, "arp-scan permission", statusOK, caps, "")
		return
	}

	fix := fmt.Sprintf("sudo setcap cap_net_raw+p %s\ngetcap %s\nsystemctl --user restart watchyourlan", arpScanPath, arpScanPath)
	addItem(result, "arp-scan permission", statusError, "missing cap_net_raw for non-root scanning", fix)
}

func hasCapNetRaw(caps string) bool {
	for _, field := range strings.Fields(caps) {
		if !strings.Contains(field, "cap_net_raw") {
			continue
		}

		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 && strings.Contains(parts[1], "p") {
			return true
		}
	}

	return false
}

func checkInterfaces(result *models.SelfCheck, ifaces string) {
	selection := arp.ScanInterfaceSelection(ifaces)

	switch {
	case len(selection.Selected) == 0:
		addItem(result, "interfaces", statusError, "no usable IPv4 interfaces found", "check the Interfaces value in Config and make sure the target network adapter is connected")
	case len(selection.Invalid) > 0:
		detail := fmt.Sprintf("selected: %s; unavailable: %s", strings.Join(selection.Selected, " "), strings.Join(selection.Invalid, " "))
		addItem(result, "interfaces", statusWarn, detail, "remove unavailable interfaces from Config or connect those adapters")
	default:
		addItem(result, "interfaces", statusOK, "selected: "+strings.Join(selection.Selected, " "), "")
	}
}

func checkLastScanErrors(result *models.SelfCheck) {
	errors := arp.LastScanErrors()
	if len(errors) == 0 {
		addItem(result, "last scan", statusOK, "no scan errors captured", "")
		return
	}

	for _, item := range errors {
		detail := item.Error
		if item.Output != "" {
			detail = detail + ": " + item.Output
		}
		addItem(result, "last scan "+item.Source, statusError, detail, item.Command)
	}
}
