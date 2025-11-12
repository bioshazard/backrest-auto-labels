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
	var cfg model.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.EnsureNonNil()
	return &cfg, data, nil
}

// Write writes the config atomically.
func Write(path string, cfg *model.Config) ([]byte, error) {
	cfg.EnsureNonNil()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	if err := fsutil.AtomicWrite(path, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return data, nil
}
