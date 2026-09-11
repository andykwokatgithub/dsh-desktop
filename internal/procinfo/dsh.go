package procinfo

import (
	"fmt"
	"strings"
)

// DshEvidence records how strongly some text (an image path and/or a command
// line) looks like a DeepSeek Harness instance.
//
// Matched follows the project rule "any DSH keyword is enough"; Strong marks
// the keywords that are dsh-specific on their own -- the npm scope/package, the
// product name, the `dsh web` CLI alias, the installed CLI script. Callers that
// need proof should require Strong, or combine Matched with the dsh HTTP
// fingerprint (docs/dsh-desktop-prd.md FR-01, layer L2).
type DshEvidence struct {
	Matched  bool     // any keyword matched
	Strong   bool     // a dsh-specific keyword matched
	Keywords []string // matched keywords, in detection order
}

// strongDshKeywords are conclusive on their own. They are listed in normalized
// form (lower case, forward slashes folded to backslashes, runs of separators
// collapsed), which is how LooksLikeDsh compares them.
//
// On a real global install the listener's command line is
//
//	"node"   "…\npm\\node_modules\@deepseek-ai\dsh\lib\bin.js" web --no-open --host 127.0.0.1 --port 3080
//
// and its cmd.exe parent carries the shim form:
//
//	cmd /c "dsh web --no-open --host 127.0.0.1 --port 3080"
var strongDshKeywords = []string{
	"@deepseek-ai\\dsh",  // npm scope + package; also covers node_modules\@deepseek-ai\dsh
	"deepseek-harness",   // the product/repository name
	"dsh web",            // CLI alias as printed by the npm shim
	"\\dsh\\lib\\bin.js", // …\@deepseek-ai\dsh\lib\bin.js (source checkout / direct invocation)
}

// bareDshKeyword is a corroborating hint only: dsh-desktop.exe, dshmarket, or
// an unrelated process started with a "--port" argument can all contain it.
const bareDshKeyword = "dsh"

// LooksLikeDsh reports which DSH keywords appear in the given texts (image
// paths and/or command lines). port scopes the "--port <n>" hint to the
// endpoint actually being probed instead of hard-coding 3080.
//
// The result is evidence, never a negative proof: unreadable text simply
// produces an empty DshEvidence and the caller must degrade to weaker layers.
func LooksLikeDsh(port int, texts ...string) DshEvidence {
	var b strings.Builder
	for _, t := range texts {
		b.WriteString(normalizeText(t))
		b.WriteString("\n")
	}
	text := b.String()

	var ev DshEvidence
	seen := map[string]bool{}
	add := func(keyword string, strong bool) {
		if keyword == "" || seen[keyword] || !strings.Contains(text, keyword) {
			return
		}
		seen[keyword] = true
		ev.Keywords = append(ev.Keywords, keyword)
		ev.Matched = true
		if strong {
			ev.Strong = true
		}
	}

	for _, kw := range strongDshKeywords {
		add(kw, true)
	}
	if port > 0 {
		add(fmt.Sprintf("--port %d", port), false)
		add(fmt.Sprintf("--port=%d", port), false)
	}
	if !ev.Strong {
		// Only worth reporting when nothing conclusive matched: every strong
		// keyword already contains "dsh" as a substring.
		add(bareDshKeyword, false)
	}
	return ev
}

// normalizeText lower-cases and folds a command line or path so the keyword
// tests do not depend on quoting, separator style, or repeated separators (the
// npm shim emits paths such as …\npm\\node_modules\…).
func normalizeText(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "/", "\\")
	for strings.Contains(s, "\\\\") {
		s = strings.ReplaceAll(s, "\\\\", "\\")
	}
	return strings.Join(strings.Fields(s), " ")
}
