package arp

import (
	"log/slog"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aceberg/WatchYourLAN/internal/models"
)

var arpArgs string
var lastScanErrors = scanErrors{
	items: make(map[string]ScanError),
}

// ScanError is the last arp-scan failure captured for one source.
type ScanError struct {
	Source  string
	Command string
	Error   string
	Output  string
}

type scanErrors struct {
	sync.RWMutex
	items map[string]ScanError
}

func scanIface(iface string) string {
	args := []string{"-glNx"}
	if arpArgs != "" {
		args = append(args, strings.Fields(arpArgs)...)
	}
	args = append(args, "-I", iface)

	cmd := exec.Command("arp-scan", args...)
	out, err := cmd.CombinedOutput()
	slog.Debug(cmd.String())

	if err != nil {
		recordScanError(iface, cmd.String(), err.Error(), string(out))
		slog.Warn("arp-scan failed", "iface", iface, "cmd", cmd.String(), "err", err, "output", strings.TrimSpace(string(out)))
		return string("")
	}
	clearScanError(iface)
	return string(out)
}

func scanStr(str string) string {

	args := strings.Split(str, " ")
	cmd := exec.Command("arp-scan", args...)

	out, err := cmd.CombinedOutput()
	slog.Debug(cmd.String())

	if err != nil {
		recordScanError(str, cmd.String(), err.Error(), string(out))
		slog.Warn("arp-scan failed", "scan", str, "cmd", cmd.String(), "err", err, "output", strings.TrimSpace(string(out)))
		return string("")
	}
	clearScanError(str)
	return string(out)
}

func recordScanError(source, command, errText, output string) {
	lastScanErrors.Lock()
	defer lastScanErrors.Unlock()

	lastScanErrors.items[source] = ScanError{
		Source:  source,
		Command: command,
		Error:   strings.TrimSpace(errText),
		Output:  strings.TrimSpace(output),
	}
}

func clearScanError(source string) {
	lastScanErrors.Lock()
	defer lastScanErrors.Unlock()

	delete(lastScanErrors.items, source)
}

// LastScanErrors returns the last arp-scan failures keyed by interface or scan string.
func LastScanErrors() []ScanError {
	lastScanErrors.RLock()
	defer lastScanErrors.RUnlock()

	errors := make([]ScanError, 0, len(lastScanErrors.items))
	for _, item := range lastScanErrors.items {
		errors = append(errors, item)
	}
	sort.Slice(errors, func(i, j int) bool {
		return errors[i].Source < errors[j].Source
	})

	return errors
}

func parseOutput(text, iface string) []models.Host {
	var foundHosts = []models.Host{}

	p := strings.Split(text, "\n")

	for _, host := range p {
		if host != "" {
			var oneHost models.Host
			p := strings.Split(host, "	")
			oneHost.Iface = iface
			oneHost.IP = p[0]
			oneHost.Mac = p[1]
			oneHost.Hw = p[2]
			oneHost.Date = time.Now().Format("2006-01-02 15:04:05")
			oneHost.Now = 1
			foundHosts = append(foundHosts, oneHost)
		}
	}

	return foundHosts
}

// Scan all interfaces
func Scan(ifaces, args string, strs []string) []models.Host {
	var text string
	var p []string
	var foundHosts = []models.Host{}
	arpArgs = args

	p = resolveScanInterfaces(ifaces)
	for _, iface := range p {
		slog.Debug("Scanning interface " + iface)
		text = scanIface(iface)
		slog.Debug("Found IPs: \n" + text)

		foundHosts = append(foundHosts, parseOutput(text, iface)...)
	}

	for _, s := range strs {
		slog.Debug("Scanning string " + s)
		text = scanStr(s)
		slog.Debug("Found IPs: \n" + text)
		p = strings.Split(s, " ")

		foundHosts = append(foundHosts, parseOutput(text, p[len(p)-1])...)
	}

	return foundHosts
}

