// Command ard validates ARD documents, resolves a publisher, probes a registry and
// serves one.
//
// It is a second implementation of the conformance modes of the official Python tool in
// conformance/bin/conformance-test, and it agrees with that tool on the example
// manifests of the specification.
//
// Usage:
//
//	ard validate <file|url>      check a manifest or a single entry
//	ard resolve  <domain>        resolve a publisher as a consumer does
//	ard probe    <registry-url>  probe a live registry REST API
//	ard serve    <manifest.json> serve a registry over the manifest
//
// It exits with 0 when the target conforms and with 1 when it does not.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/veggiemonk/go-ard"
	"github.com/veggiemonk/go-ard/discover"
	"github.com/veggiemonk/go-ard/registry"
	"github.com/veggiemonk/go-ard/registry/memindex"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

type cli struct {
	out io.Writer
	err io.Writer
}

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	c := cli{out: out, err: errOut}
	if len(args) == 0 {
		c.usage()
		return 2
	}
	var err error
	switch args[0] {
	case "validate":
		err = c.validate(ctx, args[1:])
	case "resolve":
		err = c.resolve(ctx, args[1:])
	case "probe":
		err = c.probe(ctx, args[1:])
	case "serve":
		err = c.serve(ctx, args[1:])
	case "-h", "--help", "help":
		c.usage()
		return 0
	default:
		fmt.Fprintf(c.err, "ard: unknown command %q\n\n", args[0])
		c.usage()
		return 2
	}
	if err != nil {
		fmt.Fprintf(c.err, "\n%s %v\n", paint(red, "FAIL"), err)
		return 1
	}
	return 0
}

func (c cli) usage() {
	fmt.Fprint(c.err, `ard — Agentic Resource Discovery conformance tool

Usage:
  ard validate <file|url>       check a manifest or a single entry
  ard resolve  <domain>         resolve a publisher as a consumer does (section 5.1)
  ard probe    <registry-url>   probe a live registry REST API (section 5.3)
  ard serve    <manifest.json>  serve a registry over the entries of a manifest

Options of validate:
  none

Options of resolve:
  -no-predecessor   do not consult /.well-known/ai-catalog.json

Options of probe:
  -header "K: V"    send a request header; repeat for several

Options of serve:
  -addr    :9010    listen address
  -source  URL      the source URL that every search result carries

Exit codes: 0 conforms, 1 does not conform, 2 wrong usage.
`)
}

func (c cli) flags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(c.err)
	return flags
}

func parseArgs(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		rest := flags.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func (c cli) validate(ctx context.Context, args []string) error {
	flags := c.flags("validate")
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("validate takes one file path or URL")
	}
	target := positional[0]

	c.header("Manifest validation")
	raw, err := readTarget(ctx, target)
	if err != nil {
		return err
	}
	c.pass(fmt.Sprintf("read %s (%d bytes)", target, len(raw)))

	report, kind, err := validateDocument(raw)
	if err != nil {
		return err
	}
	c.pass(fmt.Sprintf("parsed as %s", kind))
	c.printReport(report)
	return c.verdict(report)
}

func validateDocument(raw []byte) (ard.Report, string, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ard.Report{}, "", fmt.Errorf("the document is no JSON object: %w", err)
	}
	if _, ok := probe[ard.TermEntries]; ok {
		var manifest ard.Manifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return ard.Report{}, "", fmt.Errorf("the document is no manifest: %w", err)
		}
		kind := fmt.Sprintf("a manifest of %d entries", len(manifest.Entries))
		return ard.ValidateManifest(manifest), kind, nil
	}
	if _, ok := probe[ard.TermIdentifier]; ok {
		var entry ard.Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return ard.Report{}, "", fmt.Errorf("the document is no entry: %w", err)
		}
		return ard.Validate(entry), "a single entry", nil
	}
	return ard.Report{}, "", fmt.Errorf("the document carries neither %q nor %q", ard.TermEntries, ard.TermIdentifier)
}

