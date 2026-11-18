package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type point struct {
	At       int64    `json:"at"`
	CPU      *float64 `json:"cpu"`
	Memory   *float64 `json:"memory"`
	Receive  *float64 `json:"receive"`
	Send     *float64 `json:"send"`
	Swapping bool     `json:"swapping"`
}
type system struct {
	Hostname string   `json:"hostname"`
	OS       string   `json:"os"`
	Kernel   string   `json:"kernel"`
	Uptime   *float64 `json:"uptime"`
}
type cpu struct {
	Usage  *float64   `json:"usage"`
	IOWait *float64   `json:"iowait"`
	Count  *float64   `json:"count"`
	Cores  []*float64 `json:"cores"`
	Load   []float64  `json:"load"`
}
type memory struct {
	Total       float64  `json:"total"`
	Available   *float64 `json:"available"`
	Cached      *float64 `json:"cached"`
	SwapTotal   *float64 `json:"swapTotal"`
	SwapUsed    *float64 `json:"swapUsed"`
	SwapInRate  *float64 `json:"swapInRate"`
	SwapOutRate *float64 `json:"swapOutRate"`
}
type disk struct {
	Device       string   `json:"device"`
	Mount        string   `json:"mount"`
	Type         string   `json:"type"`
	Total        float64  `json:"total"`
	Used         float64  `json:"used"`
	Available    float64  `json:"available"`
	Percent      float64  `json:"percent"`
	InodePercent *float64 `json:"inodePercent"`
}
type counters struct {
	Receive float64 `json:"receiveBytes"`
	Send    float64 `json:"sendBytes"`
	Errors  float64 `json:"errors"`
	Drops   float64 `json:"drops"`
}
type network struct {
	Name string `json:"name"`
	counters
	ReceiveRate *float64 `json:"receiveRate"`
	SendRate    *float64 `json:"sendRate"`
	ErrorRate   *float64 `json:"errorRate"`
	DropRate    *float64 `json:"dropRate"`
}
type diskIO struct {
	Device    string   `json:"device"`
	ReadRate  *float64 `json:"readRate"`
	WriteRate *float64 `json:"writeRate"`
}
type snapshot struct {
	Version          int       `json:"version"`
	System           system    `json:"system"`
	CPU              cpu       `json:"cpu"`
	Memory           *memory   `json:"memory"`
	Disks            []disk    `json:"disks"`
	Network          []network `json:"network"`
	IO               []diskIO  `json:"io"`
	DefaultInterface string    `json:"defaultInterface"`
}
type tick struct {
	Total, Idle float64
	Wait        *float64
}
type ioCounters struct{ Read, Written float64 }
type baseline struct {
	timestamp float64
	cpu       []tick
	swap      map[string]float64
	network   map[string]counters
	io        map[string]ioCounters
}

func ptr(v float64) *float64 { return &v }
func num(s string) *float64 {
	n, err := strconv.ParseFloat(s, 64)
	if s == "" || err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return nil
	}
	return ptr(n)
}
func text(s string) string {
	r := []rune(s)
	if len(r) > 160 {
		r = r[:160]
	}
	return string(r)
}
func field(v string, index int) string {
	f := strings.Fields(v)
	if len(f) > index {
		return f[index]
	}
	return ""
}
func tables(raw string) []disk {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	typed := len(lines) > 0 && strings.Contains(lines[0], "Type")
	offset := 1
	if typed {
		offset = 2
	}
	out := []disk{}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		if len(f) != offset+5 {
			continue
		}
		total, used, available, percent := num(f[offset]), num(f[offset+1]), num(f[offset+2]), num(strings.TrimSuffix(f[offset+3], "%"))
		if total == nil || *total <= 0 || used == nil || available == nil || percent == nil || *percent > 100 {
			continue
		}
		kind := ""
		if typed {
			kind = text(f[1])
		}
		out = append(out, disk{Device: text(f[0]), Type: kind, Mount: text(strings.ReplaceAll(f[offset+4], `\040`, " ")), Total: *total, Used: *used, Available: *available, Percent: *percent})
	}
	return out
}
func cpuDelta(cur, prev tick, wait bool) *float64 {
	if cur.Total <= prev.Total {
		return nil
	}
	a, b := cur.Idle, prev.Idle
	if wait {
		if cur.Wait == nil || prev.Wait == nil {
			return nil
		}
		a, b = *cur.Wait, *prev.Wait
	}
	if a < b {
		return nil
	}
	ratio := (a - b) / (cur.Total - prev.Total)
	if !wait {
		ratio = 1 - ratio
	}
	return ptr(math.Max(0, math.Min(100, ratio*100)))
}

