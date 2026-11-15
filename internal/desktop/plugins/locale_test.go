package plugins

import (
	"encoding/json"
	"testing"
)

func TestManifestCanonicalLocales(t *testing.T) {
	for _, item := range []struct {
		locale string
		valid  bool
	}{
		{"en", true}, {"zh-CN", true}, {"pt-BR", true}, {"xx", true}, {"xx-YY", true}, {"en-abcde", true}, {"xx-Latn-YY-abcde", true}, {"en-1901", true}, {"und", true}, {"en-Latn", true}, {"en-Qaaa", true}, {"en-QQ", true}, {"en-123", true},
		{"en-us", false}, {"cmn", false}, {"sh", false}, {"zh-cmn", false}, {"sl-rozaj-biske", false}, {"en-ABCD", false}, {"en-abcde-abcde", false},
	} {
		t.Run(item.locale, func(t *testing.T) {
			plugin := fixturePackage(t)
			plugin.Manifest.I18n = &LocalizationConfig{DefaultLocale: item.locale, Locales: map[string]string{item.locale: "locales/" + item.locale + ".json"}}
			data, _ := json.Marshal(plugin.Manifest)
			_, err := ParseManifest(data)
			if (err == nil) != item.valid {
				t.Fatalf("locale acceptance=%v; want %v (%v)", err == nil, item.valid, err)
			}
		})
	}
}
