package config

import "testing"

func TestPrimaryLangID(t *testing.T) {
	cases := []struct {
		langID uint16
		want   uint16
	}{
		{0x0804, 0x04}, // zh-CN (Simplified): primary Chinese
		{0x0404, 0x04}, // zh-TW (Traditional): primary Chinese
		{0x0409, 0x09}, // en-US: primary English
		{0x0000, 0x00}, // neutral
	}
	for _, c := range cases {
		if got := primaryLangID(c.langID); got != c.want {
			t.Errorf("primaryLangID(%#x) = %#x, want %#x", c.langID, got, c.want)
		}
	}
}

func TestIsChineseLangID(t *testing.T) {
	if !isChineseLangID(0x0804) { // zh-CN
		t.Error("0x0804 should be treated as Chinese")
	}
	if !isChineseLangID(0x0404) { // zh-TW
		t.Error("0x0404 should be treated as Chinese")
	}
	if isChineseLangID(0x0409) { // en-US
		t.Error("0x0409 should not be treated as Chinese")
	}
	if isChineseLangID(0x0411) { // ja-JP
		t.Error("0x0411 should not be treated as Chinese")
	}
}

func TestIsChineseLocaleName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"zh", true},
		{"zh_CN", true},
		{"zh_CN.UTF-8", true},
		{"zh_CN.utf8", true},
		{"zh-CN", true},
		{"zh_TW", true},
		{"Chinese", true},
		{"chinese", true},
		{"en_US", false},
		{"en_US.UTF-8", false},
		{"ja_JP", false},
		{"C", false},
		{"POSIX", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isChineseLocaleName(c.name); got != c.want {
			t.Errorf("isChineseLocaleName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLocaleEnvPrecedence(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "en_US.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := localeEnv(); got != "zh_CN.UTF-8" {
		t.Errorf("localeEnv() = %q, want LC_ALL to win (%q)", got, "zh_CN.UTF-8")
	}
}

func TestUseChineseUIEnvOverride(t *testing.T) {
	// Clear the higher-precedence vars so LANG alone drives the decision,
	// independent of the host OS UI language.
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")

	t.Setenv("LANG", "en_US.UTF-8")
	if UseChineseUI() {
		t.Error("LANG=en_US should select English")
	}

	t.Setenv("LANG", "zh_CN.UTF-8")
	if !UseChineseUI() {
		t.Error("LANG=zh_CN should select Chinese")
	}

	// LC_ALL takes precedence over LANG.
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if UseChineseUI() {
		t.Error("LC_ALL=en_US should win over LANG=zh_CN and select English")
	}
}
