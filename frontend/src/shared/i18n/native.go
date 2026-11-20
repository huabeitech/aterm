// Package i18n shares the renderer's native message catalogs with Go dialogs.
package i18n

import (
	"embed"
	"encoding/json"
	"strings"

	"golang.org/x/text/language"
)

//go:embed locales/*/native.json
var nativeCatalogs embed.FS

var nativeLocaleIDs = []string{"en", "de", "es", "fr", "ja", "ko", "pt-BR", "zh-CN", "zh-TW"}

var nativeLocaleMatcher = func() language.Matcher {
	tags := make([]language.Tag, 0, len(nativeLocaleIDs))
	for _, id := range nativeLocaleIDs {
		tags = append(tags, language.MustParse(id))
	}
	return language.NewMatcher(tags)
}()

func NativeMessages(preference string, languages []string) map[string]string {
	if preference == "system" {
		preference = "en"
		for _, name := range languages {
			tag, err := language.Parse(strings.ReplaceAll(name, "_", "-"))
			if err != nil {
				continue
			}
			match, index, _ := nativeLocaleMatcher.Match(tag)
			base, _ := tag.Base()
			matchedBase, _ := match.Base()
			if base == matchedBase {
				preference = nativeLocaleIDs[index]
				break
			}
		}
	}
	messages := make(map[string]string)
	for _, locale := range []string{"en", preference} {
		if data, err := nativeCatalogs.ReadFile("locales/" + locale + "/native.json"); err == nil {
			_ = json.Unmarshal(data, &messages)
		}
	}
	return messages
}
