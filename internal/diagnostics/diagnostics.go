package diagnostics

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
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

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// CPU runs top and returns usage, top process, and raw sample.
func CPU() (cpuUsage int, topProcess string, raw string, err error) {
	var out string
	if runtime.GOOS == "darwin" {
		out, err = runCommand("top", "-l", "1")
	} else {
		out, err = runCommand("top", "-bn1")
	}
	if err != nil {
		return 0, "", "", err
	}
	cpuUsage, topProcess = parseTopOutput(out, runtime.GOOS)
	return cpuUsage, topProcess, firstLines(out, 20), nil
}

// Memory returns memory stats (total_mb, used_mb, free_mb, usage_percent, etc.).
func Memory() (map[string]interface{}, error) {
	if runtime.GOOS == "darwin" {
		out, err := runCommand("vm_stat")
		if err != nil {
			return nil, err
		}
		return parseVMStatOutput(out), nil
	}
	out, err := runCommand("free", "-m")
	if err != nil {
		return nil, err
	}
	return parseFreeOutput(out), nil
}

// Disk returns disk usage per filesystem.
func Disk() (map[string]interface{}, error) {
	out, err := runCommand("df", "-h")
	if err != nil {
		return nil, err
	}
	return parseDfOutput(out), nil
}

// Processes returns top processes and raw sample.
func Processes() (processes []map[string]string, raw string, err error) {
	var out string
	if runtime.GOOS == "darwin" {
		out, err = runCommand("ps", "aux")
	} else {
		out, err = runCommand("sh", "-c", "ps aux --sort=-%cpu 2>/dev/null | head -21")
		if err != nil {
			out, err = runCommand("sh", "-c", "ps aux | head -21")
		}
	}
	if err != nil {
		return nil, "", err
	}
	processes = parsePsOutput(out)
	if runtime.GOOS == "darwin" && len(processes) > 21 {
		processes = processes[:21]
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
