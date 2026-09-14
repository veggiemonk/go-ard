package memindex

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	ard "github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/registry"
)

var timestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02",
}

var (
	equalityOperators  = []string{"=", "==", ":"}
	thresholdOperators = []string{"=", "==", ">", ">="}
)

type clause struct {
	field    string
	operator string
	value    string
}

type listFilter struct {
	displayName  []string
	types        []string
	publishers   []string
	createdAfter time.Time
	updatedAfter time.Time
}

type ordering struct {
	field      string
	descending bool
}

func parseListFilter(expression string) (listFilter, error) {
	var filter listFilter
	clauses, err := parseClauses(expression)
	if err != nil {
		return listFilter{}, err
	}
	for _, c := range clauses {
		if err := filter.apply(c); err != nil {
			return listFilter{}, err
		}
	}
	return filter, nil
}

func (f *listFilter) apply(c clause) error {
	switch strings.ToLower(c.field) {
	case "displayname":
		if err := checkOperator(c, equalityOperators); err != nil {
			return err
		}
		for _, value := range splitValues(c.value) {
			f.displayName = append(f.displayName, strings.ToLower(value))
		}
	case "type":
		if err := checkOperator(c, equalityOperators); err != nil {
			return err
		}
		for _, value := range splitValues(c.value) {
			f.types = append(f.types, ard.CanonicalMediaType(value))
		}
	case "publisherid":
		if err := checkOperator(c, equalityOperators); err != nil {
			return err
		}
		f.publishers = append(f.publishers, splitValues(c.value)...)
	case "createdafter":
		moment, err := thresholdValue(c)
		if err != nil {
			return err
		}
		f.createdAfter = moment
	case "updatedafter":
		moment, err := thresholdValue(c)
		if err != nil {
			return err
		}
		f.updatedAfter = moment
	default:
		return fmt.Errorf("%w: appendix A defines no filter field %q", registry.ErrInvalidArgument, c.field)
	}
	return nil
}

func (f listFilter) matches(r record) bool {
	if len(f.displayName) > 0 && !containsAny(strings.ToLower(r.entry.DisplayName), f.displayName) {
		return false
	}
	if len(f.types) > 0 && !slices.Contains(f.types, ard.CanonicalMediaType(r.entry.Type)) {
		return false
	}
	if len(f.publishers) > 0 && !slices.Contains(f.publishers, r.publisher) {
		return false
	}
	if !f.createdAfter.IsZero() && !after(r.created, f.createdAfter) {
		return false
	}
	if !f.updatedAfter.IsZero() && !after(r.updated, f.updatedAfter) {
		return false
	}
	return true
}

func after(moment func() (time.Time, bool), threshold time.Time) bool {
	value, known := moment()
	return known && value.After(threshold)
}

func parseOrderBy(expression string) ([]ordering, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, nil
	}
	var order []ordering
	for _, term := range strings.Split(trimmed, ",") {
		words := strings.Fields(term)
		if len(words) == 0 || len(words) > 2 {
			return nil, fmt.Errorf("%w: %q is no sort term of the form \"field [ASC|DESC]\"", registry.ErrInvalidArgument, term)
		}
		field, ok := orderField(words[0])
		if !ok {
			return nil, fmt.Errorf("%w: %q names no field that this registry sorts on", registry.ErrInvalidArgument, words[0])
		}
		descending, err := orderDirection(words)
		if err != nil {
			return nil, err
		}
		order = append(order, ordering{field: field, descending: descending})
	}
	return order, nil
}

func ordered(order []ordering) func(a, b record) int {
	return func(a, b record) int {
		for _, by := range order {
			difference := cmp.Compare(a.sortKey(by.field), b.sortKey(by.field))
			if by.descending {
				difference = -difference
			}
			if difference != 0 {
				return difference
			}
		}
		return cmp.Compare(a.entry.Identifier, b.entry.Identifier)
	}
}

func (r record) sortKey(field string) string {
	switch field {
	case "displayname":
		return strings.ToLower(r.entry.DisplayName)
	case "identifier":
		return r.entry.Identifier
	case "type":
		return r.entry.Type
	case "version":
		return r.entry.Version
	case "publisherid":
		return r.publisher
	case "updatedat":
		return momentKey(r.updated())
	case "createdat":
		return momentKey(r.created())
	}
	return ""
}

func (r record) updated() (time.Time, bool) {
	moment, err := r.entry.UpdatedAtTime()
	if err != nil {
		return time.Time{}, false
	}
	return moment.UTC(), true
}

func (r record) created() (time.Time, bool) {
	for _, path := range createdPaths {
		values, err := r.values(path)
		if err != nil || len(values) == 0 {
			continue
		}
		if moment, err := parseTimestamp(values[0]); err == nil {
			return moment, true
		}
	}
	return time.Time{}, false
}

