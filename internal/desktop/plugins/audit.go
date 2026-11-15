package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"ssh-connect/internal/fsutil"
	"sync"
	"time"
)

type Activity struct {
	Time          string `json:"time"`
	PluginID      string `json:"pluginId"`
	Target        string `json:"target"`
	Capability    string `json:"capability"`
	Executable    string `json:"executable"`
	ArgumentCount int    `json:"argumentCount"`
	Outcome       string `json:"outcome"`
}
type Audit struct {
	mu         sync.Mutex
	path       string
	records    []Activity
	unreadable bool
}

func NewAudit(path string) *Audit {
	audit := &Audit{path: path, records: []Activity{}}
	data, err := regularRead(path, 262144)
	if os.IsNotExist(err) {
		return audit
	}
	var records []Activity
	if err == nil {
		err = json.Unmarshal(data, &records)
		if records == nil || len(records) > 256 {
			err = errors.New("invalid audit file")
		}
		for _, record := range records {
			if record.Time == "" || record.PluginID == "" || record.Target == "" || !programName.MatchString(record.Executable) || (record.Capability != "process.execute" && record.Capability != "terminal.create") || (record.Outcome != "started" && record.Outcome != "succeeded" && record.Outcome != "failed" && record.Outcome != "denied" && record.Outcome != "cancelled") {
				err = errors.New("invalid audit file")
			}
		}
	}
	if err != nil {
		audit.unreadable = true
	} else {
		audit.records = records
	}
	return audit
}
func (a *Audit) Add(record Activity) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.unreadable {
		return errors.New("plugin execution history is unreadable; restore or remove the audit file before granting execution access")
	}
	record.Time = time.Now().UTC().Format(time.RFC3339Nano)
	start := 0
	if len(a.records) > 255 {
		start = len(a.records) - 255
	}
	next := append(append([]Activity{}, a.records[start:]...), record)
	data, err := marshalJSON(next)
	if err != nil {
		return err
	}
	if err = fsutil.AtomicWriteFile(a.path, data, 0600); err != nil {
		return err
	}
	a.records = next
	return nil
}
func (a *Audit) List(id string) []Activity {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := []Activity{}
	for i := len(a.records) - 1; i >= 0 && len(result) < 100; i-- {
		if a.records[i].PluginID == id {
			result = append(result, a.records[i])
		}
	}
	return result
}
