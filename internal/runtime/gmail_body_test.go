package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGmailBodyFormatDefaultWithoutConfigFile(t *testing.T) {
	paths, err := LinuxPaths(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadRuntimeConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifier.Gmail.BodyFormat != "plain" {
		t.Fatalf("default format = %q, want plain", cfg.Notifier.Gmail.BodyFormat)
	}
}

func TestGmailBodyFormatConfig(t *testing.T) {
	for _, tc := range []struct {
		value, want string
		invalid     bool
	}{
		{"", "plain", false}, {"plain", "plain", false}, {"multipart", "multipart", false},
		{"html", "", true}, {"MULTIPART", "", true}, {"secret-unknown-format", "", true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			paths, err := LinuxPaths(t.TempDir(), "test")
			if err != nil {
				t.Fatal(err)
			}
			if err := InitInstance(paths); err != nil {
				t.Fatal(err)
			}
			config := "notifier:\n  type: gmail\n  gmail:\n    from: agent@example.com\n    to:\n      - operator@example.com\n"
			if tc.value != "" {
				config += "    body_format: " + tc.value + "\n"
			}
			if err := os.WriteFile(filepath.Join(paths.ConfigDir, "config.yaml"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := ReadRuntimeConfig(paths)
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "body_format") {
					t.Fatalf("expected safe body_format error, got %v", err)
				}
				if strings.Contains(err.Error(), tc.value) {
					t.Fatal("error echoed invalid value")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Notifier.Gmail.BodyFormat != tc.want {
				t.Fatalf("got format %q, want %q", cfg.Notifier.Gmail.BodyFormat, tc.want)
			}
		})
	}
}
