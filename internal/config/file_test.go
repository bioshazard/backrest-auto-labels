package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zettaio/backrest-sidecar/internal/model"
)

func TestWritePreservesRepoExtras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	seed := `{
  "repos": [
    {
      "id": "bios-lab1-backrest-b2",
      "guid": "11111111-2222-3333-4444-555555555555",
      "uri": "b2:bucket/prefix",
      "auto_initialize": false,
      "env": ["B2_ACCOUNT_ID=abc", "B2_ACCOUNT_KEY=def"]
    }
  ],
  "plans": []
}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg.Plans = []model.Plan{{ID: "plan-alpha", Repo: "bios-lab1-backrest-b2", Paths: []string{"/data"}}}
	cfg.Normalize()

	if _, err := Write(path, cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, "guid") {
		t.Fatalf("expected guid field to be preserved, got: %s", output)
	}
	if !strings.Contains(output, "auto_initialize") {
		t.Fatalf("expected auto_initialize field to be preserved, got: %s", output)
	}
}
