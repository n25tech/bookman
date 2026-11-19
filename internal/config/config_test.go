package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bookman.toml")
	err := os.WriteFile(p, []byte(`
listen = ":9000"
rescan_interval = "1h"
thumb_workers = 3
libraries = [{ name = "Books", path = "/media/books/", exclude = ["comics/"] }]
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" || cfg.RescanInterval.Duration != time.Hour || cfg.ThumbWorkers != 3 {
		t.Fatalf("unexpected config %+v", cfg)
	}
	if cfg.Libraries[0].Path != "/media/books" || cfg.Libraries[0].Exclude[0] != "comics" {
		t.Fatalf("paths not cleaned: %+v", cfg.Libraries)
	}

	env := map[string]string{"BOOKMAN_LISTEN": ":1", "BOOKMAN_LIBRARIES": "A=/a; B=/b", "BOOKMAN_THUMB_WORKERS": "5"}
	if err := applyEnv(&cfg, func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":1" || len(cfg.Libraries) != 2 || cfg.Libraries[1].Path != "/b" || cfg.ThumbWorkers != 5 {
		t.Fatalf("env not applied: %+v", cfg)
	}
}

func TestUnknownKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.toml")
	os.WriteFile(p, []byte("lisen = \":1\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
