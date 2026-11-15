package service

import (
	"context"
	"testing"
)

// Synthetic connection data; never use real host configuration in fixtures.
const sampleHostSnippet = `
Host sample-search
	User sample
	HostName 192.0.2.10
	#SCEGroup 00000000-0000-4000-8000-000000000001

Host sample-test
	User sample
	HostName 192.0.2.20
	#SCEGroup 00000000-0000-4000-8000-000000000001

Host example.com
	User sample
	HostName 192.0.2.30
	#SCEGroup 00000000-0000-4000-8000-000000000002
`

func TestPreviewHostSnippetImportParsesSampleAndIgnoresComments(t *testing.T) {
	app := NewAppService(AppServiceOptions{Store: newMemoryHostStore()})

	preview, err := app.PreviewHostSnippetImport(context.Background(), sampleHostSnippet)
	if err != nil {
		t.Fatalf("PreviewHostSnippetImport() error = %v", err)
	}

	if len(preview.Rows) != 3 {
		t.Fatalf("Rows len = %d, want 3", len(preview.Rows))
	}
	assertPreviewRow(t, preview.Rows[0], "sample-search", "192.0.2.10", "sample", HostSnippetStatusReady, true)
	assertPreviewRow(t, preview.Rows[1], "sample-test", "192.0.2.20", "sample", HostSnippetStatusReady, true)
	assertPreviewRow(t, preview.Rows[2], "example.com", "192.0.2.30", "sample", HostSnippetStatusReady, true)
	if preview.Rows[0].Input.Remark != "" {
		t.Fatalf("Remark = %q, want empty because comments are ignored", preview.Rows[0].Input.Remark)
	}
}

func assertPreviewRow(t *testing.T, row HostSnippetPreviewRow, alias, hostName, user, status string, selectable bool) {
	t.Helper()
	if row.Alias != alias {
		t.Fatalf("Alias = %q, want %q", row.Alias, alias)
	}
	if row.HostName != hostName {
		t.Fatalf("HostName = %q, want %q", row.HostName, hostName)
	}
	if row.User != user {
		t.Fatalf("User = %q, want %q", row.User, user)
	}
	if row.Status != status {
		t.Fatalf("Status = %q, want %q", row.Status, status)
	}
	if row.Selectable != selectable {
		t.Fatalf("Selectable = %t, want %t", row.Selectable, selectable)
	}
}
