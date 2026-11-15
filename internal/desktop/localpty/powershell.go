package localpty

import (
	"encoding/base64"
	"encoding/binary"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var powerShellName = regexp.MustCompile(`(?i)(?:^|[\\/])(?:pwsh|powershell)(?:\.exe)?$`)
var reportEncoding = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// PowerShellIntegration preserves user profiles and calls their prompt first,
// then reports only FileSystem provider paths using a per-session private nonce.
// EncodedCommand is UTF-16LE, matching the existing Electron launch contract.
func PowerShellIntegration(shell, nonce string) []string {
	if !powerShellName.MatchString(shell) {
		return nil
	}
	script := `$global:__atermPrompt=$function:prompt; function global:prompt { $result=if($global:__atermPrompt){& $global:__atermPrompt}else{'PS '+$PWD+'> '}; try { if($PWD.Provider.Name -eq 'FileSystem'){ $path=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($PWD.ProviderPath)); [Console]::Write([string][char]27+']777;aterm-cwd;` + nonce + `;'+$path+[string][char]7) } } catch {}; $result }`
	units := utf16.Encode([]rune(script))
	bytes := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(bytes[index*2:], unit)
	}
	return []string{"-NoLogo", "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(bytes)}
}

// DirectoryReports observes a private OSC sequence without removing or changing
// PTY bytes. One terminal reader owns it; state is bounded across split frames.
type DirectoryReports struct {
	prefix, pending string
	changed         func(string)
}

func NewDirectoryReports(nonce string, changed func(string)) *DirectoryReports {
	return &DirectoryReports{prefix: "\x1b]777;aterm-cwd;" + nonce + ";", changed: changed}
}
func (r *DirectoryReports) Consume(data []byte) {
	r.pending += string(data)
	for {
		start := strings.Index(r.pending, r.prefix)
		if start < 0 {
			if len(r.pending) > len(r.prefix) {
				r.pending = r.pending[len(r.pending)-len(r.prefix):]
			}
			return
		}
		end := strings.IndexByte(r.pending[start:], 7)
		if end < 0 {
			r.pending = r.pending[start:]
			if len(r.pending) > 16384 {
				r.pending = r.pending[len(r.pending)-16384:]
			}
			return
		}
		end += start
		encoded := r.pending[start+len(r.prefix) : end]
		r.pending = r.pending[end+1:]
		if len(encoded) > 12000 || !reportEncoding.MatchString(encoded) {
			continue
		}
		// Node's Buffer accepts unpadded base64, including a final incomplete group.
		decoded, _ := base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
		path := string(decoded)
		if !utf8.ValidString(path) {
			path = decodeReportUTF8(decoded)
		}
		if !windowsAbsolute(path) || strings.ContainsAny(path, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f") {
			continue
		}
		r.changed(path)
	}
}
func windowsAbsolute(path string) bool {
	if len(path) == 0 {
		return false
	}
	slash := func(c byte) bool { return c == '/' || c == '\\' }
	if slash(path[0]) {
		return true
	}
	return len(path) >= 3 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':' && slash(path[2])
}

// Match Buffer.toString("utf8"): one replacement per maximal invalid prefix,
// including an incomplete multi-byte character at the end of a report.
func decodeReportUTF8(data []byte) string {
	var text strings.Builder
	for len(data) > 0 {
		character, size := utf8.DecodeRune(data)
		if character != utf8.RuneError || size != 1 {
			text.WriteRune(character)
			data = data[size:]
			continue
		}
		expected := 1
		lead := data[0]
		switch {
		case lead >= 0xc2 && lead <= 0xdf:
			expected = 2
		case lead >= 0xe0 && lead <= 0xef:
			expected = 3
		case lead >= 0xf0 && lead <= 0xf4:
			expected = 4
		}
		consumed := 1
		for consumed < expected && consumed < len(data) {
			next := data[consumed]
			if next < 0x80 || next > 0xbf {
				break
			}
			if consumed == 1 && ((lead == 0xe0 && next < 0xa0) || (lead == 0xed && next > 0x9f) || (lead == 0xf0 && next < 0x90) || (lead == 0xf4 && next > 0x8f)) {
				break
			}
			consumed++
		}
		text.WriteRune(utf8.RuneError)
		data = data[consumed:]
	}
	return text.String()
}
