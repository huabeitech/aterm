//go:build !windows && !cgo

package localpty

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Static builds cannot call the system NSS/directory service. Read the local
// effective-user account entry; a missing entry remains an explicit error.
func accountShell() (string, error) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return "", err
	}
	defer file.Close()
	return readPasswdShell(file, os.Geteuid())
}
func readPasswdShell(reader io.Reader, uid int) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 1024*1024))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	wanted := strconv.Itoa(uid)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) == 7 && fields[2] == wanted {
			return fields[6], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("effective user %d has no local account entry", uid)
}
