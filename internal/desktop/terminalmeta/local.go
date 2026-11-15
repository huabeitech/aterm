// Package terminalmeta reads terminal metadata without injecting commands into
// the interactive input stream. Unavailable metadata never fails a connection.
package terminalmeta

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func ParseWorkingDirectories(output string) map[int]string {
	paths := map[int]string{}
	pid := 0
	for _, raw := range strings.Split(output, "\x00") {
		field := strings.TrimPrefix(raw, "\n")
		if strings.HasPrefix(field, "p") {
			pid, _ = strconv.Atoi(field[1:])
		} else if pid > 0 && strings.HasPrefix(field, "n/") {
			paths[pid] = field[1:]
		}
	}
	return paths
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
	cancel context.CancelFunc
}

func (b *boundedBuffer) String() string { return b.buffer.String() }
func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > b.limit {
		b.cancel()
		return 0, errors.New("terminal metadata output exceeds its limit")
	}
	return b.buffer.Write(data)
}
func ReadWorkingDirectories(ctx context.Context, pids []int) map[int]string {
	result := map[int]string{}
	if len(pids) == 0 {
		return result
	}
	switch runtime.GOOS {
	case "linux":
		for _, pid := range pids {
			if ctx.Err() != nil {
				return result
			}
			if pid > 0 {
				if path, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd"); err == nil {
					result[pid] = path
				}
			}
		}
	case "darwin":
		ids := make([]string, 0, len(pids))
		for _, pid := range pids {
			if pid > 0 {
				ids = append(ids, strconv.Itoa(pid))
			}
		}
		if len(ids) == 0 {
			return result
		}
		bounded, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		output := &boundedBuffer{limit: 256 * 1024, cancel: cancel}
		command := exec.CommandContext(bounded, "/usr/sbin/lsof", "-a", "-p", strings.Join(ids, ","), "-d", "cwd", "-F0pn")
		command.Stdout = output
		if err := command.Run(); err == nil {
			return ParseWorkingDirectories(output.String())
		}
	}
	return result
}
