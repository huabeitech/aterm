package plugins

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

var scriptSubtag = regexp.MustCompile(`^[A-Z][a-z]{3}$`)
var regionSubtag = regexp.MustCompile(`^(?:[A-Z]{2}|[0-9]{3})$`)
var variantSubtag = regexp.MustCompile(`^(?:[a-z0-9]{5,8}|[0-9][a-z0-9]{3})$`)

//go:embed locale-aliases.json
var localeAliasData []byte
var localeAliases = func() map[string]map[string]bool {
	var values map[string][]string
	if err := json.Unmarshal(localeAliasData, &values); err != nil {
		panic(err)
	}
	result := map[string]map[string]bool{}
	for category, names := range values {
		result[category] = map[string]bool{}
		for _, name := range names {
			result[category][name] = true
		}
	}
	return result
}()

// The manifest grammar excludes singleton extension/private-use subtags. Intl
// accepts unknown well-formed subtags in its remaining language/script/region/
// variant subset. Keep those future dictionaries valid; reject aliases rather
// than requiring every subtag to be registered in a language library.
func canonicalLocale(locale string) bool {
	if !localeName.MatchString(locale) || len(locale) > 32 {
		return false
	}
	parts := strings.Split(locale, "-")
	if localeAliases["languages"][parts[0]] {
		return false
	}
	index := 1
	if index < len(parts) && scriptSubtag.MatchString(parts[index]) {
		if localeAliases["scripts"][parts[index]] {
			return false
		}
		index++
	}
	if index < len(parts) && regionSubtag.MatchString(parts[index]) {
		if localeAliases["regions"][parts[index]] {
			return false
		}
		index++
	}
	variants := parts[index:]
	seen := map[string]bool{}
	for _, variant := range variants {
		if !variantSubtag.MatchString(variant) || seen[variant] || localeAliases["variants"][variant] {
			return false
		}
		seen[variant] = true
	}
	return slices.IsSorted(variants)
}
