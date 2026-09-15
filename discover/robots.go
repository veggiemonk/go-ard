package discover

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// RobotsDirective is the entry-source directive of robots.txt. See section 5.1.
const RobotsDirective = "agentmap"

// ScanRobots reads the Agentmap directives of a robots.txt, in order and without repeats.
func ScanRobots(r io.Reader, base *url.URL) ([]string, error) {
	var sources []string
	seen := map[string]bool{}
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 0, 64*1024), int(DefaultMaxBytes))
	for lines.Scan() {
		value, ok := agentmapValue(lines.Text())
		if !ok {
			continue
		}
		target := resolveReference(base, value)
		if seen[target] {
			continue
		}
		seen[target] = true
		sources = append(sources, target)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("discover: read robots.txt: %w", err)
	}
	return sources, nil
}

func agentmapValue(line string) (string, bool) {
	if comment := strings.IndexByte(line, '#'); comment >= 0 {
		line = line[:comment]
	}
	name, value, found := strings.Cut(line, ":")
	if !found || !strings.EqualFold(strings.TrimSpace(name), RobotsDirective) {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}
