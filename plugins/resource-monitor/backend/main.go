package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	sdk "ssh-connect/packages/plugin-go"
)

type preference struct {
	Mount   string `json:"mount,omitempty"`
	Network string `json:"network,omitempty"`
}
type sample struct {
	mu          sync.Mutex
	scope       sdk.SessionInfo
	resources   snapshot
	previous    *baseline
	history     []point
	preference  preference
	initialized bool
	ready       bool
}
type backend struct {
	r       *sdk.SessionRuntime
	mu      sync.Mutex
	tokens  map[string]*sample
	history map[string]*sample
}

func main() {
	sdk.ServeSession(func(r *sdk.SessionRuntime) sdk.SessionProgram { return newBackend(r) })
}
func newBackend(r *sdk.SessionRuntime) *backend {
	return &backend{r: r, tokens: map[string]*sample{}, history: map[string]*sample{}}
}
func (b *backend) Tasks() int { return 0 }
func (b *backend) Close() {
	b.mu.Lock()
	b.tokens = map[string]*sample{}
	b.history = map[string]*sample{}
	b.mu.Unlock()
}
func (b *backend) Release(token string)                          { b.mu.Lock(); delete(b.tokens, token); b.mu.Unlock() }
func (b *backend) Stream(context.Context, sdk.StreamEvent) error { return nil }
func (b *backend) Refresh(ctx context.Context, scope sdk.SessionInfo) error {
	response, err := b.r.Run(ctx, scope.Token, sdk.ProcessRequest{Executable: "sh", Args: []string{"-c", resourceProbeCommand}, TimeoutMS: 6000})
	if err != nil {
		return err
	}
	if response.ExitCode == nil || *response.ExitCode != 0 {
		return errors.New("Resource collection failed")
	}
	b.mu.Lock()
	data := b.tokens[scope.Token]
	if data == nil {
		data = &sample{scope: scope, history: []point{}}
		if old := b.history[scope.SessionID]; old != nil {
			data.history = append([]point{}, old.history...)
			data.preference = old.preference
			data.history = appendPoint(data.history, point{At: time.Now().UnixMilli() - 1})
		}
		b.tokens[scope.Token] = data
	}
	if len(b.tokens) > 128 {
		for key := range b.tokens {
			if key != scope.Token {
				delete(b.tokens, key)
				break
			}
		}
	}
	b.mu.Unlock()
	data.mu.Lock()
	defer data.mu.Unlock()
	resources, previous, err := parse(response.Stdout, data.previous, time.Now())
	if err != nil && err.Error() == "unsupported" {
		return b.r.Publish(ctx, scope.Token, map[string]any{"state": "unsupported", "items": []any{}})
	}
	if err != nil {
		return err
	}
	if !data.initialized {
		if err = b.r.Capability(ctx, "storage.get", map[string]string{"key": "preferences." + scope.HostKey}, &data.preference); err != nil {
			return err
		}
		data.initialized = true
	}
	data.scope = scope
	data.ready = true
	data.resources = resources
	data.previous = previous
	var used *float64
	if resources.Memory != nil && resources.Memory.Available != nil {
		used = ptr((1 - *resources.Memory.Available/resources.Memory.Total) * 100)
	}
	net := pickNetwork(resources, data.preference.Network)
	p := point{At: time.Now().UnixMilli(), CPU: resources.CPU.Usage, Memory: used}
	if net != nil {
		p.Receive, p.Send = net.ReceiveRate, net.SendRate
	}
	p.Swapping = resources.Memory != nil && resources.Memory.SwapOutRate != nil && *resources.Memory.SwapOutRate > 0
	data.history = appendPoint(data.history, p)
	// Save a detached history snapshot: the map lock never nests a sample lock.
	copy := &sample{scope: scope, history: append([]point{}, data.history...), preference: data.preference, initialized: true}
	b.mu.Lock()
	b.history[scope.SessionID] = copy
	if len(b.history) > 128 {
		for key := range b.history {
			if key != scope.SessionID {
				delete(b.history, key)
				break
			}
		}
	}
	b.mu.Unlock()
	return b.publish(ctx, data)
}
func (b *backend) publish(ctx context.Context, data *sample) error {
	t := b.r.T
	s := data.resources
	pref := data.preference
	d := pickDisk(s, pref.Mount)
	net := pickNetwork(s, pref.Network)
	cpuWarning := sustained(data.history, data.scope.Interval, func(p point) bool { return p.CPU != nil && *p.CPU >= 90 })
	memoryWarning := sustained(data.history, data.scope.Interval, func(p point) bool { return p.Memory != nil && *p.Memory >= 90 && p.Swapping })
	warnings := []string{}
	if cpuWarning {
		warnings = append(warnings, t("monitorCpuWarning", nil))
	}
	if memoryWarning {
		warnings = append(warnings, t("monitorMemoryWarning", nil))
	}
	for _, disk := range s.Disks {
		if disk.Percent >= 90 {
			warnings = append(warnings, t("monitorDiskWarning", map[string]any{"mount": disk.Mount, "free": bytes(ptr(disk.Available)), "used": percent(ptr(disk.Percent))}))
		}
		if disk.InodePercent != nil && *disk.InodePercent >= 90 {
			warnings = append(warnings, t("monitorInodeWarning", map[string]any{"mount": disk.Mount}))
		}
	}
	level := func(warning bool) string {
		if warning {
			return "warning"
		}
		return "normal"
	}
	cpuValues := []*float64{}
	for _, p := range data.history {
		cpuValues = append(cpuValues, p.CPU)
	}
	memoryValue, memoryCompact := "—", "—"
	if m := s.Memory; m != nil && m.Available != nil {
		used := ptr((1 - *m.Available/m.Total) * 100)
		memoryCompact = percent(used)
		memoryValue = t("monitorMemoryOverview", map[string]any{"used": memoryCompact, "available": bytes(m.Available)})
	}
	diskLabel, diskValue, diskCompact, diskHelp, diskWarning := t("monitorDisks", nil), "—", "—", t("pluginsDiskHelp", nil), false
	if d != nil {
		diskLabel = d.Mount
		diskCompact = percent(ptr(d.Percent))
		diskValue = diskCompact
		if d.Percent >= 90 {
			diskValue = t("monitorDiskOverview", map[string]any{"used": diskCompact, "free": bytes(ptr(d.Available))})
		}
		diskHelp = t("monitorDiskOverviewHelp", map[string]any{"mount": d.Mount, "inodes": percent(d.InodePercent), "free": bytes(ptr(d.Available))})
		diskWarning = d.Percent >= 90 || (d.InodePercent != nil && *d.InodePercent >= 90)
	}
	rate := func(v *float64) string {
		if v == nil {
			return "—"
		}
		return bytes(v) + "/s"
	}
	networkValue, networkCompact, networkHelp := "↓ — ↑ —", "↕ —", t("monitorNetworkHelp", nil)
	if net != nil {
		networkValue = "↓ " + rate(net.ReceiveRate) + " ↑ " + rate(net.SendRate)
		if net.ReceiveRate != nil && net.SendRate != nil {
			networkCompact = "↕ " + rate(ptr(*net.ReceiveRate+*net.SendRate))
		}
		networkHelp = net.Name + "\n" + networkHelp
	}
	items := []map[string]any{{"id": "cpu", "label": "CPU", "value": percent(s.CPU.Usage), "detail": t("pluginsCpuHelp", nil), "command": "monitor.cpu", "priority": 10, "level": level(cpuWarning), "trend": map[string]any{"values": cpuValues, "ceiling": 100}}, {"id": "memory", "label": t("pluginsMemory", nil), "value": memoryValue, "compactValue": memoryCompact, "detail": t("pluginsMemoryHelp", nil), "command": "monitor.memory", "priority": 10, "level": level(memoryWarning)}, {"id": "disk", "label": diskLabel, "value": diskValue, "compactValue": diskCompact, "detail": diskHelp, "command": "monitor.disk", "level": level(diskWarning)}, {"id": "network", "label": "", "value": networkValue, "compactValue": networkCompact, "detail": networkHelp, "command": "monitor.network"}, {"id": "details", "label": "", "value": t("monitorDetails", nil), "detail": t("name", nil), "command": "monitor.system", "priority": -10}}
	return b.r.Publish(ctx, data.scope.Token, map[string]any{"state": "ready", "items": items, "data": map[string]any{"resources": s, "history": data.history, "interval": data.scope.Interval, "preference": pref, "warnings": warnings}})
}
func (b *backend) Command(ctx context.Context, scope sdk.SessionInfo, command string, raw json.RawMessage) (any, error) {
	if command != "monitor.select" {
		return nil, errors.New("unknown monitor command")
	}
	b.mu.Lock()
	data := b.tokens[scope.Token]
	b.mu.Unlock()
	if data == nil {
		return nil, nil
	}
	data.mu.Lock()
	defer data.mu.Unlock()
	var input map[string]*string
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	next := data.preference
	for key, value := range input {
		if key != "mount" && key != "network" {
			return nil, errors.New("invalid monitor preference")
		}
		if value == nil {
			if key == "mount" {
				next.Mount = ""
			} else {
				next.Network = ""
			}
			continue
		}
		if key == "mount" {
			for _, d := range data.resources.Disks {
				if d.Mount == *value {
					next.Mount = *value
				}
			}
		} else {
			for _, n := range data.resources.Network {
				if n.Name == *value {
					next.Network = *value
				}
			}
		}
	}
	if err := b.r.Capability(ctx, "storage.set", map[string]any{"key": "preferences." + scope.HostKey, "value": next}, nil); err != nil {
		return nil, err
	}
	if next.Network != data.preference.Network {
		for i := range data.history {
			data.history[i].Receive = nil
			data.history[i].Send = nil
		}
	}
	data.preference = next
	return nil, b.publish(ctx, data)
}
func (b *backend) LocaleChanged(ctx context.Context, scopes []sdk.SessionInfo) {
	for _, scope := range scopes {
		b.mu.Lock()
		data := b.tokens[scope.Token]
		b.mu.Unlock()
		if data != nil {
			data.mu.Lock()
			if data.ready {
				_ = b.publish(ctx, data)
			}
			data.mu.Unlock()
		}
	}
}
