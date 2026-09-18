package config

import (
	"path/filepath"
	"testing"
)

func TestLoadSmokeProfile(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config", "scenarios.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.Profile("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if profile.MeasuredTrials != 2 {
		t.Fatalf("measured trials %d", profile.MeasuredTrials)
	}
	if _, err := cfg.Profile("full"); err != nil {
		t.Fatal(err)
	}
}
