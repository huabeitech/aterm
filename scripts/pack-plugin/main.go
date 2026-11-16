package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"ssh-connect/internal/desktop/plugins"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: pack-plugin <staging-directory> <archive>")
		os.Exit(2)
	}
	if err := pack(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func pack(root, path string) error {
	plugin, err := plugins.InspectDirectory(root)
	if err != nil {
		return err
	}
	names := []string{"manifest.json", plugin.Manifest.Entry}
	for _, view := range plugin.Manifest.Contributes.Views {
		names = append(names, view.Entry)
	}
	if plugin.Manifest.I18n != nil {
		for _, name := range plugin.Manifest.I18n.Locales {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		entry, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(content); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if _, err := plugins.InspectArchiveBytes(data.Bytes()); err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0644)
}
