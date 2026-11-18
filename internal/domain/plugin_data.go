package domain

import "encoding/json"

type PluginData struct {
	PluginID  string          `json:"pluginID"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	HostIDs   []string        `json:"hostIDs"`
	UpdatedAt string          `json:"updatedAt"`
}
