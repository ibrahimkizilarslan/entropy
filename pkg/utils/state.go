package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	StateDirName  = ".entropy"
	StateFileName = "state.json"
	LogFileName   = "engine.log"
)

type EventRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Action       string    `json:"action"`
	Target       string    `json:"target"`
	DryRun       bool      `json:"dry_run"`
	ResultStatus string    `json:"result_status"`
	Error        string    `json:"error"`
}

type EngineState struct {
	PID               int           `json:"pid"`
	StartedAt         time.Time     `json:"started_at"`
	ConfigPath        string        `json:"config_path"`
	DryRun            bool          `json:"dry_run"`
	CycleCount        int           `json:"cycle_count"`
	DownContainers    []string      `json:"down_containers"`
	CooldownRemaining float64       `json:"cooldown_remaining"`
	CooldownTotal     int           `json:"cooldown_total"`
	LastEvent         *EventRecord  `json:"last_event"`
	History           []EventRecord `json:"history"`
}

type StateManager struct {
	dir       string
	stateFile string
	logFile   string
}

func NewStateManager(cwd string) *StateManager {
	if cwd == "" {
		cwd = "."
	}
	dir := filepath.Join(cwd, StateDirName)
	return &StateManager{
		dir:       dir,
		stateFile: filepath.Join(dir, StateFileName),
		logFile:   filepath.Join(dir, LogFileName),
	}
}

func (s *StateManager) EnsureDir() error {
	return os.MkdirAll(s.dir, 0755)
}

func (s *StateManager) StateFile() string {
	return s.stateFile
}

func (s *StateManager) LogFile() string {
	return s.logFile
}

func (s *StateManager) Write(data *EngineState) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}
	tmp := s.stateFile + ".tmp"
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.stateFile)
}

func (s *StateManager) Read() (*EngineState, error) {
	b, err := os.ReadFile(s.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var data EngineState
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (s *StateManager) Clear() error {
	err := os.Remove(s.stateFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *StateManager) IsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if err.Error() == "os: process already finished" || err.Error() == "no such process" {
		return false
	}
	return true
}

func (s *StateManager) RunningPID() *int {
	state, err := s.Read()
	if err != nil || state == nil {
		return nil
	}
	if s.IsAlive(state.PID) {
		return &state.PID
	}
	return nil
}
