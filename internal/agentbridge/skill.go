package agentbridge

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"ssh-connect/internal/fsutil"
)

//go:embed skills/aterm-ssh/SKILL.md skills/aterm-ssh/scripts/aterm.py
var skillFiles embed.FS

var skillNames = []string{"SKILL.md", "scripts/aterm.py"}

// Accepted hashes are the exact bundled files from before background launch.
var previousSkillHashes = map[string][]string{
	"SKILL.md": {
		"8f603d4bf102be60be8d9fdcca4efb7e6ee9e2d5ea2f6f3b99a7a506fc0145ff",
		"18ef5b1e0dc4f3a3d553888d587172f075b08949ffc963b9e1968633979de672",
	},
	"scripts/aterm.py": {"20df70cc90f32e1d6d7b6f13e35ca2e41b0a1d1d3172973b948086314f2183e7"},
}

type SkillProfile struct {
	Socket  string `json:"socket"`
	Bundle  string `json:"bundle,omitempty"`
	DataDir string `json:"dataDir,omitempty"`
}

func matchesSkill(dir string, allowPrevious bool) bool {
	for _, name := range skillNames {
		expected, err := skillFiles.ReadFile("skills/aterm-ssh/" + name)
		if err != nil {
			return false
		}
		current, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return false
		}
		if !bytes.Equal(expected, current) {
			hash := fmt.Sprintf("%x", sha256.Sum256(current))
			if !allowPrevious || !slices.Contains(previousSkillHashes[name], hash) {
				return false
			}
		}
	}
	return true
}

func SkillInstalled(home string, expected SkillProfile) bool {
	dir := filepath.Join(home, ".agents", "skills", "aterm-ssh")
	data, err := os.ReadFile(filepath.Join(dir, "profile.json"))
	var profile SkillProfile
	return err == nil && json.Unmarshal(data, &profile) == nil && profile == expected && matchesSkill(dir, false)
}

func InstallSkill(home string, settings SkillProfile) error {
	parent := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	dest := filepath.Join(parent, "aterm-ssh")
	profile, err := json.Marshal(settings)
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
		if !matchesSkill(dest, true) {
			return errors.New("different aterm-ssh skill already installed")
		}
		// Install the profile first so the updated script always has launch details.
		if err := fsutil.AtomicWriteFile(filepath.Join(dest, "profile.json"), profile, 0600); err != nil {
			return err
		}
		for _, name := range skillNames {
			data, err := os.ReadFile(filepath.Join(stage, name))
			if err != nil {
				return err
			}
			if err := fsutil.AtomicWriteFile(filepath.Join(dest, name), data, 0600); err != nil {
				return err
			}
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(stage, dest)
}