func resolveScanInterfaces(ifaces string) []string {
	selection := ScanInterfaceSelection(ifaces)

	if len(selection.Invalid) > 0 {
		slog.Warn("Ignoring unavailable scan interfaces", "ifaces", strings.Join(selection.Invalid, " "))
	}
	if selection.UsedAuto && len(selection.Selected) > 0 {
		slog.Warn("Using auto-detected scan interfaces", "ifaces", strings.Join(selection.Selected, " "))
	}

	return selection.Selected
}

// InterfaceSelection describes how scan interfaces were selected.
type InterfaceSelection struct {
	Configured []string
	Available  []string
	Selected   []string
	Invalid    []string
	UsedAuto   bool
}

// ScanInterfaceSelection returns configured, available, selected, and invalid scan interfaces.
func ScanInterfaceSelection(ifaces string) InterfaceSelection {
	configured := strings.Fields(ifaces)
	available := usableInterfaceMap()
	auto := autoScanInterfaces(available)
	selected, invalid, usedAuto := selectScanInterfaces(configured, available, auto)

	return InterfaceSelection{
		Configured: configured,
		Available:  mapKeys(available),
		Selected:   selected,
		Invalid:    invalid,
		UsedAuto:   usedAuto,
	}
}

func selectScanInterfaces(configured []string, available map[string]bool, auto []string) ([]string, []string, bool) {
	if len(configured) == 0 {
		return uniqueStrings(auto), nil, len(auto) > 0
	}

	var selected []string
	var invalid []string
	for _, iface := range configured {
		if available[iface] {
			selected = appendUniqueString(selected, iface)
			continue
		}
		invalid = appendUniqueString(invalid, iface)
	}

	if len(selected) > 0 {
		return selected, invalid, false
	}

	return uniqueStrings(auto), invalid, len(auto) > 0
}

func usableInterfaceMap() map[string]bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		slog.Warn("Cannot list network interfaces", "err", err)
		return nil
	}

	available := make(map[string]bool)
	for _, iface := range interfaces {
		if isUsableInterface(iface) {
			available[iface.Name] = true
		}
	}

	return available
}

func isUsableInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return false
	}

	addrs, err := iface.Addrs()
	if err != nil {
		slog.Debug("Cannot list interface addresses", "iface", iface.Name, "err", err)
		return false
	}

	for _, addr := range addrs {
		if hasUsableIPv4(addr) {
			return true
		}
	}

	return false
}

func hasUsableIPv4(addr net.Addr) bool {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return false
	}

	ip := ipNet.IP.To4()
	return ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

func autoScanInterfaces(available map[string]bool) []string {
	if len(available) == 0 {
		return nil
	}

	ifaces := defaultRouteInterfaces(available)
	if len(ifaces) > 0 {
		return ifaces
	}

	for iface := range available {
		if isVirtualInterface(iface) {
			continue
		}
		ifaces = append(ifaces, iface)
	}
	sort.Strings(ifaces)
	return ifaces
}

func defaultRouteInterfaces(available map[string]bool) []string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		slog.Debug("Cannot read default routes", "err", err)
		return nil
	}

	var ifaces []string
	minMetric := -1
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 7 || fields[1] != "00000000" {
			continue
		}

		iface := fields[0]
		if !available[iface] || isVirtualInterface(iface) {
			continue
		}

		metric, err := strconv.Atoi(fields[6])
		if err != nil {
			metric = 0
		}

		switch {
		case minMetric == -1 || metric < minMetric:
			minMetric = metric
			ifaces = []string{iface}
		case metric == minMetric:
			ifaces = appendUniqueString(ifaces, iface)
		}
	}

	sort.Strings(ifaces)
	return ifaces
}

func isVirtualInterface(iface string) bool {
	prefixes := []string{
		"br-",
		"docker",
		"lxc",
		"tailscale",
		"tap",
		"tun",
		"vboxnet",
		"veth",
		"virbr",
		"vmnet",
		"wg",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(iface, prefix) {
			return true
		}
	}

	return false
}

func uniqueStrings(values []string) []string {
	var out []string
	for _, value := range values {
		out = appendUniqueString(out, value)
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}

	for _, item := range values {
		if item == value {
			return values
		}
	}

	return append(values, value)
}

func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
