//go:build darwin && !mas

package app

import (
	"testing"

	"ssh-connect/frontend/src/shared/i18n"
	"ssh-connect/internal/service"
)

func TestAgentApprovalBody(t *testing.T) {
	host := service.Host{Name: "Payments API", Alias: "payments-prod"}
	directory := agentDirectoryPath("payments", []service.Directory{
		{ID: "prod", Name: "Production"},
		{ID: "payments", ParentID: "prod", Name: "Payments"},
	})
	const endpoint = "deploy@[2001:db8::1]:22"
	for _, test := range []struct {
		locale string
		want   string
	}{
		{"en", "Payments API\nSSH alias: payments-prod\nFolder: Production / Payments\nLogin target: " + endpoint},
		{"zh-CN", "Payments API\nSSH 别名：payments-prod\n所属目录：Production / Payments\n登录目标：" + endpoint},
	} {
		t.Run(test.locale, func(t *testing.T) {
			if got := agentApprovalBody(i18n.NativeMessages(test.locale, nil), host, directory, endpoint); got != test.want {
				t.Fatalf("body = %q, want %q", got, test.want)
			}
		})
	}
	host.Name = ""
	if got := agentApprovalBody(i18n.NativeMessages("en", nil), host, "", endpoint); got != "payments-prod\nLogin target: "+endpoint {
		t.Fatalf("body with alias as display name = %q", got)
	}
}

func TestAgentApprovalSystemLocale(t *testing.T) {
	for _, test := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"it-IT", "zh-Hans-CN", "en-US"}, "拒绝"},
		{[]string{"zh-Hant-HK", "en-US"}, "拒絕"},
		{[]string{"en-US", "zh-CN"}, "Deny"},
	} {
		if got := i18n.NativeMessages("system", test.preferred)["agent.rejectConnection"]; got != test.want {
			t.Errorf("languages %v returned %q, want %q", test.preferred, got, test.want)
		}
	}
}
