package hostkeys

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Reject duplicate properties rather than silently choosing one fingerprint
// from an ambiguous trust file. Unknown top-level fields remain compatible.
func decodeDocument(data []byte) (document, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return document{}, errors.New("invalid SSH trust document")
	}
	var value document
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return document{}, err
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return document{}, errors.New("duplicate SSH trust document property")
		}
		seen[name] = true
		switch name {
		case "version":
			err = decoder.Decode(&value.Version)
		case "keys":
			token, err = decoder.Token()
			if err != nil || token != json.Delim('{') {
				return document{}, errors.New("invalid SSH trust keys")
			}
			value.Keys = map[string]string{}
			for decoder.More() {
				token, err = decoder.Token()
				if err != nil {
					return document{}, err
				}
				id, ok := token.(string)
				if !ok {
					return document{}, errors.New("invalid SSH trust key identity")
				}
				if _, exists := value.Keys[id]; exists {
					return document{}, errors.New("duplicate SSH trust key identity")
				}
				var fingerprint string
				if err = decoder.Decode(&fingerprint); err != nil {
					return document{}, err
				}
				value.Keys[id] = fingerprint
			}
			_, err = decoder.Token()
		default:
			var ignored json.RawMessage
			err = decoder.Decode(&ignored)
		}
		if err != nil {
			return document{}, err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return document{}, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return document{}, errors.New("trailing SSH trust document data")
	}
	return value, nil
}
