// Package main runs the opsagent-ai server agent that exposes system diagnostic endpoints.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

const port = "8080"

func main() {
	http.HandleFunc("/cpu", handleCPU)
	http.HandleFunc("/memory", handleMemory)
	http.HandleFunc("/disk", handleDisk)
	http.HandleFunc("/processes", handleProcesses)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Server agent listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

// runCommand executes a shell command and returns stdout and error.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exec %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleCPU(w http.ResponseWriter, _ *http.Request) {
	var out string
	var err error
	if runtime.GOOS == "darwin" {
		out, err = runCommand("top", "-l", "1")
	} else {
		out, err = runCommand("top", "-bn1")
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	cpuUsage, topProcess := parseTopOutput(out, runtime.GOOS)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"cpu_usage":   cpuUsage,
		"top_process": topProcess,
		"raw_sample":  firstLines(out, 20),
	})
}

func handleMemory(w http.ResponseWriter, _ *http.Request) {
	var mem map[string]interface{}
	if runtime.GOOS == "darwin" {
		out, err := runCommand("vm_stat")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err)
			return
		}
		mem = parseVMStatOutput(out)
	} else {
		out, err := runCommand("free", "-m")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err)
			return
		}
		mem = parseFreeOutput(out)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mem)
}

func handleDisk(w http.ResponseWriter, _ *http.Request) {
	out, err := runCommand("df", "-h")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	disk := parseDfOutput(out)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(disk)
}

func handleProcesses(w http.ResponseWriter, _ *http.Request) {
	// Linux: ps aux --sort=-%cpu; macOS: ps aux (no --sort), we sort in Go
	var out string
	var err error
	if runtime.GOOS == "darwin" {
		out, err = runCommand("ps", "aux")
	} else {
		out, err = runCommand("sh", "-c", "ps aux --sort=-%cpu 2>/dev/null | head -21")
		if err != nil {
			out, err = runCommand("sh", "-c", "ps aux | head -21")
		}
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	processes := parsePsOutput(out)
	if runtime.GOOS == "darwin" && len(processes) > 21 {
		processes = processes[:21]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"processes":  processes,
		"raw_sample": firstLines(out, 22),
	})
}

func writeJSONError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// parseTopOutput extracts CPU usage and top process from top output.
// goos: "linux" for top -bn1, "darwin" for top -l 1 (macOS).
func parseTopOutput(out string, goos string) (cpuUsage int, topProcess string) {
	lines := strings.Split(out, "\n")
	if goos == "darwin" {
		// macOS: "CPU usage: 12.3% user, 4.5% sys, 83.1% idle"
		cpuRe := regexp.MustCompile(`CPU usage:\s*([\d.]+)\s*%\s*user`)
		for _, line := range lines {
			if m := cpuRe.FindStringSubmatch(line); len(m) > 1 {
				var f float64
				_, _ = fmt.Sscanf(m[1], "%f", &f)
				cpuUsage = int(f)
				break
			}
		}
		// macOS top -l 1: process lines are "  PID COMMAND %CPU ..."; COMMAND is typically field 1
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "PID") || strings.HasPrefix(trimmed, "CPU usage") || strings.HasPrefix(trimmed, "Processes") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// First numeric field is PID; next is COMMAND
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
		// Linux: %Cpu(s): 12.3 us, ...
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
	result := map[string]interface{}{
		"raw": out,
	}
	if len(lines) < 2 {
		return result
	}
	// Mem: total used free shared buff/cache available
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

// parseVMStatOutput parses macOS vm_stat output into free-form memory stats.
// Page size is 4096 bytes; 256 pages = 1 MB.
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
	totalMB := (free + used) / 256 // 4096*256 = 1MB per 256 pages
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
	result := map[string]interface{}{
		"raw": out,
	}
	var filesystems []map[string]string
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			fs := map[string]string{
				"filesystem": fields[0],
				"size":       fields[1],
				"used":       fields[2],
				"avail":      fields[3],
				"use_percent": strings.TrimSuffix(fields[4], "%"),
				"mounted_on": fields[5],
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
				"user":   fields[0],
				"pid":    fields[1],
				"cpu":    fields[2],
				"mem":    fields[3],
				"command": strings.Join(fields[10:], " "),
			})
		}
	}
	return processes
}
