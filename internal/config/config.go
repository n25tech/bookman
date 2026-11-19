// Package config loads bookman.toml and applies BOOKMAN_* environment overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Library is one root folder of books.
type Library struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
	// Exclude lists folders (relative to Path) that are skipped while scanning,
	// for example a sub-folder that is configured as its own library.
	Exclude []string `toml:"exclude"`
}

// Config is the whole server configuration.
type Config struct {
	Listen string `toml:"listen"`
	// DatabaseURL is a PostgreSQL connection string. When empty, the standard
	// libpq variables (PGHOST, PGPORT, PGDATABASE, PGUSER, PGPASSWORD) are used.
	DatabaseURL string `toml:"database_url"`
	// Libraries seeds the library list on first start. After that, libraries
	// are managed in the admin UI (and stored in the database).
	Libraries []Library `toml:"libraries"`
	// MediaRoots are the folders the admin UI may browse and add libraries from.
	// Default: "/".
	MediaRoots     []string `toml:"media_roots"`
	RescanInterval Duration `toml:"rescan_interval"`
	ThumbWorkers   int      `toml:"thumb_workers"`
	PageCacheMB    int      `toml:"page_cache_mb"`
	// Watch enables fsnotify watching of the library folders.
	Watch bool `toml:"watch"`
}

// Duration is a time.Duration that decodes from a TOML string like "6h".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// Default returns the configuration used when no file is given.
func Default() Config {
	return Config{
		Listen:         ":8080",
		RescanInterval: Duration{6 * time.Hour},
		ThumbWorkers:   max(1, runtime.NumCPU()/2),
		PageCacheMB:    128,
		Watch:          true,
	}
}

// Load reads the file at path (if non-empty) over the defaults, then applies
// environment overrides and validates the result.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		md, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return cfg, fmt.Errorf("reading %s: %w", path, err)
		}
		if und := md.Undecoded(); len(und) > 0 {
			return cfg, fmt.Errorf("%s: unknown keys %v", path, und)
		}
	}
	if err := applyEnv(&cfg, os.LookupEnv); err != nil {
		return cfg, err
	}
	return cfg, cfg.validate()
}

func applyEnv(cfg *Config, lookup func(string) (string, bool)) error {
	if v, ok := lookup("BOOKMAN_LISTEN"); ok {
		cfg.Listen = v
	}
	if v, ok := lookup("BOOKMAN_DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := lookup("BOOKMAN_RESCAN_INTERVAL"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("BOOKMAN_RESCAN_INTERVAL: %w", err)
		}
		cfg.RescanInterval = Duration{d}
	}
	for name, dst := range map[string]*int{"BOOKMAN_THUMB_WORKERS": &cfg.ThumbWorkers, "BOOKMAN_PAGE_CACHE_MB": &cfg.PageCacheMB} {
		if v, ok := lookup(name); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			*dst = n
		}
	}
	if v, ok := lookup("BOOKMAN_WATCH"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("BOOKMAN_WATCH: %w", err)
		}
		cfg.Watch = b
	}
	if v, ok := lookup("BOOKMAN_MEDIA_ROOTS"); ok {
		cfg.MediaRoots = nil
		for _, p := range strings.Split(v, ";") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.MediaRoots = append(cfg.MediaRoots, p)
			}
		}
	}
	// BOOKMAN_LIBRARIES="Books=/media/books;Comics=/media/comics" replaces the list.
	if v, ok := lookup("BOOKMAN_LIBRARIES"); ok {
		cfg.Libraries = nil
		for _, part := range strings.Split(v, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, p, found := strings.Cut(part, "=")
			if !found {
				return fmt.Errorf("BOOKMAN_LIBRARIES: %q is not name=path", part)
			}
			cfg.Libraries = append(cfg.Libraries, Library{Name: strings.TrimSpace(name), Path: strings.TrimSpace(p)})
		}
	}
	return nil
}

func (c *Config) validate() error {
	if c.ThumbWorkers < 1 {
		c.ThumbWorkers = 1
	}
	if c.PageCacheMB < 0 {
		c.PageCacheMB = 0
	}
	if len(c.MediaRoots) == 0 {
		c.MediaRoots = []string{"/"}
	}
	for i, r := range c.MediaRoots {
		if !filepath.IsAbs(r) {
			return fmt.Errorf("media root %q must be an absolute path", r)
		}
		c.MediaRoots[i] = filepath.Clean(r)
	}
	seen := map[string]bool{}
	for i := range c.Libraries {
		l := &c.Libraries[i]
		if l.Name == "" || l.Path == "" {
			return fmt.Errorf("library %d needs a name and a path", i+1)
		}
		l.Path = filepath.Clean(l.Path)
		if seen[l.Path] {
			return fmt.Errorf("library path %s is listed twice", l.Path)
		}
		seen[l.Path] = true
		for j, e := range l.Exclude {
			l.Exclude[j] = filepath.Clean(e)
		}
	}
	return nil
}