func readTarget(ctx context.Context, target string) ([]byte, error) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return os.ReadFile(target)
	}
	manifest, err := discover.FetchManifest(ctx, target)
	if err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func (c cli) resolve(ctx context.Context, args []string) error {
	flags := c.flags("resolve")
	skip := flags.Bool("no-predecessor", false, "do not consult the predecessor path")
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("resolve takes one domain")
	}
	domain := positional[0]

	c.header("Publisher resolution")
	resolver := discover.NewResolver()
	resolver.ConsultPredecessor = !*skip
	result, err := resolver.Resolve(ctx, domain)
	if err != nil {
		return err
	}
	c.pass(fmt.Sprintf("resolved %s from %s (%s)", domain, result.URL, result.Kind))
	for _, w := range result.Warnings {
		c.warn(fmt.Sprintf("%s: %s", w.Code, w.Message))
	}
	c.pass(fmt.Sprintf("the source gave %d entries", len(result.Entries)))

	report := ard.ValidateManifest(result.Manifest)
	c.printReport(report)
	return c.verdict(report)
}

func (c cli) probe(ctx context.Context, args []string) error {
	flags := c.flags("probe")
	var headers headerList
	flags.Var(&headers, "header", `a request header, such as "Authorization: Bearer x"`)
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("probe takes one registry base URL")
	}
	client, err := headers.client(positional[0])
	if err != nil {
		return err
	}

	c.header("Registry API validation")
	failures := 0
	failures += c.probeList(ctx, client)
	failures += c.probeSearch(ctx, client)
	failures += c.probeExplore(ctx, client)
	if failures > 0 {
		return fmt.Errorf("%d probe(s) did not conform", failures)
	}
	fmt.Fprintf(c.out, "\n%s the registry conforms\n", paint(green, "PASS"))
	return nil
}

func (c cli) probeList(ctx context.Context, client *registry.Client) int {
	fmt.Fprintln(c.out, "\n"+paint(bold, "GET /agents (optional)"))
	list, err := client.List(ctx, registry.ListOptions{PageSize: 5})
	if c.optional(err, "deterministic listing") {
		return 0
	}
	if err != nil {
		c.fail(err.Error())
		return 1
	}
	c.pass(fmt.Sprintf("200 with an items array of %d entries", len(list.Items)))
	return c.countMissingIdentifiers(list.Items, "items")
}

func (c cli) probeSearch(ctx context.Context, client *registry.Client) int {
	fmt.Fprintln(c.out, "\n"+paint(bold, "POST /search (required)"))
	request := ard.SearchRequest{
		Query:      ard.Query{Text: "weather forecast"},
		Federation: ard.FederationNone,
		PageSize:   5,
	}
	response, err := client.Search(ctx, request)
	if err != nil {
		c.fail(err.Error())
		return 1
	}
	c.pass(fmt.Sprintf("200 with a results array of %d results", len(response.Results)))

	failures := 0
	for i, result := range response.Results {
		if result.Entry.Identifier == "" {
			c.fail(fmt.Sprintf("results[%d] carries no identifier, which section 5.3.2 requires", i))
			failures++
			continue
		}
		if _, err := ard.ParseURN(result.Entry.Identifier); err != nil {
			c.fail(fmt.Sprintf("results[%d]: %v", i, err))
			failures++
		}
		if result.Score == nil {
			c.info(fmt.Sprintf("results[%d] carries no score, which section 5.3.2 allows", i))
			continue
		}
		if *result.Score < 0 || *result.Score > 100 {
			c.fail(fmt.Sprintf("results[%d] scores %d, outside the range 0 to 100", i, *result.Score))
			failures++
		}
	}
	if failures == 0 && len(response.Results) > 0 {
		c.pass("every result carries a valid identifier and score")
	}
	if response.PageToken != "" {
		c.pass("the response carries a pageToken, so the registry pages")
	}
	return failures
}

func (c cli) probeExplore(ctx context.Context, client *registry.Client) int {
	fmt.Fprintln(c.out, "\n"+paint(bold, "POST /explore (optional)"))
	request := ard.ExploreRequest{
		ResultType: ard.ExploreShape{Facets: []ard.FacetRequest{{Field: ard.TermType}}},
	}
	response, err := client.Explore(ctx, request)
	if c.optional(err, "registry introspection") {
		return 0
	}
	if err != nil {
		c.fail(err.Error())
		return 1
	}
	if response.ResultType != registry.ResultTypeFacets {
		c.fail(fmt.Sprintf("resultType is %q and not %q", response.ResultType, registry.ResultTypeFacets))
		return 1
	}
	c.pass(fmt.Sprintf("200 with %d facet(s)", len(response.Facets)))
	for field, facet := range response.Facets {
		c.pass(fmt.Sprintf("facet %q gave %d bucket(s)", field, len(facet.Buckets)))
	}
	return 0
}

