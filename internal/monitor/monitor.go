// Package monitor provides a background monitoring loop that polls CPU and RAM
// usage and triggers an AI diagnosis when thresholds are breached.
package monitor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/josephpaul/opsagent/internal/diagnostics"
)

// Config holds the settings for a monitoring run.
type Config struct {
	Interval     time.Duration
	CPUThreshold int
	RAMThreshold int
	LogPath      string
	Diagnose     func(ctx context.Context, query string) (string, error)
}

// Run starts the monitoring loop. It blocks until the context is cancelled or
// a SIGINT/SIGTERM is received. When a threshold is breached, Diagnose is called
// to get an AI-powered explanation.
func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var logWriter io.Writer = os.Stdout
	if cfg.LogPath != "" {
		f, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer f.Close()
		logWriter = io.MultiWriter(os.Stdout, f)
	}

	log := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(logWriter, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg)
	}

	log("monitor started (interval=%s cpu_threshold=%d%% ram_threshold=%d%%)",
		cfg.Interval, cfg.CPUThreshold, cfg.RAMThreshold)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	check := func() {
		cpuUsage, topProc, _, err := diagnostics.CPU()
		if err != nil {
			log("cpu check error: %v", err)
		}

		memStats, err := diagnostics.Memory()
		if err != nil {
			log("memory check error: %v", err)
		}

		ramUsage := 0
		if pct, ok := memStats["usage_percent"]; ok {
			switch v := pct.(type) {
			case int:
				ramUsage = v
			case int64:
				ramUsage = int(v)
			}
		}

		log("cpu=%d%% ram=%d%% top_process=%s", cpuUsage, ramUsage, topProc)

		cpuBreached := cfg.CPUThreshold > 0 && cpuUsage >= cfg.CPUThreshold
		ramBreached := cfg.RAMThreshold > 0 && ramUsage >= cfg.RAMThreshold

		if !cpuBreached && !ramBreached {
			return
		}

		var reason string
		switch {
		case cpuBreached && ramBreached:
			reason = fmt.Sprintf("ALERT: CPU at %d%% (threshold %d%%) and RAM at %d%% (threshold %d%%)",
				cpuUsage, cfg.CPUThreshold, ramUsage, cfg.RAMThreshold)
		case cpuBreached:
			reason = fmt.Sprintf("ALERT: CPU at %d%% (threshold %d%%)", cpuUsage, cfg.CPUThreshold)
		default:
			reason = fmt.Sprintf("ALERT: RAM at %d%% (threshold %d%%)", ramUsage, cfg.RAMThreshold)
		}
		log("%s", reason)

		if cfg.Diagnose != nil {
			query := fmt.Sprintf("CPU is at %d%% and RAM is at %d%%. Top process is %s. Diagnose why the server is under heavy load.",
				cpuUsage, ramUsage, topProc)
			log("requesting AI diagnosis...")
			diagnosis, err := cfg.Diagnose(ctx, query)
			if err != nil {
				log("diagnosis error: %v", err)
			} else {
				log("--- AI Diagnosis ---\n%s", diagnosis)
			}
		}
	}

	check()

	for {
		select {
		case <-ctx.Done():
			log("monitor stopped")
			return nil
		case <-ticker.C:
			check()
		}
	}
}
