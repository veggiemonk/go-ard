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
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "validate":
		err = cmdValidate(ctx, os.Args[2:])
	case "resolve":
		err = cmdResolve(ctx, os.Args[2:])
	case "probe":
		err = cmdProbe(ctx, os.Args[2:])
	case "serve":
		err = cmdServe(ctx, os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "ard: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s %v\n", paint(red, "FAIL"), err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ard — Agentic Resource Discovery conformance tool

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

func cmdValidate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("validate takes one file path or URL")
	}
	target := positional[0]

	header("Manifest validation")
	raw, err := readTarget(ctx, target)
	if err != nil {
		return err
	}
	pass(fmt.Sprintf("read %s (%d bytes)", target, len(raw)))

	report, kind, err := validateDocument(raw)
	if err != nil {
		return err
	}
	pass(fmt.Sprintf("parsed as %s", kind))
	printReport(report)
	return verdict(report)
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

func cmdResolve(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("resolve", flag.ExitOnError)
	skip := flags.Bool("no-predecessor", false, "do not consult the predecessor path")
	positional, err := parseArgs(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("resolve takes one domain")
	}
	domain := positional[0]

	header("Publisher resolution")
	resolver := discover.NewResolver()
	resolver.ConsultPredecessor = !*skip
	result, err := resolver.Resolve(ctx, domain)
	if err != nil {
		return err
	}
	pass(fmt.Sprintf("resolved %s from %s (%s)", domain, result.URL, result.Kind))
	for _, w := range result.Warnings {
		warn(fmt.Sprintf("%s: %s", w.Code, w.Message))
	}
	pass(fmt.Sprintf("the source gave %d entries", len(result.Entries)))

	report := ard.ValidateManifest(result.Manifest)
	printReport(report)
	return verdict(report)
}

func cmdProbe(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("probe", flag.ExitOnError)
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

	header("Registry API validation")
	failures := 0
	failures += probeList(ctx, client)
	failures += probeSearch(ctx, client)
	failures += probeExplore(ctx, client)
	if failures > 0 {
		return fmt.Errorf("%d probe(s) did not conform", failures)
	}
	fmt.Printf("\n%s the registry conforms\n", paint(green, "PASS"))
	return nil
}

func probeList(ctx context.Context, client *registry.Client) int {
	fmt.Println("\n" + paint(bold, "GET /agents (optional)"))
	list, err := client.List(ctx, registry.ListOptions{PageSize: 5})
	if optional(err, "deterministic listing") {
		return 0
	}
	if err != nil {
		fail(err.Error())
		return 1
	}
	pass(fmt.Sprintf("200 with an items array of %d entries", len(list.Items)))
	return countMissingIdentifiers(list.Items, "items")
}

func probeSearch(ctx context.Context, client *registry.Client) int {
	fmt.Println("\n" + paint(bold, "POST /search (required)"))
	request := ard.SearchRequest{
		Query:      ard.Query{Text: "weather forecast"},
		Federation: ard.FederationNone,
		PageSize:   5,
	}
	response, err := client.Search(ctx, request)
	if err != nil {
		fail(err.Error())
		return 1
	}
	pass(fmt.Sprintf("200 with a results array of %d results", len(response.Results)))

	failures := 0
	for i, result := range response.Results {
		if result.Entry.Identifier == "" {
			fail(fmt.Sprintf("results[%d] carries no identifier, which section 5.3.2 requires", i))
			failures++
			continue
		}
		if _, err := ard.ParseURN(result.Entry.Identifier); err != nil {
			fail(fmt.Sprintf("results[%d]: %v", i, err))
			failures++
		}
		if result.Score == nil {
			info(fmt.Sprintf("results[%d] carries no score, which section 5.3.2 allows", i))
			continue
		}
		if *result.Score < 0 || *result.Score > 100 {
			fail(fmt.Sprintf("results[%d] scores %d, outside the range 0 to 100", i, *result.Score))
			failures++
		}
	}
	if failures == 0 && len(response.Results) > 0 {
		pass("every result carries a valid identifier and score")
	}
	if response.PageToken != "" {
		pass("the response carries a pageToken, so the registry pages")
	}
	return failures
}

