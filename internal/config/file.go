package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/zettaio/backrest-sidecar/internal/model"
	fsutil "github.com/zettaio/backrest-sidecar/internal/util/fs"
)

// Load reads the JSON config file if it exists.
func Load(path string) (*model.Config, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := &model.Config{}
			cfg.EnsureNonNil()
			return cfg, nil, nil
		}
		return nil, nil, fmt.Errorf("read config: %w", err)
	}
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	cfg := &model.Config{}
	if v, ok := raw["repos"]; ok {
		if err := json.Unmarshal(v, &cfg.Repos); err != nil {
			return nil, nil, fmt.Errorf("parse repos: %w", err)
		}
		delete(raw, "repos")
	}
	if v, ok := raw["plans"]; ok {
		if err := json.Unmarshal(v, &cfg.Plans); err != nil {
			return nil, nil, fmt.Errorf("parse plans: %w", err)
		}
		delete(raw, "plans")
	}
	cfg.SetExtras(raw)
	cfg.EnsureNonNil()
	return cfg, data, nil
}

// Write writes the config atomically.
func Write(path string, cfg *model.Config) ([]byte, error) {
	cfg.EnsureNonNil()
	out := make(map[string]json.RawMessage, len(cfg.Extras())+2)
	for k, v := range cfg.Extras() {
		out[k] = v
	}
	reposBytes, err := json.Marshal(cfg.Repos)
	if err != nil {
		return nil, fmt.Errorf("marshal repos: %w", err)
	}
	plansBytes, err := json.Marshal(cfg.Plans)
	if err != nil {
		return nil, fmt.Errorf("marshal plans: %w", err)
	}
	out["repos"] = reposBytes
	out["plans"] = plansBytes
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	if err := fsutil.AtomicWrite(path, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return data, nil
}
