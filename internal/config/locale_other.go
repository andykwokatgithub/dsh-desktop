//go:build !windows

package config

// systemUILanguage reports no OS user-interface language on non-Windows
// platforms; callers then fall back to the LANG/LC_ALL/LC_MESSAGES environment
// variables.
func systemUILanguage() (uint16, bool) {
	return 0, false
}
