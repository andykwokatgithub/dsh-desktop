package config

import (
	"os"
	"strings"
)

// langChinesePrimary is the Windows primary language identifier (the low 10
// bits of a LANGID) for Chinese: LANG_CHINESE = 0x04. It covers both Simplified
// and Traditional Chinese sublanguages.
const langChinesePrimary uint16 = 0x04

// primaryLangID extracts the primary language identifier from a Windows LANGID,
// i.e. the low 10 bits (LANGIDFROMLANGID / PRIMARYLANGID).
func primaryLangID(langID uint16) uint16 {
	return langID & 0x03FF
}

// isChineseLangID reports whether a Windows LANGID denotes a Chinese primary
// language (Simplified or Traditional).
func isChineseLangID(langID uint16) bool {
	return primaryLangID(langID) == langChinesePrimary
}

// localeEnv returns the first non-empty locale override from LC_ALL, LC_MESSAGES
// or LANG, in that order of precedence; it returns "" when none is set.
func localeEnv() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// isChineseLocaleName reports whether a locale / language name denotes Chinese.
// It accepts the POSIX forms (zh, zh_CN, zh_CN.UTF-8), the hyphenated forms used
// on Windows (zh-CN, zh-TW) and the English name "Chinese", ignoring the
// encoding (.UTF-8) and modifier (@... ) suffixes.
func isChineseLocaleName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name = name[:i]
	}
	return name == "zh" ||
		strings.HasPrefix(name, "zh_") ||
		strings.HasPrefix(name, "zh-") ||
		strings.HasPrefix(name, "chinese")
}

// UseChineseUI reports whether the current system/UI language is Chinese, so
// callers can present their user-facing text in the matching language.
func UseChineseUI() bool { return useChineseUI() }

// useChineseUI reports whether the CLI help should be presented in Chinese. An
// explicit locale environment variable (LC_ALL > LC_MESSAGES > LANG) wins when
// set; otherwise the OS user-interface language decides; it falls back to
// English when nothing can be determined.
func useChineseUI() bool {
	if locale := localeEnv(); locale != "" {
		return isChineseLocaleName(locale)
	}
	if langID, ok := systemUILanguage(); ok {
		return isChineseLangID(langID)
	}
	return false
}
