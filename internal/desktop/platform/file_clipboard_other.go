//go:build !darwin

package platform

func ClipboardFilePaths() []string {
	return []string{}
}
