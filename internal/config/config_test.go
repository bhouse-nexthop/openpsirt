package config

import (
	"log/slog"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load with nothing set: %v", err)
	}
	if c.Addr == "" || c.LogFormat == "" || c.ShutdownGrace == 0 {
		t.Fatalf("a default is missing: %+v", c)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"LOG_LEVEL", "chatty"},
		{"LOG_FORMAT", "yaml"},
		{"ADDR", "  "},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(envPrefix+tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatalf("%s=%q was accepted", tc.key, tc.value)
			}
		})
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv(envPrefix+"ADDR", "127.0.0.1:9999")
	t.Setenv(envPrefix+"LOG_LEVEL", "debug")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != "127.0.0.1:9999" {
		t.Errorf("Addr = %q", c.Addr)
	}
	if c.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v", c.LogLevel)
	}
}

func TestAutoMigrateIsOnUnlessTurnedOff(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.AutoMigrate {
		t.Error("auto-migration should be on by default")
	}
	t.Setenv(envPrefix+"AUTO_MIGRATE", "false")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.AutoMigrate {
		t.Error("auto-migration should be off when set to false")
	}
}
