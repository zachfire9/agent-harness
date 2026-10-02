package runtime

import (
	"os"
	"path/filepath"
)

const SystemdUserUnitName = "agent-harness@.service"

// RenderSystemdUserUnit returns the Linux user service template for named instances.
func RenderSystemdUserUnit() string {
	return `[Unit]
Description=agent-harness instance %i
After=network-online.target

[Service]
Type=simple
EnvironmentFile=-%h/.local/share/agent-harness/instances/%i/config/secrets/env
ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i
WorkingDirectory=%h/.local/share/agent-harness/instances/%i
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
`
}

// InstallSystemdUserUnit writes the user-level systemd template under configHome.
func InstallSystemdUserUnit(configHome string) (string, error) {
	unitPath := filepath.Join(configHome, "systemd", "user", SystemdUserUnitName)
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(unitPath, []byte(RenderSystemdUserUnit()), 0o644); err != nil {
		return "", err
	}
	return unitPath, nil
}
