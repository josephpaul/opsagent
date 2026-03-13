package docker

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

const (
	labelProject = "com.docker.compose.project"
	labelService = "com.docker.compose.service"
)

type ProjectSummary struct {
	Project        string   `json:"project"`
	Services       []string `json:"services"`
	RunningCount   int      `json:"running_count"`
	TotalCount     int      `json:"total_count"`
	HealthyCount   int      `json:"healthy_count"`
	UnhealthyCount int      `json:"unhealthy_count"`
	Status         string   `json:"status"`
	ContainerNames []string `json:"container_names"`
}

type ProjectDetail struct {
	Project        string          `json:"project"`
	Services       []ServiceDetail `json:"services"`
	Containers     []ContainerInfo `json:"containers"`
	RunningCount   int             `json:"running_count"`
	TotalCount     int             `json:"total_count"`
	HealthyCount   int             `json:"healthy_count"`
	UnhealthyCount int             `json:"unhealthy_count"`
	Networks       []string        `json:"networks"`
	Volumes        []string        `json:"volumes"`
	Images         []string        `json:"images"`
}

type ServiceDetail struct {
	Name         string          `json:"name"`
	RunningCount int             `json:"running_count"`
	TotalCount   int             `json:"total_count"`
	Containers   []ContainerInfo `json:"containers"`
}

type ContainerInfo struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Service    string      `json:"service"`
	Image      string      `json:"image"`
	Command    string      `json:"command,omitempty"`
	State      string      `json:"state"`
	Status     string      `json:"status"`
	Health     string      `json:"health,omitempty"`
	ExitCode   int         `json:"exit_code,omitempty"`
	CreatedAt  string      `json:"created_at,omitempty"`
	StartedAt  string      `json:"started_at,omitempty"`
	FinishedAt string      `json:"finished_at,omitempty"`
	Ports      []string    `json:"ports,omitempty"`
	Networks   []string    `json:"networks,omitempty"`
	Mounts     []MountInfo `json:"mounts,omitempty"`
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadWrite   bool   `json:"read_write"`
}

type ContainerStat struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Service    string `json:"service"`
	Project    string `json:"project"`
	CPUPercent string `json:"cpu_percent"`
	MemUsage   string `json:"mem_usage"`
	MemPercent string `json:"mem_percent"`
	NetIO      string `json:"net_io"`
	BlockIO    string `json:"block_io"`
	PIDs       string `json:"pids"`
}

type ContainerLog struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Service string `json:"service"`
	Project string `json:"project"`
	Tail    int    `json:"tail"`
	Logs    string `json:"logs,omitempty"`
	Error   string `json:"error,omitempty"`
}

type inspectContainer struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	Config  struct {
		Image  string            `json:"Image"`
		Cmd    []string          `json:"Cmd"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
		Networks map[string]struct{} `json:"Networks"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

type statsRow struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	NetIO    string `json:"NetIO"`
	BlockIO  string `json:"BlockIO"`
	PIDs     string `json:"PIDs"`
}

func ListProjects() ([]ProjectSummary, error) {
	containers, err := composeContainers("")
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return []ProjectSummary{}, nil
	}

	projectMap := make(map[string][]inspectContainer)
	for _, c := range containers {
		project := c.Config.Labels[labelProject]
		if project == "" {
			continue
		}
		projectMap[project] = append(projectMap[project], c)
	}

	projects := make([]string, 0, len(projectMap))
	for project := range projectMap {
		projects = append(projects, project)
	}
	sort.Strings(projects)

	out := make([]ProjectSummary, 0, len(projects))
	for _, project := range projects {
		summary := summarizeProject(project, projectMap[project])
		if summary.RunningCount == 0 {
			continue
		}
		out = append(out, summary)
	}
	return out, nil
}

func InspectProject(project string) (*ProjectDetail, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}

	containers, err := composeContainers(project)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("docker compose project %q not found", project)
	}

	return buildProjectDetail(project, containers), nil
}

func ContainerStats(project, service, container string) ([]ContainerStat, error) {
	matched, err := selectContainers(project, service, container)
	if err != nil {
		return nil, err
	}
	if len(matched) == 0 {
		return []ContainerStat{}, nil
	}

	args := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	for _, c := range matched {
		args = append(args, trimmedName(c))
	}

	out, err := runDocker(args...)
	if err != nil {
		return nil, err
	}

	statsByName := make(map[string]statsRow)
	statsByID := make(map[string]statsRow)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row statsRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker stats output: %w", err)
		}
		statsByName[row.Name] = row
		statsByID[row.ID] = row
	}

	result := make([]ContainerStat, 0, len(matched))
	for _, c := range matched {
		name := trimmedName(c)
		row, ok := statsByName[name]
		if !ok {
			row, ok = statsByID[c.ID]
		}
		if !ok {
			continue
		}
		result = append(result, ContainerStat{
			ID:         c.ID,
			Name:       name,
			Service:    c.Config.Labels[labelService],
			Project:    c.Config.Labels[labelProject],
			CPUPercent: row.CPUPerc,
			MemUsage:   row.MemUsage,
			MemPercent: row.MemPerc,
			NetIO:      row.NetIO,
			BlockIO:    row.BlockIO,
			PIDs:       row.PIDs,
		})
	}
	return result, nil
}