func probeExplore(ctx context.Context, client *registry.Client) int {
	fmt.Println("\n" + paint(bold, "POST /explore (optional)"))
	request := ard.ExploreRequest{
		ResultType: ard.ExploreShape{Facets: []ard.FacetRequest{{Field: ard.TermType}}},
	}
	response, err := client.Explore(ctx, request)
	if optional(err, "registry introspection") {
		return 0
	}
	if err != nil {
		fail(err.Error())
		return 1
	}
	if response.ResultType != registry.ResultTypeFacets {
		fail(fmt.Sprintf("resultType is %q and not %q", response.ResultType, registry.ResultTypeFacets))
		return 1
	}
	pass(fmt.Sprintf("200 with %d facet(s)", len(response.Facets)))
	for field, facet := range response.Facets {
		pass(fmt.Sprintf("facet %q gave %d bucket(s)", field, len(facet.Buckets)))
	}
	return 0
}

func optional(err error, what string) bool {
	if err == nil {
		return false
	}
	var api *ard.APIError
	if errors.As(err, &api) && (api.HTTPStatus == http.StatusNotFound || api.HTTPStatus == http.StatusNotImplemented) {
		pass(fmt.Sprintf("HTTP %d: %s is optional, so this conforms", api.HTTPStatus, what))
		return true
	}
	return false
}

func cmdServe(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
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
		warn(issue.Message)
	}

	if *source == "" {
		*source = "http://localhost" + *addr
	}
	index := memindex.New(manifest.Entries)
	server := &http.Server{
		Addr:              *addr,
		Handler:           registry.Handler(index, registry.Options{Source: *source}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	fmt.Printf("%s %d entries on %s\n", paint(green, "serving"), index.Len(), *addr)
	fmt.Printf("  POST %s%s\n  POST %s%s\n  GET  %s%s\n",
		*source, registry.RouteSearch, *source, registry.RouteExplore, *source, registry.RouteAgents)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func countMissingIdentifiers(entries []ard.Entry, where string) int {
	missing := 0
	for i, entry := range entries {
		if entry.Identifier == "" {
			fail(fmt.Sprintf("%s[%d] carries no identifier", where, i))
			missing++
		}
	}
	if missing == 0 && len(entries) > 0 {
		pass("every item carries an identifier")
	}
	return missing
}

func printReport(report ard.Report) {
	for _, issue := range report.Errors {
		fail(fmt.Sprintf("%s: %s [%s, section %s]", issue.Path, issue.Message, issue.Code, issue.Section))
	}
	for _, issue := range report.Warnings {
		warn(fmt.Sprintf("%s: %s [%s, section %s]", issue.Path, issue.Message, issue.Code, issue.Section))
	}
	if report.OK() && len(report.Warnings) == 0 {
		pass("no error and no warning")
	}
}

func verdict(report ard.Report) error {
	if !report.OK() {
		return fmt.Errorf("%d error(s), %d warning(s)", len(report.Errors), len(report.Warnings))
	}
	fmt.Printf("\n%s %d error(s), %d warning(s)\n", paint(green, "PASS"), 0, len(report.Warnings))
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

func header(title string) { fmt.Printf("\n%s\n", paint(bold+cyan, "=== "+title+" ===")) }

func pass(message string) { fmt.Printf("  %s %s\n", paint(green, "✓"), message) }

func fail(message string) { fmt.Printf("  %s %s\n", paint(red, "✗"), message) }

func warn(message string) { fmt.Printf("  %s %s\n", paint(amber, "⚠"), message) }

func info(message string) { fmt.Printf("  %s %s\n", paint(cyan, "·"), message) }
