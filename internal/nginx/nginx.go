package nginx

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type SiteSummary struct {
	Name        string   `json:"name"`
	ServerNames []string `json:"server_names,omitempty"`
	Listens     []string `json:"listens,omitempty"`
	ProxyPasses []string `json:"proxy_passes,omitempty"`
	AccessLog   string   `json:"access_log,omitempty"`
	ErrorLog    string   `json:"error_log,omitempty"`
	File        string   `json:"file,omitempty"`
}

type SiteDetail struct {
	Name        string   `json:"name"`
	ServerNames []string `json:"server_names,omitempty"`
	Listens     []string `json:"listens,omitempty"`
	Roots       []string `json:"roots,omitempty"`
	ProxyPasses []string `json:"proxy_passes,omitempty"`
	AccessLog   string   `json:"access_log,omitempty"`
	ErrorLog    string   `json:"error_log,omitempty"`
	File        string   `json:"file,omitempty"`
}

type UpstreamInfo struct {
	Name    string   `json:"name"`
	Servers []string `json:"servers,omitempty"`
	File    string   `json:"file,omitempty"`
}

type RuntimeInfo struct {
	Version          string         `json:"version,omitempty"`
	BinaryPath       string         `json:"binary_path,omitempty"`
	ConfigPath       string         `json:"config_path,omitempty"`
	DefaultAccessLog string         `json:"default_access_log,omitempty"`
	DefaultErrorLog  string         `json:"default_error_log,omitempty"`
	ConfigTestOK     bool           `json:"config_test_ok"`
	ConfigTestOutput string         `json:"config_test_output,omitempty"`
	MasterRunning    bool           `json:"master_running"`
	WorkerCount      int            `json:"worker_count"`
	StubStatus       bool           `json:"stub_status"`
	ConfigFiles      []string       `json:"config_files,omitempty"`
	Sites            []SiteSummary  `json:"sites,omitempty"`
	Upstreams        []UpstreamInfo `json:"upstreams,omitempty"`
}

