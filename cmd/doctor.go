package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/josephpaul/opsagent/internal/config"
	"github.com/josephpaul/opsagent/internal/monitor"
	"github.com/josephpaul/opsagent/internal/telegram"
	"github.com/spf13/cobra"

	_ "modernc.org/sqlite"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the health of all OpsAgent components",
	Long: `Displays a color-coded status overview of OpsAgent configuration,
services, and session storage.

  Green  = healthy / configured / running
  Yellow = warning / optional item missing
  Red    = error / required item missing / service stopped`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func green(s string) string  { return colorGreen + s + colorReset }
func red(s string) string    { return colorRed + s + colorReset }
func yellow(s string) string { return colorYellow + s + colorReset }
func cyan(s string) string   { return colorCyan + s + colorReset }
func bold(s string) string   { return colorBold + s + colorReset }

func ok(label, detail string) {
	fmt.Printf("    %s  %s %s\n", green("✓"), label, detail)
}

func warn(label, detail string) {
	fmt.Printf("    %s  %s %s\n", yellow("⚠"), label, detail)
}

func fail(label, detail string) {
	fmt.Printf("    %s  %s %s\n", red("✗"), label, detail)
}

func info(label, detail string) {
	fmt.Printf("    %s  %s\n", cyan(label), detail)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println()
	fmt.Println(bold("  OpsAgent Doctor"))
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Println()

	doctorGeneral()
	fmt.Println()
	doctorProvider()
	fmt.Println()
	doctorTelegram()
	fmt.Println()
	doctorLogin()
	fmt.Println()
	doctorMonitor()
	fmt.Println()
	doctorSessions()
	fmt.Println()

	return nil
}

func doctorGeneral() {
	fmt.Println(bold("  General"))

	info("Version:", Version)
	info("OS/Arch:", runtime.GOOS+"/"+runtime.GOARCH)

	cfgPath, err := config.FilePath()
	if err != nil {
		fail("Config:", "cannot determine path: "+err.Error())
		return
	}

	if fi, err := os.Stat(cfgPath); err == nil {
		ok("Config:", fmt.Sprintf("%s (%d bytes)", cfgPath, fi.Size()))
	} else {
		fail("Config:", cfgPath+" (not found)")
	}
}

func doctorProvider() {
	fmt.Println(bold("  LLM Provider"))

	pairs, err := config.Read()
	if err != nil {
		fail("Config:", "cannot read: "+err.Error())
		return
	}

	effective := effectiveProvider(provider)
	info("Active:", effective)

	type keyInfo struct {
		name     string
		provider string
	}
	keys := []keyInfo{
		{"GOOGLE_API_KEY", "gemini"},
		{"OPENAI_API_KEY", "openai"},
		{"ANTHROPIC_API_KEY", "anthropic"},
	}

	for _, k := range keys {
		val := pairs[k.name]
		if val == "" {
			val = os.Getenv(k.name)
		}
		if val != "" {
			marker := ""
			if k.provider == effective {
				marker = " (active)"
			}
			ok(k.name+":", maskValue(k.name, val)+marker)
		} else if k.provider == effective {
			fail(k.name+":", "not set (required for "+k.provider+")")
		}
	}

	modelName := defaultModel(effective, model)
	info("Model:", modelName)
}

func doctorTelegram() {
	fmt.Println(bold("  Telegram Bot"))

	pairs, _ := config.Read()

	checkKey := func(key, label string, required bool) {
		val := pairs[key]
		if val == "" {
			val = os.Getenv(key)
		}
		if val != "" {
			ok(label+":", maskValue(key, val))
		} else if required {
			fail(label+":", "not set (required)")
		} else {
			warn(label+":", "not set (optional)")
		}
	}

	checkKey("TELEGRAM_BOT_TOKEN", "Bot Token", true)
	checkKey("TELEGRAM_USER_ID", "User ID", true)
	checkKey("TELEGRAM_CHAT_ID", "Chat ID", false)
	checkKey("TELEGRAM_WEBHOOK_SECRET", "Webhook Secret", false)

	tgInfo, err := telegram.ServiceStatus()
	if err != nil {
		warn("Service:", "cannot check: "+err.Error())
		return
	}

	if !tgInfo.Installed {
		warn("Service:", "not installed")
		return
	}

	if tgInfo.Running {
		detail := "running"
		if tgInfo.Mode != "" {
			detail += " (" + tgInfo.Mode
			if tgInfo.Port != "" {
				detail += ", port " + tgInfo.Port
			}
			detail += ")"
		}
		ok("Service:", detail)
	} else {
		fail("Service:", "installed but stopped")
	}

	if tgInfo.WebhookURL != "" {
		info("Webhook URL:", tgInfo.WebhookURL)
	}
}

