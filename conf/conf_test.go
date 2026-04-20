package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPathDefaultsRetryCountToOne(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := []byte("Log:\n  Level: info\nNodes:\n  - ApiHost: https://panel.example.com\n    NodeID: 1\n    ApiKey: secret\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := New()
	if err := cfg.LoadFromPath(path); err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}
	if len(cfg.NodeConfigs) != 1 {
		t.Fatalf("NodeConfigs length = %d, want 1", len(cfg.NodeConfigs))
	}
	if cfg.NodeConfigs[0].RetryCount == nil {
		t.Fatal("RetryCount = nil, want default value")
	}
	if got := *cfg.NodeConfigs[0].RetryCount; got != 1 {
		t.Fatalf("RetryCount = %d, want 1", got)
	}
}
