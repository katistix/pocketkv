// Package config loads settings from the environment and an optional .env file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type Config struct {
	Addr              string // listen address, e.g. ":8080"
	DataFile          string // path to the JSON data file
	DashboardUser     string
	DashboardPassword string
}

// Load reads envFile (if it exists) into the process environment without
// overriding variables that are already set, then builds a Config.
func Load(envFile string) (Config, error) {
	if err := loadDotEnv(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("read %s: %w", envFile, err)
	}
	c := Config{
		Addr:              getenv("ADDR", ":8080"),
		DataFile:          getenv("DATA_FILE", "data/pocketkv.json"),
		DashboardUser:     os.Getenv("DASHBOARD_USER"),
		DashboardPassword: os.Getenv("DASHBOARD_PASSWORD"),
	}
	if c.DashboardUser == "" || c.DashboardPassword == "" {
		return Config{}, errors.New("DASHBOARD_USER and DASHBOARD_PASSWORD must be set (see .env.example)")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv parses simple KEY=VALUE lines. Blank lines and lines starting
// with # are ignored; values may be wrapped in single or double quotes.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}
