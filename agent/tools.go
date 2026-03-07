package agent

import (
	"fmt"

	"github.com/josephpaul/opsagent-ai/internal/diagnostics"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// checkCPUArgs is empty; the tool takes no arguments.
type checkCPUArgs struct{}

// checkCPUResult is returned by check_cpu.
type checkCPUResult struct {
	CPUUsage   int    `json:"cpu_usage" jsonschema:"CPU usage percentage (user)"`
	TopProcess string `json:"top_process" jsonschema:"Name of the top CPU-consuming process"`
	RawSample  string `json:"raw_sample,omitempty" jsonschema:"First 20 lines of top output"`
	Error      string `json:"error,omitempty" jsonschema:"Error message if the check failed"`
}

func checkCPU(ctx tool.Context, input checkCPUArgs) (checkCPUResult, error) {
	cpuUsage, topProcess, raw, err := diagnostics.CPU()
	if err != nil {
		return checkCPUResult{Error: err.Error()}, nil
	}
	return checkCPUResult{
		CPUUsage:   cpuUsage,
		TopProcess: topProcess,
		RawSample:  raw,
	}, nil
}

// checkMemoryArgs is empty.
type checkMemoryArgs struct{}

// checkMemoryResult holds memory stats for the LLM.
type checkMemoryResult struct {
	TotalMB      interface{} `json:"total_mb,omitempty" jsonschema:"Total memory in MB"`
	UsedMB       interface{} `json:"used_mb,omitempty" jsonschema:"Used memory in MB"`
	FreeMB       interface{} `json:"free_mb,omitempty" jsonschema:"Free memory in MB"`
	AvailableMB  interface{} `json:"available_mb,omitempty" jsonschema:"Available memory in MB"`
	UsagePercent interface{} `json:"usage_percent,omitempty" jsonschema:"Memory usage percentage"`
	Error        string      `json:"error,omitempty" jsonschema:"Error message if the check failed"`
}

func checkMemory(ctx tool.Context, input checkMemoryArgs) (checkMemoryResult, error) {
	m, err := diagnostics.Memory()
	if err != nil {
		return checkMemoryResult{Error: err.Error()}, nil
	}
	return checkMemoryResult{
		TotalMB:      m["total_mb"],
		UsedMB:       m["used_mb"],
		FreeMB:       m["free_mb"],
		AvailableMB:  m["available_mb"],
		UsagePercent: m["usage_percent"],
	}, nil
}

// checkDiskArgs is empty.
type checkDiskArgs struct{}

// checkDiskResult holds disk info for the LLM.
type checkDiskResult struct {
	Filesystems interface{} `json:"filesystems,omitempty" jsonschema:"List of filesystems with size, used, avail, use_percent, mounted_on"`
	Error       string      `json:"error,omitempty" jsonschema:"Error message if the check failed"`
}

func checkDisk(ctx tool.Context, input checkDiskArgs) (checkDiskResult, error) {
	m, err := diagnostics.Disk()
	if err != nil {
		return checkDiskResult{Error: err.Error()}, nil
	}
	return checkDiskResult{
		Filesystems: m["filesystems"],
	}, nil
}

// checkProcessesArgs is empty.
type checkProcessesArgs struct{}

// checkProcessesResult holds top processes for the LLM.
type checkProcessesResult struct {
	Processes []map[string]string `json:"processes" jsonschema:"List of processes with user, pid, cpu, mem, command"`
	Error     string              `json:"error,omitempty" jsonschema:"Error message if the check failed"`
}

func checkProcesses(ctx tool.Context, input checkProcessesArgs) (checkProcessesResult, error) {
	processes, _, err := diagnostics.Processes()
	if err != nil {
		return checkProcessesResult{Error: err.Error()}, nil
	}
	return checkProcessesResult{Processes: processes}, nil
}

// NewDiagnosticTools returns the four diagnostic tools for the agent.
func NewDiagnosticTools() ([]tool.Tool, error) {
	cpuTool, err := functiontool.New(functiontool.Config{
		Name:        "check_cpu",
		Description: "Check CPU usage and the top process on this machine. Returns cpu_usage (int), top_process (string), and a raw sample of top output.",
	}, checkCPU)
	if err != nil {
		return nil, fmt.Errorf("check_cpu tool: %w", err)
	}
	memTool, err := functiontool.New(functiontool.Config{
		Name:        "check_memory",
		Description: "Check memory usage (total, used, free, available in MB and usage_percent).",
	}, checkMemory)
	if err != nil {
		return nil, fmt.Errorf("check_memory tool: %w", err)
	}
	diskTool, err := functiontool.New(functiontool.Config{
		Name:        "check_disk",
		Description: "Check disk usage per filesystem (size, used, avail, use_percent, mounted_on).",
	}, checkDisk)
	if err != nil {
		return nil, fmt.Errorf("check_disk tool: %w", err)
	}
	procTool, err := functiontool.New(functiontool.Config{
		Name:        "check_processes",
		Description: "Check top processes by CPU (user, pid, cpu% , mem%, command).",
	}, checkProcesses)
	if err != nil {
		return nil, fmt.Errorf("check_processes tool: %w", err)
	}
	return []tool.Tool{cpuTool, memTool, diskTool, procTool}, nil
}
