package memindex

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/veggiemonk/go-ard/registry"
)

type cursor struct {
	Print  string `json:"p"`
	Offset int    `json:"o"`
}

func encodeCursor(print string, offset int) string {
	encoded, err := json.Marshal(cursor{Print: print, Offset: offset})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeCursor(token, print string) (int, error) {
	if token == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(token, "="))
	if err != nil {
		return 0, fmt.Errorf("%w: the page token is not base64", registry.ErrBadPageToken)
	}
	var at cursor
	if err := json.Unmarshal(decoded, &at); err != nil {
		return 0, fmt.Errorf("%w: the page token carries no cursor", registry.ErrBadPageToken)
	}
	if at.Print != print {
		return 0, fmt.Errorf("%w: the page token belongs to another query", registry.ErrBadPageToken)
	}
	if at.Offset < 0 {
		return 0, fmt.Errorf("%w: the page token holds a negative offset", registry.ErrBadPageToken)
	}
	return at.Offset, nil
}

func searchPrint(q registry.ResolvedQuery) string {
	var canonical strings.Builder
	canonical.WriteString("search\x00")
	canonical.WriteString(q.Text)
	for _, constraint := range q.Filter {
		canonical.WriteString("\x00")
		canonical.WriteString(constraint.Path.IRI)
		for _, segment := range constraint.Path.Rest {
			canonical.WriteString(".")
			canonical.WriteString(segment)
		}
		for _, value := range constraint.Values {
			canonical.WriteString("\x01")
			canonical.WriteString(value)
		}
	}
	return digest(canonical.String())
}

func listPrint(q registry.ListQuery) string {
	return digest("list\x00" + q.Filter + "\x00" + q.OrderBy)
}

func digest(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}