func parseClauses(expression string) ([]clause, error) {
	s := &scanner{input: expression}
	var clauses []clause
	for !s.done() {
		if len(clauses) > 0 {
			if err := s.conjunction(); err != nil {
				return nil, err
			}
		}
		field, err := s.word("a filter field name")
		if err != nil {
			return nil, err
		}
		operator, err := s.operator()
		if err != nil {
			return nil, err
		}
		value, err := s.value()
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, clause{field: field, operator: operator, value: value})
	}
	return clauses, nil
}

type scanner struct {
	input string
	at    int
}

func (s *scanner) done() bool {
	s.skipSpace()
	return s.at >= len(s.input)
}

func (s *scanner) skipSpace() {
	for s.at < len(s.input) && isSpace(s.input[s.at]) {
		s.at++
	}
}

func (s *scanner) word(want string) (string, error) {
	s.skipSpace()
	start := s.at
	for s.at < len(s.input) && isWordByte(s.input[s.at]) {
		s.at++
	}
	if start == s.at {
		return "", s.fail(want)
	}
	return s.input[start:s.at], nil
}

func (s *scanner) operator() (string, error) {
	s.skipSpace()
	start := s.at
	for s.at < len(s.input) && isOperatorByte(s.input[s.at]) {
		s.at++
	}
	if start == s.at {
		return "", s.fail("a comparison operator")
	}
	return s.input[start:s.at], nil
}

func (s *scanner) value() (string, error) {
	s.skipSpace()
	if s.at >= len(s.input) {
		return "", s.fail("a filter value")
	}
	if quote := s.input[s.at]; quote == '\'' || quote == '"' {
		return s.quoted(quote)
	}
	start := s.at
	for s.at < len(s.input) && !isSpace(s.input[s.at]) {
		s.at++
	}
	return s.input[start:s.at], nil
}

func (s *scanner) quoted(quote byte) (string, error) {
	s.at++
	start := s.at
	for s.at < len(s.input) && s.input[s.at] != quote {
		s.at++
	}
	if s.at >= len(s.input) {
		return "", fmt.Errorf("%w: the filter expression %q leaves a quoted value unclosed", registry.ErrInvalidArgument, s.input)
	}
	value := s.input[start:s.at]
	s.at++
	return value, nil
}

func (s *scanner) conjunction() error {
	word, err := s.word("the keyword AND")
	if err != nil {
		return err
	}
	if !strings.EqualFold(word, "AND") {
		return fmt.Errorf("%w: the filter expression %q joins two clauses with %q, and appendix A joins them with AND",
			registry.ErrInvalidArgument, s.input, word)
	}
	return nil
}

func (s *scanner) fail(want string) error {
	return fmt.Errorf("%w: the filter expression %q wants %s at offset %d",
		registry.ErrInvalidArgument, s.input, want, s.at)
}

func checkOperator(c clause, allowed []string) error {
	if slices.Contains(allowed, c.operator) {
		return nil
	}
	return fmt.Errorf("%w: the filter field %q takes %s, not %q",
		registry.ErrInvalidArgument, c.field, strings.Join(allowed, " or "), c.operator)
}

func thresholdValue(c clause) (time.Time, error) {
	if err := checkOperator(c, thresholdOperators); err != nil {
		return time.Time{}, err
	}
	moment, err := parseTimestamp(c.value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: the filter field %q takes an ISO 8601 timestamp: %v",
			registry.ErrInvalidArgument, c.field, err)
	}
	return moment, nil
}

func parseTimestamp(value string) (time.Time, error) {
	for _, layout := range timestampLayouts {
		if moment, err := time.Parse(layout, value); err == nil {
			return moment.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is no ISO 8601 timestamp", value)
}

func orderField(name string) (string, bool) {
	switch strings.ToLower(strings.ReplaceAll(name, "_", "")) {
	case "displayname", "name":
		return "displayname", true
	case "identifier", "id":
		return "identifier", true
	case "type":
		return "type", true
	case "version":
		return "version", true
	case "publisherid", "publisher":
		return "publisherid", true
	case "updatedat":
		return "updatedat", true
	case "createdat":
		return "createdat", true
	}
	return "", false
}

func orderDirection(words []string) (bool, error) {
	if len(words) < 2 {
		return false, nil
	}
	switch strings.ToUpper(words[1]) {
	case "ASC":
		return false, nil
	case "DESC":
		return true, nil
	}
	return false, fmt.Errorf("%w: the sort direction %q is neither ASC nor DESC",
		registry.ErrInvalidArgument, words[1])
}

func splitValues(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func momentKey(moment time.Time, known bool) string {
	if !known {
		return ""
	}
	return moment.UTC().Format(time.RFC3339Nano)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isWordByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return c == '_' || c == '.'
}

func isOperatorByte(c byte) bool {
	switch c {
	case '=', '>', '<', '!', ':', '~':
		return true
	}
	return false
}
