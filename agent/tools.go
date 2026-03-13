package agent

import (
	"fmt"

	"github.com/josephpaul/opsagent/internal/diagnostics"
	"github.com/josephpaul/opsagent/internal/docker"
	"github.com/josephpaul/opsagent/internal/nginx"
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

type listDockerProjectsArgs struct{}

type listDockerProjectsResult struct {
	Projects []docker.ProjectSummary `json:"projects,omitempty" jsonschema:"Running Docker Compose projects with grouped service and health summary"`
	Error    string                  `json:"error,omitempty" jsonschema:"Error message if the Docker project listing failed"`
}

func listDockerProjects(ctx tool.Context, input listDockerProjectsArgs) (listDockerProjectsResult, error) {
	projects, err := docker.ListProjects()
	if err != nil {
		return listDockerProjectsResult{Error: err.Error()}, nil
	}
	return listDockerProjectsResult{Projects: projects}, nil
}

type inspectDockerProjectArgs struct {
	Project string `json:"project" jsonschema:"Docker Compose project name to inspect"`
}

type inspectDockerProjectResult struct {
	Project *docker.ProjectDetail `json:"project,omitempty" jsonschema:"Detailed Docker Compose project information including services, containers, networks, volumes, and images"`
	Error   string                `json:"error,omitempty" jsonschema:"Error message if the project inspection failed"`
}

func inspectDockerProject(ctx tool.Context, input inspectDockerProjectArgs) (inspectDockerProjectResult, error) {
	project, err := docker.InspectProject(input.Project)
	if err != nil {
		return inspectDockerProjectResult{Error: err.Error()}, nil
	}
	return inspectDockerProjectResult{Project: project}, nil
}

type dockerContainerStatsArgs struct {
	Project   string `json:"project" jsonschema:"Docker Compose project name"`
	Service   string `json:"service,omitempty" jsonschema:"Optional Docker Compose service name to narrow the stats lookup"`
	Container string `json:"container,omitempty" jsonschema:"Optional container name or ID to narrow the stats lookup"`
}

type dockerContainerStatsResult struct {
	Stats []docker.ContainerStat `json:"stats,omitempty" jsonschema:"Point-in-time Docker container stats including CPU, memory, network, block IO, and PIDs"`
	Error string                 `json:"error,omitempty" jsonschema:"Error message if fetching container stats failed"`
}

func dockerContainerStats(ctx tool.Context, input dockerContainerStatsArgs) (dockerContainerStatsResult, error) {
	stats, err := docker.ContainerStats(input.Project, input.Service, input.Container)
	if err != nil {
		return dockerContainerStatsResult{Error: err.Error()}, nil
	}
	return dockerContainerStatsResult{Stats: stats}, nil
}

type dockerContainerLogsArgs struct {
	Project   string `json:"project" jsonschema:"Docker Compose project name"`
	Service   string `json:"service,omitempty" jsonschema:"Optional Docker Compose service name to narrow log lookup"`
	Container string `json:"container,omitempty" jsonschema:"Optional container name or ID to narrow log lookup"`
	Tail      int    `json:"tail,omitempty" jsonschema:"Optional number of recent log lines per matched container; defaults to 50 and is capped at 200"`
}

type dockerContainerLogsResult struct {
	Logs  []docker.ContainerLog `json:"logs,omitempty" jsonschema:"Recent bounded logs for one or more containers in a Docker Compose project"`
	Error string                `json:"error,omitempty" jsonschema:"Error message if log retrieval failed"`
}

func dockerContainerLogs(ctx tool.Context, input dockerContainerLogsArgs) (dockerContainerLogsResult, error) {
	logs, err := docker.ContainerLogs(input.Project, input.Service, input.Container, input.Tail)
	if err != nil {
		return dockerContainerLogsResult{Error: err.Error()}, nil
	}
	return dockerContainerLogsResult{Logs: logs}, nil
}

type listNginxSitesArgs struct{}

type listNginxSitesResult struct {
	Sites []nginx.SiteSummary `json:"sites,omitempty" jsonschema:"Nginx server blocks with names, listen directives, proxy targets, and log files"`
	Error string              `json:"error,omitempty" jsonschema:"Error message if the Nginx site listing failed"`
}

func listNginxSites(ctx tool.Context, input listNginxSitesArgs) (listNginxSitesResult, error) {
	sites, err := nginx.ListSites()
	if err != nil {
		return listNginxSitesResult{Error: err.Error()}, nil
	}
	return listNginxSitesResult{Sites: sites}, nil
}

type inspectNginxSiteArgs struct {
	Site string `json:"site" jsonschema:"Nginx site name, server_name, or config filename to inspect"`
}

type inspectNginxSiteResult struct {
	Site  *nginx.SiteDetail `json:"site,omitempty" jsonschema:"Detailed Nginx site information including server names, listen directives, roots, proxy targets, and log files"`
	Error string            `json:"error,omitempty" jsonschema:"Error message if the Nginx site inspection failed"`
}

func inspectNginxSite(ctx tool.Context, input inspectNginxSiteArgs) (inspectNginxSiteResult, error) {
	site, err := nginx.InspectSite(input.Site)
	if err != nil {
		return inspectNginxSiteResult{Error: err.Error()}, nil
	}
	return inspectNginxSiteResult{Site: site}, nil
}

type inspectNginxRuntimeArgs struct{}

type inspectNginxRuntimeResult struct {
	Runtime *nginx.RuntimeInfo `json:"runtime,omitempty" jsonschema:"Nginx runtime and configuration summary including version, config test result, process state, config files, sites, and upstreams"`
	Error   string             `json:"error,omitempty" jsonschema:"Error message if Nginx runtime inspection failed"`
}

func inspectNginxRuntime(ctx tool.Context, input inspectNginxRuntimeArgs) (inspectNginxRuntimeResult, error) {
	runtime, err := nginx.RuntimeStatus()
	if err != nil {
		return inspectNginxRuntimeResult{Error: err.Error()}, nil
	}
	return inspectNginxRuntimeResult{Runtime: runtime}, nil
}

type nginxLogSampleArgs struct {
	Site string `json:"site,omitempty" jsonschema:"Optional Nginx site name or server_name to narrow log lookup"`
	Kind string `json:"kind,omitempty" jsonschema:"Log kind: error or access. Defaults to error"`
	Tail int    `json:"tail,omitempty" jsonschema:"Optional number of recent log lines per matched file; defaults to 50 and is capped at 200"`
}

type nginxLogSampleResult struct {
	Logs  []nginx.LogSample `json:"logs,omitempty" jsonschema:"Bounded recent Nginx access or error log samples"`
	Error string            `json:"error,omitempty" jsonschema:"Error message if retrieving Nginx log samples failed"`
}

func nginxLogSample(ctx tool.Context, input nginxLogSampleArgs) (nginxLogSampleResult, error) {
	logs, err := nginx.LogSamples(input.Site, input.Kind, input.Tail)
	if err != nil {
		return nginxLogSampleResult{Error: err.Error()}, nil
	}
	return nginxLogSampleResult{Logs: logs}, nil
}

// NewDiagnosticTools returns the host diagnostic and Docker inspection tools for the agent.
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

	dockerProjectsTool, err := functiontool.New(functiontool.Config{
		Name:        "list_docker_projects",
		Description: "List running Docker Compose projects on this machine, including grouped services, running container counts, and basic health summary.",
	}, listDockerProjects)
	if err != nil {
		return nil, fmt.Errorf("list_docker_projects tool: %w", err)
	}
	dockerProjectTool, err := functiontool.New(functiontool.Config{
		Name:        "inspect_docker_project",
		Description: "Inspect one Docker Compose project in detail, including service membership, containers, images, ports, networks, mounts, and health/state.",
	}, inspectDockerProject)
	if err != nil {
		return nil, fmt.Errorf("inspect_docker_project tool: %w", err)
	}
	dockerStatsTool, err := functiontool.New(functiontool.Config{
		Name:        "docker_container_stats",
		Description: "Get current Docker container stats for a Docker Compose project. Optionally narrow by service or container.",
	}, dockerContainerStats)
	if err != nil {
		return nil, fmt.Errorf("docker_container_stats tool: %w", err)
	}
	dockerLogsTool, err := functiontool.New(functiontool.Config{
		Name:        "docker_container_logs",
		Description: "Get bounded recent logs for containers in a Docker Compose project. Optionally narrow by service or container.",
	}, dockerContainerLogs)
	if err != nil {
		return nil, fmt.Errorf("docker_container_logs tool: %w", err)
	}
	nginxSitesTool, err := functiontool.New(functiontool.Config{
		Name:        "list_nginx_sites",
		Description: "List Nginx server blocks with names, listen directives, proxy targets, and configured log files.",
	}, listNginxSites)
	if err != nil {
		return nil, fmt.Errorf("list_nginx_sites tool: %w", err)
	}
	nginxSiteTool, err := functiontool.New(functiontool.Config{
		Name:        "inspect_nginx_site",
		Description: "Inspect one Nginx site by server_name or config filename, including roots, listen directives, proxy targets, and log files.",
	}, inspectNginxSite)
	if err != nil {
		return nil, fmt.Errorf("inspect_nginx_site tool: %w", err)
	}
	nginxRuntimeTool, err := functiontool.New(functiontool.Config{
		Name:        "inspect_nginx_runtime",
		Description: "Inspect Nginx runtime and configuration state, including version, config test result, running master/worker processes, loaded config files, and upstreams.",
	}, inspectNginxRuntime)
	if err != nil {
		return nil, fmt.Errorf("inspect_nginx_runtime tool: %w", err)
	}
	nginxLogsTool, err := functiontool.New(functiontool.Config{
		Name:        "nginx_log_sample",
		Description: "Get bounded recent Nginx access or error log samples, optionally narrowed to a specific site.",
	}, nginxLogSample)
	if err != nil {
		return nil, fmt.Errorf("nginx_log_sample tool: %w", err)
	}

	return []tool.Tool{
		cpuTool,
		memTool,
		diskTool,
		procTool,
		dockerProjectsTool,
		dockerProjectTool,
		dockerStatsTool,
		dockerLogsTool,
		nginxSitesTool,
		nginxSiteTool,
		nginxRuntimeTool,
		nginxLogsTool,
	}, nil
}
