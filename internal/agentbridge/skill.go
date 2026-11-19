package agentbridge

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"ssh-connect/internal/fsutil"
)

//go:embed skills/aterm-ssh/SKILL.md skills/aterm-ssh/scripts/aterm.py
var skillFiles embed.FS

var skillNames = []string{"SKILL.md", "scripts/aterm.py"}

func matchesSkill(dir string) bool {
	for _, name := range skillNames {
		expected, err := skillFiles.ReadFile("skills/aterm-ssh/" + name)
		if err != nil {
			return false
		}
		current, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(expected, current) {
			return false
		}
	}
	return true
}

func SkillInstalled(home, socket string) bool {
	dir := filepath.Join(home, ".agents", "skills", "aterm-ssh")
	data, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	var profile struct{ Socket string }
	return err == nil && json.Unmarshal(data, &profile) == nil && profile.Socket == socket && matchesSkill(dir)
}

func InstallSkill(home, socket string) error {
	parent := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	dest := filepath.Join(parent, "aterm-ssh")
	profile, err := json.Marshal(map[string]string{"socket": socket})
	if err != nil {
		return err
	}
	// Stage the whole skill so discovery cannot see a partially installed script.
	stage, err := os.MkdirTemp(parent, ".aterm-skill-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, name := range skillNames {
		data, err := skillFiles.ReadFile("skills/aterm-ssh/" + name)
		if err != nil {
			return err
		}
		if err = fsutil.AtomicWriteFile(filepath.Join(stage, name), data, 0600); err != nil {
			return err
		}
	}
	if err = fsutil.AtomicWriteFile(filepath.Join(stage, "profile.json"), profile, 0600); err != nil {
		return err
	}
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() {
			return errors.New("skill destination already exists")
		}
		// Only rebind profiles for an identical installed skill.
		if !matchesSkill(dest) {
			return errors.New("different aterm-ssh skill already installed")
		}
		return fsutil.AtomicWriteFile(filepath.Join(dest, "profile.json"), profile, 0600)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, dest)
}