type LogSample struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Site    string `json:"site,omitempty"`
	Tail    int    `json:"tail"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

type snapshot struct {
	Version          string
	BinaryPath       string
	ConfigPath       string
	DefaultAccessLog string
	DefaultErrorLog  string
	ConfigTestOK     bool
	ConfigTestOutput string
	MasterRunning    bool
	WorkerCount      int
	StubStatus       bool
	ConfigFiles      []string
	Sites            []SiteDetail
	Upstreams        []UpstreamInfo
}

func ListSites() ([]SiteSummary, error) {
	snap, err := loadSnapshot()
	if err != nil {
		return nil, err
	}

	out := make([]SiteSummary, 0, len(snap.Sites))
	for _, site := range snap.Sites {
		out = append(out, SiteSummary{
			Name:        site.Name,
			ServerNames: site.ServerNames,
			Listens:     site.Listens,
			ProxyPasses: site.ProxyPasses,
			AccessLog:   site.AccessLog,
			ErrorLog:    site.ErrorLog,
			File:        site.File,
		})
	}
	return out, nil
}

func InspectSite(name string) (*SiteDetail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("site is required")
	}

	snap, err := loadSnapshot()
	if err != nil {
		return nil, err
	}

	for _, site := range snap.Sites {
		if matchesSite(site, name) {
			siteCopy := site
			return &siteCopy, nil
		}
	}
	return nil, fmt.Errorf("nginx site %q not found", name)
}

func RuntimeStatus() (*RuntimeInfo, error) {
	snap, err := loadSnapshot()
	if err != nil {
		return nil, err
	}
	return &RuntimeInfo{
		Version:          snap.Version,
		BinaryPath:       snap.BinaryPath,
		ConfigPath:       snap.ConfigPath,
		DefaultAccessLog: snap.DefaultAccessLog,
		DefaultErrorLog:  snap.DefaultErrorLog,
		ConfigTestOK:     snap.ConfigTestOK,
		ConfigTestOutput: snap.ConfigTestOutput,
		MasterRunning:    snap.MasterRunning,
		WorkerCount:      snap.WorkerCount,
		StubStatus:       snap.StubStatus,
		ConfigFiles:      snap.ConfigFiles,
		Sites:            summarizeSites(snap.Sites),
		Upstreams:        snap.Upstreams,
	}, nil
}

func LogSamples(siteName, kind string, tail int) ([]LogSample, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		kind = "error"
	}
	if kind != "error" && kind != "access" {
		return nil, fmt.Errorf("kind must be error or access")
	}
	if tail <= 0 {
		tail = 50
	}
	if tail > 200 {
		tail = 200
	}

	snap, err := loadSnapshot()
	if err != nil {
		return nil, err
	}

	entries := make(map[string]LogSample)
	if strings.TrimSpace(siteName) != "" {
		found := false
		for _, site := range snap.Sites {
			if !matchesSite(site, siteName) {
				continue
			}
			found = true
			path := pickLogPath(site, kind)
			if path == "" {
				continue
			}
			entries[path] = LogSample{Kind: kind, Path: path, Site: site.Name, Tail: tail}
		}
		if !found {
			return nil, fmt.Errorf("nginx site %q not found", siteName)
		}
	} else {
		path := snap.DefaultErrorLog
		if kind == "access" {
			path = snap.DefaultAccessLog
		}
		if path == "" {
			for _, site := range snap.Sites {
				path = pickLogPath(site, kind)
				if path != "" {
					entries[path] = LogSample{Kind: kind, Path: path, Site: site.Name, Tail: tail}
				}
			}
		} else {
			entries[path] = LogSample{Kind: kind, Path: path, Tail: tail}
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no %s log path found in nginx configuration", kind)
	}

	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	out := make([]LogSample, 0, len(paths))
	for _, path := range paths {
		entry := entries[path]
		content, err := readLastLines(path, tail)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Content = content
		}
		out = append(out, entry)
	}
	return out, nil
}

func loadSnapshot() (*snapshot, error) {
	binaryPath, err := exec.LookPath("nginx")
	if err != nil {
		return nil, fmt.Errorf("nginx binary not found in PATH")
	}

	versionOut, versionErr := exec.Command(binaryPath, "-V").CombinedOutput()
	if versionErr != nil && strings.TrimSpace(string(versionOut)) == "" {
		return nil, fmt.Errorf("nginx -V failed: %w", versionErr)
	}
	version, confPath, accessLog, errorLog := parseVersionOutput(string(versionOut))

	testOut, testErr := exec.Command(binaryPath, "-t").CombinedOutput()
	configTestOutput := strings.TrimSpace(string(testOut))
	configTestOK := testErr == nil

	configOut, configErr := exec.Command(binaryPath, "-T").CombinedOutput()
	if configErr != nil && strings.TrimSpace(string(configOut)) == "" {
		return nil, fmt.Errorf("nginx -T failed: %w", configErr)
	}

	files, sites, upstreams, stubStatus := parseConfigDump(string(configOut))
	masterRunning, workerCount := runningProcesses()

	return &snapshot{
		Version:          version,
		BinaryPath:       binaryPath,
		ConfigPath:       confPath,
		DefaultAccessLog: accessLog,
		DefaultErrorLog:  errorLog,
		ConfigTestOK:     configTestOK,
		ConfigTestOutput: configTestOutput,
		MasterRunning:    masterRunning,
		WorkerCount:      workerCount,
		StubStatus:       stubStatus,
		ConfigFiles:      files,
		Sites:            sites,
		Upstreams:        upstreams,
	}, nil
}

func parseVersionOutput(out string) (version, confPath, accessLog, errorLog string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "nginx version:") {
			version = strings.TrimSpace(strings.TrimPrefix(line, "nginx version:"))
		}
		if strings.Contains(line, "configure arguments:") {
			fields := strings.Fields(line)
			for _, field := range fields {
				switch {
				case strings.HasPrefix(field, "--conf-path="):
					confPath = strings.Trim(strings.TrimPrefix(field, "--conf-path="), `"`)
				case strings.HasPrefix(field, "--http-log-path="):
					accessLog = strings.Trim(strings.TrimPrefix(field, "--http-log-path="), `"`)
				case strings.HasPrefix(field, "--error-log-path="):
					errorLog = strings.Trim(strings.TrimPrefix(field, "--error-log-path="), `"`)
				}
			}
		}
	}
	return version, confPath, accessLog, errorLog
}

func parseConfigDump(out string) ([]string, []SiteDetail, []UpstreamInfo, bool) {
	currentFile := ""
	fileSet := map[string]struct{}{}
	var sites []SiteDetail
	var upstreams []UpstreamInfo
	stack := []string{}
	var currentSite *SiteDetail
	var currentUpstream *UpstreamInfo
	stubStatus := false

	for _, rawLine := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if strings.HasPrefix(trimmed, "# configuration file ") {
			path := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "# configuration file "), ":"))
			currentFile = path
			if currentFile != "" {
				fileSet[currentFile] = struct{}{}
			}
			continue
		}

		trimmed = stripInlineComment(trimmed)
		if trimmed == "" {
			continue
		}

		if strings.Contains(trimmed, "stub_status") {
			stubStatus = true
		}

		if currentSite != nil {
			parseSiteDirective(currentSite, trimmed)
		}
		if currentUpstream != nil {
			parseUpstreamDirective(currentUpstream, trimmed)
		}

		opens := strings.Count(trimmed, "{")
		closes := strings.Count(trimmed, "}")
		recognizedOpens := 0

		if strings.HasPrefix(trimmed, "upstream ") && strings.Contains(trimmed, "{") {
			name := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(trimmed, "upstream "), "{"))
			if len(name) > 0 {
				currentUpstream = &UpstreamInfo{Name: name[0], File: currentFile}
				stack = append(stack, "upstream")
				recognizedOpens++
			}
		} else if isServerBlockStart(trimmed) {
			currentSite = &SiteDetail{File: currentFile}
			stack = append(stack, "server")
			recognizedOpens++
		}

		for i := 0; i < opens-recognizedOpens; i++ {
			stack = append(stack, "other")
		}
		for i := 0; i < closes && len(stack) > 0; i++ {
			popped := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch popped {
			case "server":
				if currentSite != nil {
					finalizeSite(currentSite)
					sites = append(sites, *currentSite)
					currentSite = nil
				}
			case "upstream":
				if currentUpstream != nil {
					finalizeUpstream(currentUpstream)
					upstreams = append(upstreams, *currentUpstream)
					currentUpstream = nil
				}
			}
		}
	}

	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	sort.Slice(upstreams, func(i, j int) bool { return upstreams[i].Name < upstreams[j].Name })
	return setToSortedList(fileSet), sites, upstreams, stubStatus
}

