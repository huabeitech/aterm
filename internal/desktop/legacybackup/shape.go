package legacybackup

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Go's JSON decoder accepts null for scalar fields. Electron rejects it and
// requires the original host fields, so validate presence and type explicitly.
func validateShape(plaintext []byte) error {
	var value struct {
		Library struct {
			Hosts       []map[string]json.RawMessage `json:"hosts"`
			Directories []map[string]json.RawMessage `json:"directories"`
		} `json:"library"`
	}
	if err := json.Unmarshal(plaintext, &value); err != nil {
		return errors.New("invalid legacy library records")
	}
	for _, host := range value.Library.Hosts {
		if err := requiredFields(host, []string{"id", "directoryID", "alias", "hostName", "user", "identityFile", "proxyJump", "jumpHostID", "remark"}, []string{"passwordAuth", "forwardAgent", "compression", "favorite"}, []string{"port", "serverAliveInterval", "serverAliveCountMax", "connectTimeout"}); err != nil {
			return err
		}
	}
	for _, directory := range value.Library.Directories {
		if err := requiredFields(directory, []string{"id", "parentID", "name"}, nil, []string{"sortOrder"}); err != nil {
			return err
		}
	}
	return nil
}
func requiredFields(value map[string]json.RawMessage, texts, flags, numbers []string) error {
	present := func(key string) bool {
		return len(value[key]) > 0 && !bytes.Equal(bytes.TrimSpace(value[key]), []byte("null"))
	}
	for _, key := range texts {
		var text string
		if !present(key) || json.Unmarshal(value[key], &text) != nil {
			return errors.New("invalid legacy text field")
		}
		size := 0
		for _, r := range text {
			size++
			if r > 0xffff {
				size++
			}
		}
		limit := 65536
		if key == "id" || key == "parentID" {
			limit = 200
		}
		if size > limit {
			return errors.New("legacy text field exceeds its size limit")
		}
	}
	for _, key := range flags {
		var flag bool
		if !present(key) || json.Unmarshal(value[key], &flag) != nil {
			return errors.New("invalid legacy boolean field")
		}
	}
	for _, key := range numbers {
		var number int64
		if !present(key) || json.Unmarshal(value[key], &number) != nil || number > 9007199254740991 || number < -9007199254740991 {
			return errors.New("invalid legacy numeric field")
		}
	}
	return nil
}
