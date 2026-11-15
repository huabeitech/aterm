package desktop

// RendererPlatform translates Go OS names to the Node process.platform values
// consumed by the unchanged Electron renderer, including Windows feature gates.
func RendererPlatform(goos string) string {
	if goos == "windows" {
		return "win32"
	}
	return goos
}
