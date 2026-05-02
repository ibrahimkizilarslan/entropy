package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ibrahimkizilarslan/entropy-cli/pkg/config"
)

type InjectionEvent struct {
	Action       string
	Target       string
	Success      bool
	ResultStatus string
	Error        string
	DryRun       bool
}

type ChaosLogger struct {
	file   *os.File
	logger *log.Logger
}

func NewChaosLogger(logFilePath string) (*ChaosLogger, error) {
	if logFilePath == "" {
		logFilePath = ".entropy/engine.log"
	}
	dir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &ChaosLogger{
		file:   f,
		logger: log.New(f, "", 0),
	}, nil
}

func (l *ChaosLogger) format(level, msg string) string {
	ts := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("%s | %-6s | %s", ts, level, msg)
}

func (l *ChaosLogger) LogStart(cfg *config.ChaosConfig) {
	msg := fmt.Sprintf("ENGINE STARTED | targets=%s interval=%ds max_down=%d cooldown=%ds dry_run=%v",
		strings.Join(cfg.Targets, ","), cfg.Interval, cfg.Safety.MaxDown, cfg.Safety.Cooldown, cfg.Safety.DryRun)
	l.logger.Println(l.format("INFO", msg))
}

func (l *ChaosLogger) LogStop(cycleCount, injectionCount int) {
	msg := fmt.Sprintf("ENGINE STOPPED | cycles=%d injections=%d", cycleCount, injectionCount)
	l.logger.Println(l.format("INFO", msg))
}

func (l *ChaosLogger) LogInjection(event InjectionEvent) {
	dry := ""
	if event.DryRun {
		dry = "[DRY-RUN] "
	}
	if event.Success {
		msg := fmt.Sprintf("%s%s → %s | result=%s", dry, strings.ToUpper(event.Action), event.Target, event.ResultStatus)
		l.logger.Println(l.format("ACTION", msg))
	} else {
		msg := fmt.Sprintf("%s%s → %s | %s", dry, strings.ToUpper(event.Action), event.Target, event.Error)
		l.logger.Println(l.format("ERROR", msg))
	}
}

func (l *ChaosLogger) LogCooldownSkip(remaining float64) {
	msg := fmt.Sprintf("COOLDOWN | remaining=%.1fs", remaining)
	l.logger.Println(l.format("SKIP", msg))
}

func (l *ChaosLogger) LogMaxDownSkip(downContainers []string) {
	msg := fmt.Sprintf("MAX_DOWN | down=%s", strings.Join(downContainers, ","))
	l.logger.Println(l.format("SKIP", msg))
}

func (l *ChaosLogger) LogError(message string) {
	msg := fmt.Sprintf("ENGINE ERROR | %s", message)
	l.logger.Println(l.format("ERROR", msg))
}

func (l *ChaosLogger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}
