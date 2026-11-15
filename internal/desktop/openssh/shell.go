package openssh

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed ssh-wrapper.sh
var wrapperTemplate string

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func wrapper(directory, binary string) string {
	return strings.NewReplacer(quote("__ATERM_BINARY__"), quote(binary), quote("__ATERM_DIRECTORY__/connections"), quote(filepath.Join(directory, "connections"))).Replace(wrapperTemplate)
}
func envMap(values []string) map[string]string {
	result := map[string]string{}
	for _, value := range values {
		if key, value, ok := strings.Cut(value, "="); ok {
			result[key] = value
		}
	}
	return result
}
func envList(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
func (i *Integration) ShellEnvironment(shell string, environment []string) ([]string, []string, error) {
	env := envMap(environment)
	command := "function ssh() { " + quote(filepath.Join(i.directory, "ssh")) + " \"$@\"; }\nexport PATH=" + quote(i.directory) + ":\"$PATH\"\n"
	switch filepath.Base(shell) {
	case "zsh":
		original := env["ZDOTDIR"]
		if original == "" {
			var err error
			original, err = os.UserHomeDir()
			if err != nil {
				return nil, nil, err
			}
		}
		for _, file := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
			script := ""
			if file == ".zshenv" {
				script = "_aterm_zdotdir=" + quote(original) + "\n"
			}
			script += "ZDOTDIR=\"$_aterm_zdotdir\"\n"
			if file == ".zshrc" {
				script += "[[ \"$HISTFILE\" == " + quote(filepath.Join(i.directory, ".zsh_history")) + " ]] && HISTFILE=\"$ZDOTDIR/.zsh_history\"\n"
			}
			script += "[[ -f \"$ZDOTDIR/" + file + "\" ]] && source \"$ZDOTDIR/" + file + "\"\n_aterm_zdotdir=${ZDOTDIR:-$HOME}\nZDOTDIR=" + quote(i.directory) + "\n"
			if file == ".zshrc" {
				script += command
			}
			if file == ".zlogin" {
				script += "ZDOTDIR=\"$_aterm_zdotdir\"\nunset _aterm_zdotdir\n"
			}
			if err := os.WriteFile(filepath.Join(i.directory, file), []byte(script), 0600); err != nil {
				return nil, nil, err
			}
		}
		env["ZDOTDIR"] = i.directory
		return envList(env), []string{"-l"}, nil
	case "bash":
		rc := filepath.Join(i.directory, "bashrc")
		script := "if [ -f /etc/profile ]; then . /etc/profile; fi\nfor p in \"$HOME/.bash_profile\" \"$HOME/.bash_login\" \"$HOME/.profile\"; do\n if [ -f \"$p\" ]; then . \"$p\"; break; fi\ndone\n" + command
		if err := os.WriteFile(rc, []byte(script), 0600); err != nil {
			return nil, nil, err
		}
		return envList(env), []string{"--rcfile", rc, "-i"}, nil
	default:
		path := env["PATH"]
		if path == "" {
			path = "/usr/bin:/bin"
		}
		env["PATH"] = i.directory + ":" + path
		return envList(env), []string{"-l"}, nil
	}
}
