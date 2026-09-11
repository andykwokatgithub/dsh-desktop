package procinfo

import (
	"strings"
	"testing"
)

// The command lines below are real samples captured on Windows 11 for a running
// `dsh web` instance on 127.0.0.1:3080: the npm shim (cmd.exe) spawns node.exe,
// which owns the listening socket.
const (
	sampleNodeCommandLine  = `"node"   "C:\Users\AndyKwok\AppData\Roaming\npm\\node_modules\@deepseek-ai\dsh\lib\bin.js" web --no-open --host 127.0.0.1 --port 3080`
	sampleShimCommandLine  = `cmd /c "dsh web --no-open --host 127.0.0.1 --port 3080"`
	sampleSelfCommandLine  = `"C:\AndyROOT\Working\dsh-desktop\dsh-desktop.exe" --stop-on-exit`
	sampleOtherCommandLine = `nginx.exe -p C:\nginx --port 3080`
)

func TestLooksLikeDsh(t *testing.T) {
	cases := []struct {
		name        string
		port        int
		texts       []string
		wantMatched bool
		wantStrong  bool
	}{
		{"listener runs the dsh CLI", 3080, []string{sampleNodeCommandLine}, true, true},
		{"npm shim wrapper", 3080, []string{sampleShimCommandLine}, true, true},
		{"installed CLI path only", 3080, []string{`C:\Users\A\AppData\Roaming\npm\node_modules\@deepseek-ai\dsh\lib\bin.js`}, true, true},
		{"source checkout", 3080, []string{`node C:\src\deepseek-harness\apps\cli\dist\bin.js web`}, true, true},
		{"forward slashes are folded", 3080, []string{`node D:/x/node_modules/@deepseek-ai/dsh/lib/bin.js web`}, true, true},
		{"own shell exe is only a weak hint", 3080, []string{sampleSelfCommandLine}, true, false},
		{"unrelated listener on the same port", 3080, []string{sampleOtherCommandLine}, true, false},
		{"port hint is scoped to the probed port", 3080, []string{`nginx.exe --port 4000`}, false, false},
		{"unrelated service", 3080, []string{`python -m http.server 8080`}, false, false},
		{"no texts", 3080, nil, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := LooksLikeDsh(tc.port, tc.texts...)
			if ev.Matched != tc.wantMatched || ev.Strong != tc.wantStrong {
				t.Fatalf("LooksLikeDsh(%d, %q) = {matched %v, strong %v, keywords %v}, want {matched %v, strong %v}",
					tc.port, tc.texts, ev.Matched, ev.Strong, ev.Keywords, tc.wantMatched, tc.wantStrong)
			}
		})
	}
}

func TestLooksLikeDshKeywords(t *testing.T) {
	ev := LooksLikeDsh(3080, sampleNodeCommandLine)
	if !containsKeyword(ev.Keywords, `@deepseek-ai\dsh`) {
		t.Fatalf("keywords = %v, want the npm package path", ev.Keywords)
	}
	if containsKeyword(ev.Keywords, "dsh") {
		t.Fatalf("keywords = %v, want no bare-dsh noise once a strong keyword matched", ev.Keywords)
	}

	ev = LooksLikeDsh(3080, sampleOtherCommandLine)
	if containsKeyword(ev.Keywords, `@deepseek-ai\dsh`) {
		t.Fatalf("an unrelated listener reported a strong keyword: %v", ev.Keywords)
	}
	if !containsKeyword(ev.Keywords, "--port 3080") {
		t.Fatalf("keywords = %v, want the port hint", ev.Keywords)
	}

	ev = LooksLikeDsh(3080, sampleShimCommandLine)
	joined := strings.Join(ev.Keywords, ",")
	if !strings.Contains(joined, "dsh web") || !strings.Contains(joined, "--port 3080") {
		t.Fatalf("keywords = %v, want both the CLI alias and the port hint", ev.Keywords)
	}
}

func TestNormalizeText(t *testing.T) {
	got := normalizeText(`  "node"   "C:/x//@deepseek-ai/Dsh/\\lib\\Bin.JS"  `)
	want := `"node" "c:\x\@deepseek-ai\dsh\lib\bin.js"`
	if got != want {
		t.Fatalf("normalizeText() = %q, want %q", got, want)
	}
}

func containsKeyword(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
