package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func probe(uptime, user, idle, rx, tx int) string {
	return fmt.Sprintf(`ATERM_SYSTEM
fixture
6.1
PRETTY_NAME="Fixture Linux"
ATERM_UPTIME
%d 0
ATERM_CPU
cpu %d 0 0 %d 0 0 0 0 0 0
cpu0 %d 0 0 %d 0 0 0 0
ATERM_CPUCOUNT
1
ATERM_LOAD
0.1 0.2 0.3 1/10 100
ATERM_MEMORY
MemTotal: 1000 kB
MemAvailable: 400 kB
Cached: 200 kB
SwapTotal: 500 kB
SwapFree: 300 kB
ATERM_SWAPIO
pswpin %d
pswpout %d
ATERM_PAGESIZE
4096
ATERM_DISK
Filesystem Type 1024-blocks Used Available Capacity Mounted on
/dev/vda ext4 1000 800 200 80%% /
/dev/vda ext4 1000 800 200 80%% /duplicate
tmpfs tmpfs 100 0 100 0%% /run
overlay overlay 1000 500 500 50%% /container
ATERM_INODES
Filesystem Type Inodes IUsed IFree IUse%% Mounted on
/dev/vda ext4 1000 950 50 95%% /
ATERM_NETWORK
eth0: %d 0 0 0 0 0 0 0 %d 0 0 0 0 0 0 0
lo: 999 0 0 0 0 0 0 0 999 0 0 0 0 0 0 0
ATERM_ROUTE
default via 10.0.0.1 dev eth0
ATERM_DISKIO
8 0 vda 10 0 %d 0 20 0 %d 0 0
8 1 vda1 10 0 10 0 20 0 20 0 0
ATERM_END
`, uptime, user, idle, user, idle, uptime, uptime, rx, tx, rx, tx)
}
func TestSamplingBaselineRatesAndCounterReset(t *testing.T) {
	now := time.Now()
	first, b, e := parse(probe(100, 10, 90, 1000, 2000), nil, now)
	if e != nil || first.CPU.Usage != nil || first.Network[0].ReceiveRate != nil {
		t.Fatal(first, e)
	}
	second, _, e := parse(probe(110, 15, 95, 1200, 2400), b, now.Add(time.Second*10))
	if e != nil || *second.CPU.Usage != 50 || *second.Network[0].ReceiveRate != 20 || *second.Memory.SwapOutRate != 4096 || len(second.Disks) != 1 || *second.Disks[0].InodePercent != 95 || len(second.IO) != 1 || second.Memory.Total != 1024000 {
		t.Fatalf("bad snapshot %+v err=%v", second, e)
	}
	reset, _, _ := parse(probe(90, 1, 9, 1, 2), b, now)
	if reset.CPU.Usage != nil || reset.Network[0].ReceiveRate != nil {
		t.Fatal("reboot retained rates")
	}
	gap, _, _ := parse(probe(200, 20, 180, 2000, 4000), b, now)
	if gap.CPU.Usage != nil || gap.Network[0].SendRate != nil {
		t.Fatal("large sampling gap retained rates")
	}
}
func TestMissingMemAvailableAndUnsupported(t *testing.T) {
	s, _, e := parse(strings.ReplaceAll(probe(100, 10, 90, 1000, 2000), "MemAvailable: 400 kB\n", ""), nil, time.Now())
	if e != nil || s.Memory.Available != nil {
		t.Fatal("invented available memory", e)
	}
	if _, _, e = parse("ATERM_UNSUPPORTED\n", nil, time.Now()); e == nil {
		t.Fatal("unsupported OS accepted")
	}
}
func TestWarningRequiresContinuousThirtySecondsAndHistoryBound(t *testing.T) {
	history := []point{}
	for i := 0; i <= 30; i += 5 {
		history = appendPoint(history, point{At: int64(100000 + i*1000), CPU: ptr(95)})
	}
	if !sustained(history, 5, func(p point) bool { return p.CPU != nil && *p.CPU >= 90 }) {
		t.Fatal("missing sustained warning")
	}
	history = appendPoint(history, point{At: 150000, CPU: ptr(95)})
	if sustained(history, 5, func(p point) bool { return p.CPU != nil && *p.CPU >= 90 }) {
		t.Fatal("warning crossed sampling gap")
	}
	for i := 0; i < 200; i++ {
		history = appendPoint(history, point{At: 200000 + int64(i*1000)})
	}
	if len(history) > 120 {
		t.Fatal("unbounded history")
	}
}
func TestMonitorTranslationsHaveMatchingKeysAndParameters(t *testing.T) {
	paths, _ := filepath.Glob("../locales/*.json")
	var source map[string]string
	raw, _ := os.ReadFile("../locales/en.json")
	json.Unmarshal(raw, &source)
	params := regexp.MustCompile(`\{\{[^}]+\}\}`)
	for _, p := range paths {
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		var m map[string]string
		if e = json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		// Locales may add language-specific plural categories.
		for k, v := range source {
			value, ok := m[k]
			if !ok || value == "" {
				t.Fatalf("%s missing %s", p, k)
			}
			for _, param := range params.FindAllString(v, -1) {
				if !strings.Contains(value, param) {
					t.Fatalf("%s:%s missing %s", p, k, param)
				}
			}
		}
	}
}