func doctorMonitor() {
	fmt.Println(bold("  Monitor"))

	monInfo, err := monitor.Status()
	if err != nil {
		warn("Service:", "cannot check: "+err.Error())
		return
	}

	if !monInfo.Installed {
		warn("Service:", "not installed")
		return
	}

	if monInfo.Running {
		ok("Service:", "running")
	} else {
		fail("Service:", "installed but stopped")
	}

	if monInfo.Interval != "" {
		info("Interval:", monInfo.Interval)
	}
	if monInfo.CPUThreshold != "" {
		info("CPU Threshold:", monInfo.CPUThreshold+"%")
	}
	if monInfo.RAMThreshold != "" {
		info("RAM Threshold:", monInfo.RAMThreshold+"%")
	}
	if monInfo.LogPath != "" {
		info("Log:", monInfo.LogPath)
	}
}

func doctorLogin() {
	fmt.Println(bold("  Login Notifications"))

	pairs, _ := config.Read()

	valueFor := func(key string) string {
		val := pairs[key]
		if val == "" {
			val = os.Getenv(key)
		}
		return strings.TrimSpace(val)
	}

	token := valueFor("TELEGRAM_BOT_TOKEN")
	chatID := valueFor("TELEGRAM_CHAT_ID")
	enabled := valueFor("LOGIN_NOTIFY_ENABLED")
	hostLabel := valueFor("LOGIN_NOTIFY_HOSTNAME_LABEL")
	timezone := valueFor("LOGIN_NOTIFY_TIMEZONE")
	timeFormat := valueFor("LOGIN_NOTIFY_TIME_FORMAT")

	if token != "" {
		ok("Bot Token:", maskValue("TELEGRAM_BOT_TOKEN", token))
	} else {
		fail("Bot Token:", "not set (required)")
	}
	if chatID != "" {
		ok("Chat ID:", chatID)
	} else {
		fail("Chat ID:", "not set (required)")
	}

	switch strings.ToLower(enabled) {
	case "", "true", "1", "yes":
		if enabled == "" {
			ok("Enabled:", "true (default)")
		} else {
			ok("Enabled:", enabled)
		}
	case "false", "0", "no":
		warn("Enabled:", enabled)
	default:
		warn("Enabled:", enabled+" (unrecognized value)")
	}

	if hostLabel != "" {
		info("Host Label:", hostLabel)
	}
	if timezone == "" {
		info("Time Zone:", "UTC (default)")
	} else if _, err := time.LoadLocation(timezone); err != nil {
		warn("Time Zone:", timezone+" (invalid: "+err.Error()+")")
	} else {
		ok("Time Zone:", timezone)
	}
	if timeFormat == "" {
		info("Time Format:", loginDefaultTimeFormat+" (default)")
	} else {
		info("Time Format:", timeFormat)
	}

	pamExecPath := detectPamExec()
	if pamExecPath != "" {
		ok("pam_exec.so:", pamExecPath)
	} else {
		warn("pam_exec.so:", "not found in common paths")
	}

	targetsFound := 0
	for _, path := range pamTargetFiles() {
		if !fileExists(path) {
			warn(path+":", "missing")
			continue
		}
		targetsFound++
		if okHook, _ := pamFileContainsOpsagent(path); okHook {
			ok(path+":", "opsagent hook installed")
		} else {
			warn(path+":", "present but opsagent hook not found")
		}
	}

	if targetsFound == 0 {
		warn("PAM Targets:", "no common PAM target files found")
	}

	if snippet, configPath, err := pamSnippet(); err == nil {
		info("Expected Hook:", snippet)
		info("Config Path:", configPath)
	} else {
		warn("Expected Hook:", "cannot determine: "+err.Error())
	}
}

func doctorSessions() {
	fmt.Println(bold("  Session Memory"))

	dir, err := config.Dir()
	if err != nil {
		fail("Database:", "cannot determine path: "+err.Error())
		return
	}
	dbPath := filepath.Join(dir, "sessions.db")

	fi, err := os.Stat(dbPath)
	if err != nil {
		warn("Database:", "not found (no sessions yet)")
		return
	}

	ok("Database:", fmt.Sprintf("%s (%s)", dbPath, humanSize(fi.Size())))

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		warn("Stats:", "cannot open: "+err.Error())
		return
	}
	defer db.Close()

	var sessionCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessionCount); err != nil {
		warn("Stats:", "cannot query sessions: "+err.Error())
		return
	}

	var eventCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&eventCount); err != nil {
		warn("Stats:", "cannot query events: "+err.Error())
		return
	}

	info("Sessions:", fmt.Sprintf("%d", sessionCount))
	info("Events:", fmt.Sprintf("%d stored", eventCount))
}

func humanSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
