package diagnostics

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exec %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func powershell(script string) (string, error) {
	return runCommand("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// CPU returns usage percentage, top process name, and a raw sample.
func CPU() (cpuUsage int, topProcess string, raw string, err error) {
	switch runtime.GOOS {
	case "windows":
		return cpuWindows()
	case "darwin":
		return cpuDarwin()
	default:
		return cpuLinux()
	}
}

func cpuDarwin() (int, string, string, error) {
	out, err := runCommand("top", "-l", "1")
	if err != nil {
		return 0, "", "", err
	}
	usage, top := parseTopOutput(out, "darwin")
	return usage, top, firstLines(out, 20), nil
}

func cpuLinux() (int, string, string, error) {
	out, err := runCommand("top", "-bn1")
	if err != nil {
		return 0, "", "", err
	}
	usage, top := parseTopOutput(out, "linux")
	return usage, top, firstLines(out, 20), nil
}

func cpuWindows() (int, string, string, error) {
	// Get overall CPU load
	loadOut, err := powershell(`(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average`)
	if err != nil {
		return 0, "", "", err
	}
	var cpuUsage int
	fmt.Sscanf(strings.TrimSpace(loadOut), "%d", &cpuUsage)

	// Get top CPU process
	procOut, err := powershell(`Get-Process | Sort-Object CPU -Descending | Select-Object -First 1 -ExpandProperty ProcessName`)
	if err != nil {
		return cpuUsage, "unknown", loadOut, nil
	}
	topProcess := strings.TrimSpace(procOut)
	if topProcess == "" {
		topProcess = "unknown"
	}
	return cpuUsage, topProcess, loadOut, nil
}

// Memory returns memory stats (total_mb, used_mb, free_mb, usage_percent, etc.).
func Memory() (map[string]interface{}, error) {
	switch runtime.GOOS {
	case "windows":
		return memoryWindows()
	case "darwin":
		out, err := runCommand("vm_stat")
		if err != nil {
			return nil, err
		}
		return parseVMStatOutput(out), nil
	default:
		out, err := runCommand("free", "-m")
		if err != nil {
			return nil, err
		}
		return parseFreeOutput(out), nil
	}
}

func memoryWindows() (map[string]interface{}, error) {
	out, err := powershell(`
$os = Get-CimInstance Win32_OperatingSystem
$total = [math]::Round($os.TotalVisibleMemorySize / 1024)
$free = [math]::Round($os.FreePhysicalMemory / 1024)
$used = $total - $free
$pct = [math]::Round(($used / $total) * 100)
"$total $used $free $pct"
`)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	result := map[string]interface{}{"raw": out}
	if len(fields) >= 4 {
		total, _ := strconv.Atoi(fields[0])
		used, _ := strconv.Atoi(fields[1])
		free, _ := strconv.Atoi(fields[2])
		pct, _ := strconv.Atoi(fields[3])
		result["total_mb"] = total
		result["used_mb"] = used
		result["free_mb"] = free
		result["available_mb"] = free
		result["usage_percent"] = pct
	}
	return result, nil
}

// Disk returns disk usage per filesystem / drive.
func Disk() (map[string]interface{}, error) {
	switch runtime.GOOS {
	case "windows":
		return diskWindows()
	default:
		out, err := runCommand("df", "-h")
		if err != nil {
			return nil, err
		}
		return parseDfOutput(out), nil
	}
}

func diskWindows() (map[string]interface{}, error) {
	out, err := powershell(`
Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | ForEach-Object {
  $size = [math]::Round($_.Size / 1GB, 1)
  $free = [math]::Round($_.FreeSpace / 1GB, 1)
  $used = [math]::Round(($_.Size - $_.FreeSpace) / 1GB, 1)
  $pct = if ($_.Size -gt 0) { [math]::Round((($_.Size - $_.FreeSpace) / $_.Size) * 100) } else { 0 }
  "$($_.DeviceID) $size $used $free $pct"
}
`)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{"raw": out}
	var filesystems []map[string]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			filesystems = append(filesystems, map[string]string{
				"filesystem":  fields[0],
				"size":        fields[1] + "G",
				"used":        fields[2] + "G",
				"avail":       fields[3] + "G",
				"use_percent": fields[4],
				"mounted_on":  fields[0] + "\\",
			})
		}
	}
	result["filesystems"] = filesystems
	return result, nil
}

// Processes returns top processes and raw sample.
func Processes() (processes []map[string]string, raw string, err error) {
	switch runtime.GOOS {
	case "windows":
		return processesWindows()
	case "darwin":
		return processesDarwin()
	default:
		return processesLinux()
	}
}

func processesDarwin() ([]map[string]string, string, error) {
	out, err := runCommand("ps", "aux")
	if err != nil {
		return nil, "", err
	}
	processes := parsePsOutput(out)
	if len(processes) > 20 {
		processes = processes[:20]
	}
	return processes, firstLines(out, 22), nil
}

func processesLinux() ([]map[string]string, string, error) {
	out, err := runCommand("ps", "aux", "--sort=-%cpu")
	if err != nil {
		out, err = runCommand("ps", "aux")
		if err != nil {
			return nil, "", err
		}
	}
	processes := parsePsOutput(out)
	if len(processes) > 20 {
		processes = processes[:20]
	}
	return processes, firstLines(out, 22), nil
}

