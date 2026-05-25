package utils

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
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
	logger *slog.Logger
}

func NewChaosLogger(logFilePath, format string) (*ChaosLogger, error) {
	if logFilePath == "" {
		logFilePath = ".entropy/engine.log"
	}
	dir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Use 0640 for logs to prevent world-readable exposure of potentially sensitive environment info
	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	return &ChaosLogger{
		file:   f,
		logger: slog.New(handler),
	}, nil
}

func (l *ChaosLogger) LogStart(cfg *config.ChaosConfig) {
	l.logger.Info("ENGINE STARTED",
		slog.String("targets", strings.Join(cfg.Targets, ",")),
		slog.Int("interval", cfg.Interval),
		slog.Int("max_down", cfg.Safety.MaxDown),
		slog.Int("cooldown", cfg.Safety.Cooldown),
		slog.Bool("dry_run", cfg.Safety.DryRun),
	)
}

func (l *ChaosLogger) LogStop(cycleCount, injectionCount int) {
	l.logger.Info("ENGINE STOPPED",
		slog.Int("cycles", cycleCount),
		slog.Int("injections", injectionCount),
	)
}

func (l *ChaosLogger) LogInjection(event InjectionEvent) {
	args := []any{
		slog.String("action", event.Action),
		slog.String("target", event.Target),
		slog.Bool("dry_run", event.DryRun),
	}

	if event.Success {
		args = append(args, slog.String("result", event.ResultStatus))
		l.logger.Info("ACTION", args...)
	} else {
		args = append(args, slog.String("error", event.Error))
		l.logger.Error("ERROR", args...)
	}
}

func (l *ChaosLogger) LogCooldownSkip(remaining float64) {
	l.logger.Info("COOLDOWN", slog.Float64("remaining", remaining))
}

func (l *ChaosLogger) LogMaxDownSkip(downContainers []string) {
	l.logger.Info("MAX_DOWN", slog.String("down", strings.Join(downContainers, ",")))
}

func (l *ChaosLogger) LogError(message string) {
	l.logger.Error("ENGINE ERROR", slog.String("message", message))
}

func (l *ChaosLogger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}