func ContainerLogs(project, service, container string, tail int) ([]ContainerLog, error) {
	if tail <= 0 {
		tail = 50
	}
	if tail > 200 {
		tail = 200
	}

	matched, err := selectContainers(project, service, container)
	if err != nil {
		return nil, err
	}
	if len(matched) == 0 {
		return []ContainerLog{}, nil
	}

	result := make([]ContainerLog, 0, len(matched))
	for _, c := range matched {
		name := trimmedName(c)
		out, err := runDocker("logs", "--tail", fmt.Sprintf("%d", tail), "--timestamps", name)
		entry := ContainerLog{
			ID:      c.ID,
			Name:    name,
			Service: c.Config.Labels[labelService],
			Project: c.Config.Labels[labelProject],
			Tail:    tail,
		}
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Logs = out
		}
		result = append(result, entry)
	}
	return result, nil
}

func composeContainers(project string) ([]inspectContainer, error) {
	args := []string{"ps", "-a", "--filter", "label=" + labelProject, "--format", "{{.ID}}"}
	if project != "" {
		args = []string{"ps", "-a", "--filter", "label=" + labelProject + "=" + project, "--format", "{{.ID}}"}
	}
	out, err := runDocker(args...)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	if len(ids) == 0 {
		return []inspectContainer{}, nil
	}

	inspectArgs := append([]string{"inspect"}, ids...)
	inspectOut, err := runDocker(inspectArgs...)
	if err != nil {
		return nil, err
	}

	var containers []inspectContainer
	if err := json.Unmarshal([]byte(inspectOut), &containers); err != nil {
		return nil, fmt.Errorf("parse docker inspect output: %w", err)
	}
	return containers, nil
}

func selectContainers(project, service, container string) ([]inspectContainer, error) {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	container = strings.TrimSpace(container)

	if project == "" {
		return nil, fmt.Errorf("project is required")
	}

	containers, err := composeContainers(project)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("docker compose project %q not found", project)
	}

	var matched []inspectContainer
	for _, c := range containers {
		if service != "" && c.Config.Labels[labelService] != service {
			continue
		}
		if container != "" && !matchesContainer(c, container) {
			continue
		}
		matched = append(matched, c)
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("no containers matched project=%q service=%q container=%q", project, service, container)
	}

	sort.Slice(matched, func(i, j int) bool {
		return trimmedName(matched[i]) < trimmedName(matched[j])
	})
	return matched, nil
}

func summarizeProject(project string, containers []inspectContainer) ProjectSummary {
	serviceSet := make(map[string]struct{})
	containerNames := make([]string, 0, len(containers))
	var runningCount, healthyCount, unhealthyCount int

	for _, c := range containers {
		if service := c.Config.Labels[labelService]; service != "" {
			serviceSet[service] = struct{}{}
		}
		containerNames = append(containerNames, trimmedName(c))
		if c.State.Running {
			runningCount++
		}
		switch healthStatus(c) {
		case "healthy":
			healthyCount++
		case "unhealthy":
			unhealthyCount++
		}
	}

	services := setToSortedList(serviceSet)
	sort.Strings(containerNames)
	return ProjectSummary{
		Project:        project,
		Services:       services,
		RunningCount:   runningCount,
		TotalCount:     len(containers),
		HealthyCount:   healthyCount,
		UnhealthyCount: unhealthyCount,
		Status:         projectStatus(len(containers), runningCount, unhealthyCount),
		ContainerNames: containerNames,
	}
}

