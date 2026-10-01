package runtime

// RenderSystemdUserUnit returns the Linux user service template for named instances.
func RenderSystemdUserUnit() string {
	return `[Unit]
Description=agent-harness instance %i
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/agent-harness daemon --instance %i --home %h/.local/share/agent-harness/instances/%i
WorkingDirectory=%h/.local/share/agent-harness/instances/%i
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
`
}