var cpuLine = regexp.MustCompile(`^cpu([0-9]+)?\s`)
var memoryLine = regexp.MustCompile(`(?m)^(\w+):\s+(\d+)\s+kB`)
var swapLine = regexp.MustCompile(`(?m)^(pswpin|pswpout)\s+(\d+)`)
var diskDevice = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|nvme\d+n\d+|mmcblk\d+|dm-\d+|md\d+)$`)
var pseudoFS = regexp.MustCompile(`^(tmpfs|devtmpfs|squashfs|proc|sysfs|cgroup2?|nsfs|ramfs|fusectl|debugfs|tracefs)$`)

func parse(raw string, previous *baseline, now time.Time) (snapshot, *baseline, error) {
	if strings.Contains(raw, "ATERM_UNSUPPORTED") {
		return snapshot{}, nil, fmt.Errorf("unsupported")
	}
	blocks := map[string]string{}
	section := ""
	marker := regexp.MustCompile(`^ATERM_([A-Z]+)$`)
	for _, line := range strings.Split(raw, "\n") {
		if match := marker.FindStringSubmatch(line); match != nil {
			section = match[1]
			blocks[section] = ""
			continue
		}
		if section != "" {
			blocks[section] += line + "\n"
		}
	}
	uptime := num(field(blocks["UPTIME"], 0))
	timestamp := float64(now.UnixMilli()) / 1000
	if uptime != nil {
		timestamp = *uptime
	}
	orig := previous
	elapsed := 0.0
	if previous != nil {
		elapsed = timestamp - previous.timestamp
		if elapsed <= 0 || elapsed > 90 {
			previous = nil
		}
	}
	rate := func(current float64, before float64, exists bool) *float64 {
		if previous == nil || !exists || current < before {
			return nil
		}
		return ptr((current - before) / elapsed)
	}
	current := &baseline{timestamp: timestamp, cpu: []tick{}, swap: map[string]float64{}, network: map[string]counters{}, io: map[string]ioCounters{}}
	for _, line := range strings.Split(strings.TrimSpace(blocks["CPU"]), "\n") {
		if !cpuLine.MatchString(line) {
			continue
		}
		fields := strings.Fields(line)[1:]
		if len(fields) > 8 {
			fields = fields[:8]
		}
		if len(fields) < 4 {
			continue
		}
		values := []float64{}
		valid := true
		total := 0.0
		for _, f := range fields {
			n := num(f)
			if n == nil {
				valid = false
				break
			}
			values = append(values, *n)
			total += *n
		}
		if !valid {
			continue
		}
		idle := values[3]
		var wait *float64
		if len(values) > 4 {
			idle += values[4]
			wait = ptr(values[4])
		}
		current.cpu = append(current.cpu, tick{total, idle, wait})
	}
	cp := cpu{Count: num(strings.TrimSpace(blocks["CPUCOUNT"])), Cores: []*float64{}, Load: []float64{}}
	if cp.Count == nil && len(current.cpu) > 1 {
		cp.Count = ptr(float64(len(current.cpu) - 1))
	}
	cpuPrevious := previous
	if uptime == nil {
		cpuPrevious = orig
	}
	for i, cur := range current.cpu {
		var usage *float64
		if cpuPrevious != nil && i < len(cpuPrevious.cpu) {
			usage = cpuDelta(cur, cpuPrevious.cpu[i], false)
			if i == 0 {
				cp.IOWait = cpuDelta(cur, cpuPrevious.cpu[i], true)
			}
		}
		if i == 0 {
			cp.Usage = usage
		} else if len(cp.Cores) < 64 {
			cp.Cores = append(cp.Cores, usage)
		}
	}
	for i, f := range strings.Fields(blocks["LOAD"]) {
		if i >= 3 {
			break
		}
		if n := num(f); n != nil {
			cp.Load = append(cp.Load, *n)
		}
	}
	values := map[string]float64{}
	for _, m := range memoryLine.FindAllStringSubmatch(blocks["MEMORY"], -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		values[m[1]] = v * 1024
	}
	for _, m := range swapLine.FindAllStringSubmatch(blocks["SWAPIO"], -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		current.swap[m[1]] = v
	}
	var mem *memory
	if values["MemTotal"] > 0 {
		mem = &memory{Total: values["MemTotal"]}
		if available, ok := values["MemAvailable"]; ok && available <= mem.Total {
			mem.Available = ptr(available)
		}
		cache, found := 0.0, false
		for _, key := range []string{"Cached", "Buffers", "SReclaimable"} {
			if n, ok := values[key]; ok {
				cache += n
				found = true
			}
		}
		if found {
			mem.Cached = ptr(cache)
		}
		if total, ok := values["SwapTotal"]; ok {
			mem.SwapTotal = ptr(total)
			if free, ok := values["SwapFree"]; ok {
				mem.SwapUsed = ptr(math.Max(0, total-free))
			}
		}
		if page := num(strings.TrimSpace(blocks["PAGESIZE"])); page != nil && previous != nil {
			incoming, ok := current.swap["pswpin"]
			before, was := previous.swap["pswpin"]
			if r := rate(incoming, before, ok && was); r != nil {
				mem.SwapInRate = ptr(*r * *page)
			}
			outgoing, ok := current.swap["pswpout"]
			before, was = previous.swap["pswpout"]
			if r := rate(outgoing, before, ok && was); r != nil {
				mem.SwapOutRate = ptr(*r * *page)
			}
		}
	}
	inodes := map[string]float64{}
	for _, d := range tables(blocks["INODES"]) {
		inodes[d.Mount] = d.Percent
	}
	disks := tables(blocks["DISK"])
	sort.SliceStable(disks, func(i, j int) bool { return disks[i].Mount == "/" && disks[j].Mount != "/" })
	filtered := []disk{}
	seen := map[string]bool{}
	for _, d := range disks {
		if pseudoFS.MatchString(d.Type) || (d.Type == "overlay" && d.Mount != "/") || seen[d.Device] {
			continue
		}
		seen[d.Device] = true
		d.Total *= 1024
		d.Used *= 1024
		d.Available *= 1024
		if inode, ok := inodes[d.Mount]; ok {
			d.InodePercent = ptr(inode)
		}
		filtered = append(filtered, d)
		if len(filtered) >= 32 {
			break
		}
	}
	networks := []network{}
	for _, line := range strings.Split(blocks["NETWORK"], "\n") {
		name, rest, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 16 {
			continue
		}
		vals := []float64{}
		for _, f := range fields {
			if n := num(f); n != nil {
				vals = append(vals, *n)
			}
		}
		if len(vals) != len(fields) {
			continue
		}
		name = text(name)
		c := counters{vals[0], vals[8], vals[2] + vals[10], vals[3] + vals[11]}
		current.network[name] = c
		row := network{Name: name, counters: c}
		if previous != nil {
			before, exists := previous.network[name]
			row.ReceiveRate = rate(c.Receive, before.Receive, exists)
			row.SendRate = rate(c.Send, before.Send, exists)
			row.ErrorRate = rate(c.Errors, before.Errors, exists)
			row.DropRate = rate(c.Drops, before.Drops, exists)
		}
		networks = append(networks, row)
		if len(networks) >= 16 {
			break
		}
	}
	ioRows := []diskIO{}
	for _, line := range strings.Split(blocks["DISKIO"], "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || !diskDevice.MatchString(f[2]) {
			continue
		}
		read, written := num(f[5]), num(f[9])
		if read == nil || written == nil {
			continue
		}
		device := text(f[2])
		current.io[device] = ioCounters{*read, *written}
		row := diskIO{Device: device}
		if previous != nil {
			before, exists := previous.io[device]
			if r := rate(*read, before.Read, exists); r != nil {
				row.ReadRate = ptr(*r * 512)
			}
			if r := rate(*written, before.Written, exists); r != nil {
				row.WriteRate = ptr(*r * 512)
			}
		}
		ioRows = append(ioRows, row)
		if len(ioRows) >= 16 {
			break
		}
	}
	lines := strings.Split(strings.TrimSpace(blocks["SYSTEM"]), "\n")
	sys := system{Uptime: uptime}
	if len(lines) > 0 {
		sys.Hostname = text(lines[0])
	}
	if len(lines) > 1 {
		sys.Kernel = text(lines[1])
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			sys.OS = text(strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"'`))
		}
	}
	route := ""
	if m := regexp.MustCompile(`\bdev\s+(\S+)`).FindStringSubmatch(blocks["ROUTE"]); m != nil {
		route = text(m[1])
	}
	return snapshot{1, sys, cp, mem, filtered, networks, ioRows, route}, current, nil
}
func bytes(value *float64) string {
	if value == nil {
		return "—"
	}
	v := *value
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i > 0 && v < 100 {
		return fmt.Sprintf("%.1f %s", v, units[i])
	}
	return fmt.Sprintf("%.0f %s", v, units[i])
}
func percent(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", math.Floor(*value+0.5))
}
func pickDisk(s snapshot, mount string) *disk {
	for i := range s.Disks {
		if s.Disks[i].Mount == mount {
			return &s.Disks[i]
		}
	}
	if len(s.Disks) == 0 {
		return nil
	}
	best := &s.Disks[0]
	for i := range s.Disks {
		if s.Disks[i].Percent > best.Percent {
			best = &s.Disks[i]
		}
	}
	return best
}
func pickNetwork(s snapshot, name string) *network {
	for i := range s.Network {
		if s.Network[i].Name == name {
			return &s.Network[i]
		}
	}
	for i := range s.Network {
		if s.Network[i].Name == s.DefaultInterface {
			return &s.Network[i]
		}
	}
	for i := range s.Network {
		if !regexp.MustCompile(`^(veth|docker|br-|virbr)`).MatchString(s.Network[i].Name) {
			return &s.Network[i]
		}
	}
	if len(s.Network) > 0 {
		return &s.Network[0]
	}
	return nil
}
func appendPoint(history []point, p point) []point {
	out := []point{}
	for _, v := range history {
		if v.At > p.At-300000 && v.At < p.At {
			out = append(out, v)
		}
	}
	out = append(out, p)
	if len(out) > 120 {
		out = out[len(out)-120:]
	}
	return out
}
func sustained(history []point, interval int, test func(point) bool) bool {
	if len(history) == 0 || !test(history[len(history)-1]) {
		return false
	}
	end := history[len(history)-1].At
	start := end
	for i := len(history) - 2; i >= 0; i-- {
		if !test(history[i]) || start-history[i].At > int64(interval*2000+1000) {
			break
		}
		start = history[i].At
	}
	return end-start >= 30000
}