func parseSiteDirective(site *SiteDetail, line string) {
	switch {
	case strings.HasPrefix(line, "server_name "):
		site.ServerNames = append(site.ServerNames, directiveArgs(line, "server_name")...)
	case strings.HasPrefix(line, "listen "):
		site.Listens = append(site.Listens, strings.Join(directiveArgs(line, "listen"), " "))
	case strings.HasPrefix(line, "root "):
		site.Roots = append(site.Roots, strings.Join(directiveArgs(line, "root"), " "))
	case strings.HasPrefix(line, "access_log "):
		args := directiveArgs(line, "access_log")
		if len(args) > 0 {
			site.AccessLog = args[0]
		}
	case strings.HasPrefix(line, "error_log "):
		args := directiveArgs(line, "error_log")
		if len(args) > 0 {
			site.ErrorLog = args[0]
		}
	case strings.Contains(line, "proxy_pass "):
		args := directiveArgs(line, "proxy_pass")
		if len(args) > 0 {
			site.ProxyPasses = append(site.ProxyPasses, args[0])
		}
	}
}

func parseUpstreamDirective(upstream *UpstreamInfo, line string) {
	if strings.HasPrefix(line, "server ") && !isServerBlockStart(line) {
		args := directiveArgs(line, "server")
		if len(args) > 0 {
			upstream.Servers = append(upstream.Servers, strings.Join(args, " "))
		}
	}
}

func finalizeSite(site *SiteDetail) {
	site.ServerNames = dedupeAndSort(site.ServerNames)
	site.Listens = dedupeAndSort(nonEmpty(site.Listens))
	site.Roots = dedupeAndSort(nonEmpty(site.Roots))
	site.ProxyPasses = dedupeAndSort(nonEmpty(site.ProxyPasses))
	if site.Name == "" {
		if len(site.ServerNames) > 0 {
			site.Name = site.ServerNames[0]
		} else if site.File != "" {
			site.Name = filepath.Base(site.File)
		} else {
			site.Name = "unnamed_server"
		}
	}
}

func finalizeUpstream(upstream *UpstreamInfo) {
	upstream.Servers = dedupeAndSort(nonEmpty(upstream.Servers))
}

func summarizeSites(sites []SiteDetail) []SiteSummary {
	out := make([]SiteSummary, 0, len(sites))
	for _, site := range sites {
		out = append(out, SiteSummary{
			Name:        site.Name,
			ServerNames: site.ServerNames,
			Listens:     site.Listens,
			ProxyPasses: site.ProxyPasses,
			AccessLog:   site.AccessLog,
			ErrorLog:    site.ErrorLog,
			File:        site.File,
		})
	}
	return out
}

func runningProcesses() (bool, int) {
	out, err := exec.Command("ps", "aux").CombinedOutput()
	if err != nil {
		return false, 0
	}
	var master bool
	var workers int
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.Contains(line, "nginx: master process"):
			master = true
		case strings.Contains(line, "nginx: worker process"):
			workers++
		}
	}
	return master, workers
}

func readLastLines(path string, tail int) (string, error) {
	if path == "" || path == "off" || strings.Contains(path, "$") {
		return "", fmt.Errorf("unsupported log path %q", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, tail)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > tail {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

func pickLogPath(site SiteDetail, kind string) string {
	if kind == "access" {
		return site.AccessLog
	}
	return site.ErrorLog
}

func matchesSite(site SiteDetail, query string) bool {
	query = strings.TrimSpace(query)
	if site.Name == query {
		return true
	}
	if site.File == query || filepath.Base(site.File) == query {
		return true
	}
	for _, name := range site.ServerNames {
		if name == query {
			return true
		}
	}
	return false
}

func isServerBlockStart(line string) bool {
	return strings.HasPrefix(line, "server ") && strings.Contains(line, "{")
}

func stripInlineComment(line string) string {
	if line == "" {
		return ""
	}
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return strings.TrimSpace(line)
}

func directiveArgs(line, directive string) []string {
	index := strings.Index(line, directive)
	if index < 0 {
		return nil
	}
	rest := strings.TrimSpace(line[index+len(directive):])
	rest = strings.TrimSuffix(rest, ";")
	if rest == "" {
		return nil
	}
	return strings.Fields(rest)
}

func dedupeAndSort(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func setToSortedList(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
