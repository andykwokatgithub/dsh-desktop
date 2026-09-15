package dpi

import "testing"

// TestSystemDpiDoesNotPanic guards the regression where GetDpiForSystem was
// resolved from shcore.dll (which does not export it), making LazyProc.Addr()
// panic at window creation. It must return a sane DPI and never panic.
func TestSystemDpiDoesNotPanic(t *testing.T) {
	SetAwareness()

	got := SystemDpi()
	if got < 96 || got > 768 {
		t.Fatalf("SystemDpi() = %d, want a sane DPI in [96, 768]", got)
	}
}

func TestScaleToDpi(t *testing.T) {
	cases := []struct {
		value, from, to, want int
	}{
		{100, 96, 96, 100},
		{100, 96, 192, 200},   // 2x
		{100, 192, 96, 50},    // 0.5x
		{1200, 96, 144, 1800}, // 1.5x
		{100, 0, 96, 100},     // from<=0 defaults to 96
		{100, 96, 0, 100},     // to<=0 defaults to 96
	}
	for _, tc := range cases {
		if got := ScaleToDpi(tc.value, tc.from, tc.to); got != tc.want {
			t.Errorf("ScaleToDpi(%d, %d, %d) = %d, want %d", tc.value, tc.from, tc.to, got, tc.want)
		}
	}
}