func (c cli) optional(err error, what string) bool {
	if err == nil {
		return false
	}
	var api *ard.APIError
	if errors.As(err, &api) && (api.HTTPStatus == http.StatusNotFound || api.HTTPStatus == http.StatusNotImplemented) {
		c.pass(fmt.Sprintf("HTTP %d: %s is optional, so this conforms", api.HTTPStatus, what))
		return true
	}
	return false
}

func (c cli) serve(ctx context.Context, args []string) error {
	flags := c.flags("serve")
	addr := flags.String("addr", ":9010", "listen address")
	source := flags.String("source", "", "the source URL that every search result carries")
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("serve takes one manifest path")
	}
	raw, err := os.ReadFile(positional[0])
	if err != nil {
		return err
	}
	var manifest ard.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("the document is no manifest: %w", err)
	}
	for _, issue := range ard.ValidateManifest(manifest).Warnings {
		c.warn(issue.Message)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	if *source == "" {
		*source = "http://" + listener.Addr().String()
	}
	index := memindex.New(manifest.Entries)
	server := &http.Server{
		Handler:           registry.Handler(index, registry.Options{Source: *source}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	fmt.Fprintf(c.out, "%s %d entries on %s\n", paint(green, "serving"), index.Len(), listener.Addr())
	fmt.Fprintf(c.out, "  POST %s%s\n  POST %s%s\n  GET  %s%s\n",
		*source, registry.RouteSearch, *source, registry.RouteExplore, *source, registry.RouteAgents)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (c cli) countMissingIdentifiers(entries []ard.Entry, where string) int {
	missing := 0
	for i, entry := range entries {
		if entry.Identifier == "" {
			c.fail(fmt.Sprintf("%s[%d] carries no identifier", where, i))
			missing++
		}
	}
	if missing == 0 && len(entries) > 0 {
		c.pass("every item carries an identifier")
	}
	return missing
}

func (c cli) printReport(report ard.Report) {
	for _, issue := range report.Errors {
		c.fail(fmt.Sprintf("%s: %s [%s, section %s]", issue.Path, issue.Message, issue.Code, issue.Section))
	}
	for _, issue := range report.Warnings {
		c.warn(fmt.Sprintf("%s: %s [%s, section %s]", issue.Path, issue.Message, issue.Code, issue.Section))
	}
	if report.OK() && len(report.Warnings) == 0 {
		c.pass("no error and no warning")
	}
}

func (c cli) verdict(report ard.Report) error {
	if !report.OK() {
		return fmt.Errorf("%d error(s), %d warning(s)", len(report.Errors), len(report.Warnings))
	}
	fmt.Fprintf(c.out, "\n%s %d error(s), %d warning(s)\n", paint(green, "PASS"), 0, len(report.Warnings))
	return nil
}

type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ", ") }

func (h *headerList) Set(value string) error {
	if !strings.Contains(value, ":") {
		return fmt.Errorf("a header needs the form \"Name: value\", got %q", value)
	}
	*h = append(*h, value)
	return nil
}

func (h headerList) client(base string) (*registry.Client, error) {
	client := &registry.Client{BaseURL: base}
	if len(h) == 0 {
		return client, nil
	}
	client.Header = http.Header{}
	for _, raw := range h {
		name, value, _ := strings.Cut(raw, ":")
		client.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	return client, nil
}

const (
	reset = "\033[0m"
	bold  = "\033[1m"
	red   = "\033[31m"
	green = "\033[32m"
	amber = "\033[33m"
	cyan  = "\033[36m"
)

func paint(color, text string) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	return color + text + reset
}

func (c cli) header(title string) {
	fmt.Fprintf(c.out, "\n%s\n", paint(bold+cyan, "=== "+title+" ==="))
}

func (c cli) pass(message string) { fmt.Fprintf(c.out, "  %s %s\n", paint(green, "✓"), message) }

func (c cli) fail(message string) { fmt.Fprintf(c.out, "  %s %s\n", paint(red, "✗"), message) }

func (c cli) warn(message string) { fmt.Fprintf(c.out, "  %s %s\n", paint(amber, "⚠"), message) }

func (c cli) info(message string) { fmt.Fprintf(c.out, "  %s %s\n", paint(cyan, "·"), message) }