func buildProjectDetail(project string, containers []inspectContainer) *ProjectDetail {
	serviceMap := make(map[string][]inspectContainer)
	networkSet := make(map[string]struct{})
	volumeSet := make(map[string]struct{})
	imageSet := make(map[string]struct{})

	var infos []ContainerInfo
	var runningCount, healthyCount, unhealthyCount int

	for _, c := range containers {
		service := c.Config.Labels[labelService]
		serviceMap[service] = append(serviceMap[service], c)
		info := toContainerInfo(c)
		infos = append(infos, info)

		if c.State.Running {
			runningCount++
		}
		switch info.Health {
		case "healthy":
			healthyCount++
		case "unhealthy":
			unhealthyCount++
		}
		if info.Image != "" {
			imageSet[info.Image] = struct{}{}
		}
		for _, network := range info.Networks {
			networkSet[network] = struct{}{}
		}
		for _, mount := range info.Mounts {
			if mount.Source != "" {
				volumeSet[mount.Source] = struct{}{}
			}
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	serviceNames := make([]string, 0, len(serviceMap))
	for service := range serviceMap {
		serviceNames = append(serviceNames, service)
	}
	sort.Strings(serviceNames)

	services := make([]ServiceDetail, 0, len(serviceNames))
	for _, service := range serviceNames {
		containersForService := serviceMap[service]
		serviceInfos := make([]ContainerInfo, 0, len(containersForService))
		running := 0
		for _, c := range containersForService {
			info := toContainerInfo(c)
			if c.State.Running {
				running++
			}
			serviceInfos = append(serviceInfos, info)
		}
		sort.Slice(serviceInfos, func(i, j int) bool {
			return serviceInfos[i].Name < serviceInfos[j].Name
		})
		services = append(services, ServiceDetail{
			Name:         service,
			RunningCount: running,
			TotalCount:   len(serviceInfos),
			Containers:   serviceInfos,
		})
	}

	return &ProjectDetail{
		Project:        project,
		Services:       services,
		Containers:     infos,
		RunningCount:   runningCount,
		TotalCount:     len(containers),
		HealthyCount:   healthyCount,
		UnhealthyCount: unhealthyCount,
		Networks:       setToSortedList(networkSet),
		Volumes:        setToSortedList(volumeSet),
		Images:         setToSortedList(imageSet),
	}
}

func toContainerInfo(c inspectContainer) ContainerInfo {
	mounts := make([]MountInfo, 0, len(c.Mounts))
	for _, mount := range c.Mounts {
		mounts = append(mounts, MountInfo{
			Type:        mount.Type,
			Source:      mount.Source,
			Destination: mount.Destination,
			ReadWrite:   mount.RW,
		})
	}

	return ContainerInfo{
		ID:         c.ID,
		Name:       trimmedName(c),
		Service:    c.Config.Labels[labelService],
		Image:      c.Config.Image,
		Command:    strings.Join(c.Config.Cmd, " "),
		State:      c.State.Status,
		Status:     containerStatus(c),
		Health:     healthStatus(c),
		ExitCode:   c.State.ExitCode,
		CreatedAt:  c.Created,
		StartedAt:  c.State.StartedAt,
		FinishedAt: c.State.FinishedAt,
		Ports:      formatPorts(c.NetworkSettings.Ports),
		Networks:   networkNames(c.NetworkSettings.Networks),
		Mounts:     mounts,
	}
}

func containerStatus(c inspectContainer) string {
	status := strings.TrimSpace(c.State.Status)
	health := healthStatus(c)
	if health == "" || health == "unknown" {
		return status
	}
	if status == "" {
		return health
	}
	return status + " (" + health + ")"
}

func healthStatus(c inspectContainer) string {
	if c.State.Health == nil || strings.TrimSpace(c.State.Health.Status) == "" {
		return ""
	}
	return strings.TrimSpace(c.State.Health.Status)
}

func projectStatus(totalCount, runningCount, unhealthyCount int) string {
	switch {
	case totalCount == 0:
		return "not_found"
	case unhealthyCount > 0:
		return "unhealthy"
	case runningCount == totalCount:
		return "healthy"
	case runningCount > 0:
		return "degraded"
	default:
		return "stopped"
	}
}

func networkNames(networks map[string]struct{}) []string {
	names := make([]string, 0, len(networks))
	for name := range networks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func formatPorts(ports map[string][]struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) []string {
	var out []string
	for containerPort, bindings := range ports {
		if len(bindings) == 0 {
			out = append(out, containerPort)
			continue
		}
		for _, binding := range bindings {
			if binding.HostPort == "" {
				out = append(out, containerPort)
				continue
			}
			host := binding.HostIP
			if host == "" {
				host = "0.0.0.0"
			}
			out = append(out, fmt.Sprintf("%s:%s->%s", host, binding.HostPort, containerPort))
		}
	}
	sort.Strings(out)
	return out
}

func setToSortedList(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func matchesContainer(c inspectContainer, query string) bool {
	name := trimmedName(c)
	return name == query || c.ID == query || strings.HasPrefix(c.ID, query) || strings.Contains(name, query)
}

func trimmedName(c inspectContainer) string {
	return strings.TrimPrefix(c.Name, "/")
}

func runDocker(args ...string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("docker CLI not found in PATH")
	}

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("docker %s: %s", strings.Join(args, " "), text)
		}
		return "", fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return text, nil
}
