package files

import (
	"github.com/pkg/sftp"
	"io"
	"net"
	"testing"
)

func fixture(t *testing.T) (*Manager, string, *sftp.Client) {
	t.Helper()
	clientPipe, serverPipe := net.Pipe()
	server := sftp.NewRequestServer(serverPipe, sftp.InMemHandler())
	go func() { _ = server.Serve(); _ = server.Close() }()
	client, err := sftp.NewClientPipe(clientPipe, clientPipe)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	info, err := manager.Add(client, nil, SessionInfo{HostID: "host", Target: "host", Title: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.CloseAll(); clientPipe.Close(); serverPipe.Close() })
	return manager, info.ID, client
}
func writeFixture(t *testing.T, client *sftp.Client, name, text string) {
	t.Helper()
	file, err := client.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, text); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestSFTPTextSaveProtectsRevisionAndRejectsBinary(t *testing.T) {
	manager, id, client := fixture(t)
	writeFixture(t, client, "/config", "one")
	document, err := manager.ReadText(id, "/config")
	if err != nil {
		t.Fatal(err)
	}
	document.Text = "two"
	saved, err := manager.SaveText(id, document)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Text != "two" || saved.Revision == document.Revision {
		t.Fatal("save did not update revision")
	}
	document.Text = "stale"
	if _, err := manager.SaveText(id, document); err == nil {
		t.Fatal("stale revision overwrote remote text")
	}
	read, err := manager.ReadText(id, "/config")
	if err != nil || read.Text != "two" {
		t.Fatal("revision conflict damaged remote file")
	}
	writeFixture(t, client, "/binary", "a\x00b")
	if _, err := manager.ReadText(id, "/binary"); err == nil {
		t.Fatal("binary content accepted as text")
	}
}
func TestSFTPListingMutationsAndSymlinkSafeDelete(t *testing.T) {
	manager, id, client := fixture(t)
	if err := manager.Mutate(id, Mutation{Kind: "mkdir", Parent: "/", Name: "folder"}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, client, "/folder/file", "keep")
	if err := client.Symlink("/folder", "/link"); err != nil {
		t.Fatal(err)
	}
	listing, err := manager.List(id, "/")
	if err != nil || len(listing.Entries) != 2 {
		t.Fatalf("listing: %#v, %v", listing, err)
	}
	if err := manager.Mutate(id, Mutation{Kind: "delete", Paths: []string{"/link"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stat("/folder/file"); err != nil {
		t.Fatal("symlink deletion removed target")
	}
	if err := manager.Mutate(id, Mutation{Kind: "rename", Path: "/folder/file", Destination: "/folder/renamed"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Mutate(id, Mutation{Kind: "delete", Paths: []string{"/"}}); err == nil {
		t.Fatal("root deletion accepted")
	}
	if err := manager.Mutate(id, Mutation{Kind: "touch", Parent: "/", Name: "../escape"}); err == nil {
		t.Fatal("invalid child name accepted")
	}
	if err := manager.Mutate(id, Mutation{Kind: "delete", Paths: []string{"/folder"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stat("/folder"); err == nil {
		t.Fatal("recursive deletion left folder")
	}
}
