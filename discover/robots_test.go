package discover

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestScanRobots(t *testing.T) {
	base := mustBase(t, "https://example.com/robots.txt")
	cases := []struct {
		name    string
		robots  string
		baseURL *url.URL
		want    []string
	}{
		{
			name:    "one directive",
			robots:  "Agentmap: https://example.com/entries.json\n",
			baseURL: base,
			want:    []string{"https://example.com/entries.json"},
		},
		{
			name:    "the directive name is case insensitive",
			robots:  "AGENTMAP: /a.json\nagentmap: /b.json\nAgentMap: /c.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json", "https://example.com/b.json", "https://example.com/c.json"},
		},
		{
			name:    "order is kept and repeats are dropped",
			robots:  "Agentmap: /b.json\nAgentmap: /a.json\nAgentmap: /b.json\n",
			baseURL: base,
			want:    []string{"https://example.com/b.json", "https://example.com/a.json"},
		},
		{
			name:    "a repeat after resolution is dropped",
			robots:  "Agentmap: /a.json\nAgentmap: https://example.com/a.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "comments and blank lines are ignored",
			robots:  "# entry sources\n\nAgentmap: /a.json  # the catalog\n\n# Agentmap: /never.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "other directives are ignored",
			robots:  "User-agent: *\nDisallow: /private\nSitemap: https://example.com/sitemap.xml\nAgentmap: /a.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "surrounding space is trimmed",
			robots:  "   Agentmap   :    /a.json   \n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "a directive with no value is ignored",
			robots:  "Agentmap:\nAgentmap:    \nAgentmap: /a.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "a name that only starts with the directive is ignored",
			robots:  "Agentmaps: /no.json\nAgentmap-extra: /no.json\nAgentmap: /a.json\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "carriage returns are trimmed",
			robots:  "Agentmap: /a.json\r\nAgentmap: /b.json\r\n",
			baseURL: base,
			want:    []string{"https://example.com/a.json", "https://example.com/b.json"},
		},
		{
			name:    "a last line without a newline",
			robots:  "Agentmap: /a.json",
			baseURL: base,
			want:    []string{"https://example.com/a.json"},
		},
		{
			name:    "no base leaves the value as written",
			robots:  "Agentmap: /a.json\n",
			baseURL: nil,
			want:    []string{"/a.json"},
		},
		{
			name:    "an empty file",
			robots:  "",
			baseURL: base,
			want:    nil,
		},
		{
			name:    "a file with no directive",
			robots:  "User-agent: *\nDisallow: /\n",
			baseURL: base,
			want:    nil,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := ScanRobots(strings.NewReader(test.robots), test.baseURL)
			if err != nil {
				t.Fatalf("ScanRobots: %v", err)
			}
			if !equalStrings(got, test.want) {
				t.Errorf("sources = %v, want %v", got, test.want)
			}
		})
	}
}

func TestScanRobotsReportsAReadFailure(t *testing.T) {
	want := errors.New("broken reader")
	if _, err := ScanRobots(failingReader{want}, nil); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
