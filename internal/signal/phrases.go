package signal

import (
	"embed"
	"strings"
)

//go:embed phrases
var phraseFS embed.FS

// loadPhrases reads a phrase file from the embedded phrases/ directory.
// Returns one phrase per non-empty line.
func loadPhrases(name string) []string {
	data, err := phraseFS.ReadFile("phrases/" + name)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
