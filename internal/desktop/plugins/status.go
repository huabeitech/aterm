package plugins

import (
	"encoding/json"
	"errors"
	"math"
)

type Trend struct {
	Values  []*float64 `json:"values"`
	Ceiling *float64   `json:"ceiling,omitempty"`
}
type StatusItem struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Value        string   `json:"value"`
	Level        string   `json:"level"`
	Command      string   `json:"command,omitempty"`
	Visibility   string   `json:"visibility,omitempty"`
	Detail       *string  `json:"detail,omitempty"`
	CompactValue *string  `json:"compactValue,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	Trend        *Trend   `json:"trend,omitempty"`
}
type ViewAvailability struct {
	Visible   bool    `json:"visible"`
	Available bool    `json:"available"`
	Message   *string `json:"message,omitempty"`
}
type Status struct {
	PluginID  string                      `json:"pluginId"`
	SessionID string                      `json:"sessionId"`
	State     string                      `json:"state"`
	Items     []StatusItem                `json:"items"`
	Views     map[string]ViewAvailability `json:"views,omitempty"`
	Data      json.RawMessage             `json:"data,omitempty"`
	UpdatedAt *int64                      `json:"updatedAt,omitempty"`
}

func truncateText(s string, limit int) string {
	runes := []rune(s)
	n := 0
	for i, r := range runes {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if n+width > limit {
			return string(runes[:i])
		}
		n += width
	}
	return s
}
func ValidateStatusItems(data json.RawMessage, manifest Manifest) ([]StatusItem, error) {
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || values == nil || len(values) > 16 {
		return nil, errors.New("invalid status items")
	}
	result := make([]StatusItem, 0, len(values))
	ids := map[string]bool{}
	commands := map[string]bool{}
	for _, c := range manifest.Contributes.Commands {
		commands[c.ID] = true
	}
	for _, value := range values {
		item := StatusItem{Level: "normal"}
		for key, target := range map[string]*string{"id": &item.ID, "label": &item.Label, "value": &item.Value} {
			raw, ok := value[key]
			if !ok || string(raw) == "null" || json.Unmarshal(raw, target) != nil || stringLength(*target) > 150 {
				return nil, errors.New("invalid status item")
			}
		}
		if item.ID == "" || ids[item.ID] {
			return nil, errors.New("duplicate or empty status item")
		}
		ids[item.ID] = true
		if raw, ok := value["command"]; ok {
			if json.Unmarshal(raw, &item.Command) != nil || !commands[item.Command] {
				return nil, errors.New("command not declared")
			}
		}
		var level string
		json.Unmarshal(value["level"], &level)
		if level == "warning" || level == "error" {
			item.Level = level
		}
		if raw, ok := value["visibility"]; ok {
			if json.Unmarshal(raw, &item.Visibility) != nil || (item.Visibility != "always" && item.Visibility != "expanded") {
				return nil, errors.New("invalid status visibility")
			}
		}
		for key, target := range map[string]**string{"detail": &item.Detail, "compactValue": &item.CompactValue} {
			var text string
			if raw, ok := value[key]; ok && string(raw) != "null" && json.Unmarshal(raw, &text) == nil {
				limit := 2048
				if key == "compactValue" {
					limit = 150
				}
				text = truncateText(text, limit)
				*target = &text
			}
		}
		if raw, ok := value["priority"]; ok && string(raw) != "null" {
			var number float64
			if json.Unmarshal(raw, &number) == nil {
				number = math.Max(-100, math.Min(100, number))
				item.Priority = &number
			}
		}
		if raw, ok := value["trend"]; ok {
			var trend map[string]json.RawMessage
			if json.Unmarshal(raw, &trend) != nil || trend == nil {
				return nil, errors.New("invalid status trend")
			}
			var values []*float64
			if json.Unmarshal(trend["values"], &values) != nil || values == nil || len(values) > 120 {
				return nil, errors.New("invalid status trend")
			}
			for _, v := range values {
				if v != nil && (*v < 0 || math.IsInf(*v, 0) || math.IsNaN(*v)) {
					return nil, errors.New("invalid status trend")
				}
			}
			item.Trend = &Trend{Values: values}
			if ceiling, exists := trend["ceiling"]; exists {
				var n float64
				if string(ceiling) == "null" || json.Unmarshal(ceiling, &n) != nil || n <= 0 {
					return nil, errors.New("invalid status trend")
				}
				item.Trend.Ceiling = &n
			}
		}
		result = append(result, item)
	}
	return result, nil
}
func ValidateViewAvailability(data json.RawMessage, manifest Manifest) (map[string]ViewAvailability, error) {
	normalized, err := JSONSnapshot(data, 8192)
	if err != nil {
		return nil, err
	}
	var values map[string]map[string]json.RawMessage
	if json.Unmarshal(normalized, &values) != nil || values == nil {
		return nil, errors.New("invalid view availability")
	}
	allowed := map[string]bool{}
	for _, v := range manifest.Contributes.Views {
		if v.Availability == "dynamic" {
			allowed[v.ID] = true
		}
	}
	result := map[string]ViewAvailability{}
	for id, value := range values {
		if !allowed[id] || value == nil {
			return nil, errors.New("invalid view availability")
		}
		for key := range value {
			if key != "visible" && key != "available" && key != "message" {
				return nil, errors.New("invalid view availability")
			}
		}
		var state ViewAvailability
		for key, target := range map[string]*bool{"visible": &state.Visible, "available": &state.Available} {
			raw, ok := value[key]
			if !ok || string(raw) == "null" || json.Unmarshal(raw, target) != nil {
				return nil, errors.New("invalid view availability")
			}
		}
		if raw, ok := value["message"]; ok {
			var message string
			if string(raw) == "null" || json.Unmarshal(raw, &message) != nil || stringLength(message) > 500 {
				return nil, errors.New("invalid view availability")
			}
			state.Message = &message
		}
		result[id] = state
	}
	return result, nil
}
