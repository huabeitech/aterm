package service

import "testing"

func TestValidRemoteBasename(t *testing.T) {
	for _, name := range []string{"notes", "release 1", ".config"} {
		if !validRemoteBasename(name) {
			t.Errorf("validRemoteBasename(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", ".", "..", "nested/name", `nested\\name`} {
		if validRemoteBasename(name) {
			t.Errorf("validRemoteBasename(%q) = true, want false", name)
		}
	}
}

func TestParseRemoteFileListIncludesUnixMetadata(t *testing.T) {
	output := "config\037l\03712\0371710000000.5\037-1\0371710000100\0371710000200\037lrwxrwxrwx\037deploy\037staff\037current.conf\03712345\0371\n"
	files := parseRemoteFileList(output, "/srv/app")
	if len(files) != 1 {
		t.Fatalf("parseRemoteFileList() returned %d files, want 1", len(files))
	}
	file := files[0]
	if file.Path != "/srv/app/config" || file.Kind != "l" || file.IsDirectory {
		t.Fatalf("unexpected file identity: %#v", file)
	}
	if file.Permissions != "lrwxrwxrwx" || file.Owner != "deploy" || file.Group != "staff" || file.LinkTarget != "current.conf" {
		t.Fatalf("unexpected Unix metadata: %#v", file)
	}
	if file.Inode != 12345 || file.LinkCount != 1 || file.CreatedAt != "" || file.ModifiedAt == "" || file.AccessedAt == "" || file.ChangedAt == "" {
		t.Fatalf("unexpected parsed timestamps or diagnostics: %#v", file)
	}
}
