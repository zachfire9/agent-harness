package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestGmailRejectsHeaderInjectionBeforeCredentials(t *testing.T) {
	for _, format := range []string{"", "plain", "multipart", "html"} {
		for _, field := range []string{"from", "to", "subject", "job"} {
			t.Run(format+"/"+field, func(t *testing.T) {
				config := GmailNotifierConfig{From: "agent@example.com", To: []string{"reader@example.com"}, BodyFormat: format}
				message := Message{Job: "test", Body: "private-body"}
				attack := "private-header\r\nBcc: attacker@example.com"
				switch field {
				case "from":
					config.From = attack
				case "to":
					config.To = []string{attack}
				case "subject":
					config.SubjectPrefix = attack
				case "job":
					message.Job = attack
				}
				err := (gmailNotifier{config: config}).Send(context.Background(), message)
				if err == nil || (!strings.Contains(err.Error(), "header") && !strings.Contains(err.Error(), "body_format")) {
					t.Fatalf("expected early safe validation, got %v", err)
				}
				if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "attacker") {
					t.Fatal("validation leaked input")
				}
				if field != "job" && format != "html" {
					if err := validateNotifierConfig(NotifierConfig{Type: "gmail", Gmail: config}, GoogleConfig{}); err == nil {
						t.Fatal("config accepted header injection")
					}
				}
			})
		}
	}
}
