package render

import (
	"os"
	"strings"
)

// Color applies ANSI styling, or not. Ports cli.py's Color.
type Color struct {
	Enabled bool
}

var colorCodes = map[string]string{
	"reset":   "\033[0m",
	"bold":    "\033[1m",
	"dim":     "\033[2m",
	"red":     "\033[31m",
	"green":   "\033[32m",
	"yellow":  "\033[33m",
	"blue":    "\033[34m",
	"magenta": "\033[35m",
	"cyan":    "\033[36m",
	"gray":    "\033[90m",
}

// ColorFromMode resolves --color. isTTY is passed in rather than probed so the
// decision stays testable; auto also honours NO_COLOR.
func ColorFromMode(mode string, isTTY bool) Color {
	switch mode {
	case "always":
		return Color{Enabled: true}
	case "never":
		return Color{Enabled: false}
	default:
		_, noColor := os.LookupEnv("NO_COLOR")
		return Color{Enabled: isTTY && !noColor}
	}
}

// Apply wraps text in the given styles. Unknown style names are ignored, and
// with no styles the text is returned unchanged -- both match cli.py.
func (c Color) Apply(text string, styles ...string) string {
	if !c.Enabled || len(styles) == 0 {
		return text
	}
	var prefix strings.Builder
	for _, style := range styles {
		if code, ok := colorCodes[style]; ok {
			prefix.WriteString(code)
		}
	}
	return prefix.String() + text + colorCodes["reset"]
}