func processesWindows() ([]map[string]string, string, error) {
	out, err := powershell(`
Get-Process | Sort-Object CPU -Descending | Select-Object -First 20 | ForEach-Object {
  $cpu = if ($_.CPU) { [math]::Round($_.CPU, 1) } else { 0 }
  $mem = [math]::Round($_.WorkingSet64 / 1MB, 1)
  "$($_.Id) $cpu $mem $($_.ProcessName)"
}
`)
	if err != nil {
		return nil, "", err
	}
	var processes []map[string]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			processes = append(processes, map[string]string{
				"user":    "-",
				"pid":     fields[0],
				"cpu":     fields[1],
				"mem":     fields[2],
				"command": strings.Join(fields[3:], " "),
			})
		}
	}
	return processes, firstLines(out, 22), nil
}

func parseTopOutput(out string, goos string) (cpuUsage int, topProcess string) {
	lines := strings.Split(out, "\n")
	if goos == "darwin" {
		cpuRe := regexp.MustCompile(`CPU usage:\s*([\d.]+)\s*%\s*user`)
		for _, line := range lines {
			if m := cpuRe.FindStringSubmatch(line); len(m) > 1 {
				var f float64
				_, _ = fmt.Sscanf(m[1], "%f", &f)
				cpuUsage = int(f)
				break
			}
		}
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "PID") || strings.HasPrefix(trimmed, "CPU usage") || strings.HasPrefix(trimmed, "Processes") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				topProcess = fields[1]
				if len(fields) > 2 {
					topProcess = strings.Join(fields[1:], " ")
				}
				if len(topProcess) > 50 {
					topProcess = topProcess[:50] + "..."
				}
				break
			}
		}
	} else {
		cpuRe := regexp.MustCompile(`%Cpu\(s\):\s*([\d.]+)\s+us`)
		for _, line := range lines {
			if m := cpuRe.FindStringSubmatch(line); len(m) > 1 {
				var f float64
				_, _ = fmt.Sscanf(m[1], "%f", &f)
				cpuUsage = int(f)
				break
			}
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "  PID") || strings.TrimSpace(line) == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 11 {
				topProcess = strings.Join(fields[10:], " ")
				if len(topProcess) > 50 {
					topProcess = topProcess[:50] + "..."
				}
				break
			}
		}
	}
	if topProcess == "" {
		topProcess = "unknown"
	}
	return cpuUsage, topProcess
}

func parseFreeOutput(out string) map[string]interface{} {
	lines := strings.Split(out, "\n")
	result := map[string]interface{}{"raw": out}
	if len(lines) < 2 {
		return result
	}
	var total, used, free, shared, buffCache, available int
	_, _ = fmt.Sscanf(lines[1], "Mem:%d %d %d %d %d %d",
		&total, &used, &free, &shared, &buffCache, &available)
	result["total_mb"] = total
	result["used_mb"] = used
	result["free_mb"] = free
	result["available_mb"] = available
	if total > 0 {
		result["usage_percent"] = (used * 100) / total
	}
	return result
}

func parseVMStatOutput(out string) map[string]interface{} {
	lines := strings.Split(out, "\n")
	result := map[string]interface{}{"raw": out}
	getPages := func(key string) int64 {
		for _, line := range lines {
			if strings.Contains(line, key) {
				parts := strings.Split(line, ":")
				if len(parts) < 2 {
					return 0
				}
				var n int64
				fmt.Sscanf(strings.TrimSpace(strings.TrimRight(parts[1], ".")), "%d", &n)
				return n
			}
		}
		return 0
	}
	free := getPages("Pages free") + getPages("Pages speculative")
	active := getPages("Pages active")
	inactive := getPages("Pages inactive")
	wired := getPages("Pages wired")
	compressed := getPages("Pages occupied by compressor")
	used := active + inactive + wired + compressed
	totalMB := (free + used) / 256
	usedMB := used / 256
	freeMB := free / 256
	result["total_mb"] = totalMB
	result["used_mb"] = usedMB
	result["free_mb"] = freeMB
	result["available_mb"] = freeMB
	if totalMB > 0 {
		result["usage_percent"] = int((usedMB * 100) / totalMB)
	}
	return result
}

func parseDfOutput(out string) map[string]interface{} {
	lines := strings.Split(out, "\n")
	result := map[string]interface{}{"raw": out}
	var filesystems []map[string]string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			fs := map[string]string{
				"filesystem":   fields[0],
				"size":         fields[1],
				"used":         fields[2],
				"avail":        fields[3],
				"use_percent":  strings.TrimSuffix(fields[4], "%"),
				"mounted_on":   fields[5],
			}
			filesystems = append(filesystems, fs)
		}
	}
	result["filesystems"] = filesystems
	return result
}

func parsePsOutput(out string) []map[string]string {
	lines := strings.Split(out, "\n")
	var processes []map[string]string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 11 {
			processes = append(processes, map[string]string{
				"user":    fields[0],
				"pid":     fields[1],
				"cpu":     fields[2],
				"mem":     fields[3],
				"command": strings.Join(fields[10:], " "),
			})
		}
	}
	return processes
}
