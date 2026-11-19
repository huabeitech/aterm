package agentbridge

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstalledSkillQueriesProfileWithoutMCP(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	if _, err = os.Stat("/usr/bin/curl"); err != nil {
		t.Skip("system curl unavailable")
	}
	dir, err := os.MkdirTemp("/tmp", "aterm-skill-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "test.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if r.URL.Path != "/request" || req.Method != "list" || req.Query != "测试机" {
			t.Errorf("unexpected request: %+v", req)
		}
		json.NewEncoder(w).Encode(Result{Hosts: []Host{{ID: "1", Name: "测试机"}}})
	})}
	go server.Serve(l)
	defer server.Close()
	home := t.TempDir()
	for range 2 {
		if err = InstallSkill(home, socket); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(home, ".agents", "skills", "aterm-ssh")
	data, err := exec.Command(python, filepath.Join(path, "scripts", "aterm.py"), "list", "测试机").CombinedOutput()
	if err != nil || !bytes.Contains(data, []byte("测试机")) {
		t.Fatalf("list: %s (%v)", data, err)
	}
	if err = os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	if InstallSkill(home, socket) == nil {
		t.Fatal("overwrote a user-modified skill")
	}
}
