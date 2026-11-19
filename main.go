// gh-tickets is a GitHub CLI extension that groups open pull requests by ticket key, so the
// pull requests of one ticket across many repositories can be seen together. It only reads.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"

	"github.com/iasonS/gh-tickets/internal/github"
	"github.com/iasonS/gh-tickets/internal/output"
	"github.com/iasonS/gh-tickets/internal/search"
	"github.com/iasonS/gh-tickets/internal/stacks"
	"github.com/iasonS/gh-tickets/internal/tickets"
)

const commands = `Commands:
  gh tickets [flags]             open pull requests grouped by ticket key
  gh tickets show KEY [flags]    one ticket: open pull requests and the ones merged lately
  gh tickets stacked [flags]     pull requests stacked on another pull request's branch
`

// options holds every flag; each command registers only the ones it uses.
type options struct {
	owner           string
	repos           repoList
	mine            bool
	review          bool
	json            bool
	pattern         string
	since           string
	releaseBranches string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "show" {
		return show(args[1:])
	}
	if len(args) > 0 && args[0] == "stacked" {
		return stacked(args[1:])
	}
	return list(args)
}

func list(args []string) error {
	var opts options
	flags := newFlags("gh tickets [flags]", &opts)
	addPatternFlag(flags, &opts)
	flags.Parse(args)
	if flags.NArg() > 0 {
		return fmt.Errorf("unknown command %q\n\n%s", flags.Arg(0), commands)
	}
	pattern, err := compilePattern(opts.pattern)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, where, err := connect(ctx, opts)
	if err != nil {
		return err
	}
	open, err := search.Open(ctx, client, where)
	if err != nil {
		return err
	}
	warnIfCut(open)

	grouped := tickets.Group(open.PullRequests, pattern)
	if opts.json {
		return output.JSON(os.Stdout, grouped)
	}
	textOutput().Tickets(grouped)
	return nil
}

func show(args []string) error {
	var opts options
	flags := newFlags("gh tickets show KEY [flags]", &opts)
	addPatternFlag(flags, &opts)
	flags.StringVar(&opts.since, "since", "30d", "also list pull requests merged within this period: days (30d) or hours (12h)")

	// The key may come before or after the flags.
	key := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		key, args = args[0], args[1:]
	}
	flags.Parse(args)
	if key == "" {
		key = flags.Arg(0)
	}
	if key == "" {
		return errors.New("which ticket? usage: gh tickets show KEY [flags]")
	}

	detail, err := fetchDetail(context.Background(), opts, key)
	if err != nil {
		return err
	}
	if opts.json {
		return output.JSON(os.Stdout, detail)
	}
	textOutput().Detail(detail, opts.since)
	return nil
}

func fetchDetail(ctx context.Context, opts options, key string) (tickets.Detail, error) {
	pattern, err := compilePattern(opts.pattern)
	if err != nil {
		return tickets.Detail{}, err
	}
	period, err := search.ParseSince(opts.since)
	if err != nil {
		return tickets.Detail{}, err
	}
	client, where, err := connect(ctx, opts)
	if err != nil {
		return tickets.Detail{}, err
	}

	open, err := search.Open(ctx, client, where)
	if err != nil {
		return tickets.Detail{}, err
	}
	warnIfCut(open)
	merged, err := search.MergedSince(ctx, client, where, time.Now().Add(-period))
	if err != nil {
		return tickets.Detail{}, err
	}
	warnIfCut(merged)

	return tickets.Detail{
		Key:    key,
		Open:   tickets.ForKey(open.PullRequests, pattern, key),
		Merged: tickets.ForKey(merged.PullRequests, pattern, key),
	}, nil
}

func stacked(args []string) error {
	var opts options
	flags := newFlags("gh tickets stacked [flags]", &opts)
	flags.StringVar(&opts.releaseBranches, "release-branches", strings.Join(stacks.DefaultReleaseBranches, ","),
		"comma-separated branches that pull requests normally target; any other base counts as stacked")
	flags.Parse(args)

	ctx := context.Background()
	client, where, err := connect(ctx, opts)
	if err != nil {
		return err
	}
	open, err := search.OpenWithParents(ctx, client, where)
	if err != nil {
		return err
	}
	warnIfCut(open)

	found := stacks.Find(open.PullRequests, splitList(opts.releaseBranches))
	if opts.json {
		return output.JSON(os.Stdout, found)
	}
	textOutput().Stacks(found)
	return nil
}

func newFlags(usage string, opts *options) *flag.FlagSet {
	flags := flag.NewFlagSet("gh tickets", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s\n\n%s\nFlags:\n", usage, commands)
		flags.PrintDefaults()
	}
	flags.StringVar(&opts.owner, "owner", "", "user or organisation to search (default: the owner of the current repository)")
	flags.Var(&opts.repos, "repo", "search only this repository, as owner/name; repeat for more")
	flags.BoolVar(&opts.mine, "mine", false, "only pull requests you authored")
	flags.BoolVar(&opts.review, "review", false, "only pull requests waiting for your review")
	flags.BoolVar(&opts.json, "json", false, "write JSON")
	return flags
}

func addPatternFlag(flags *flag.FlagSet, opts *options) {
	flags.StringVar(&opts.pattern, "pattern", tickets.DefaultPattern, "regular expression of a ticket key, looked for in the title, then in the head branch")
}

func compilePattern(pattern string) (*regexp.Regexp, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("--pattern: %w", err)
	}
	return compiled, nil
}

// connect makes the GitHub client and turns the flags into search qualifiers.
func connect(ctx context.Context, opts options) (search.Client, string, error) {
	scope, err := scopeOf(opts)
	if err != nil {
		return nil, "", err
	}
	graphQL, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, "", err
	}
	client := &github.Client{GraphQL: graphQL}
	where, err := search.Where(ctx, client, scope)
	if err != nil {
		return nil, "", err
	}
	return client, where, nil
}

func scopeOf(opts options) (search.Scope, error) {
	if opts.mine && opts.review {
		return search.Scope{}, errors.New("use --mine or --review, not both")
	}
	if opts.owner != "" && len(opts.repos) > 0 {
		return search.Scope{}, errors.New("use --owner or --repo, not both")
	}

	scope := search.Scope{Owner: opts.owner, Repos: opts.repos, Mine: opts.mine, Review: opts.review}
	if scope.Owner != "" || len(scope.Repos) > 0 {
		return scope, nil
	}
	current, err := repository.Current()
	if err != nil {
		return search.Scope{}, fmt.Errorf("give --owner or --repo, or run this inside a GitHub repository: %w", err)
	}
	scope.Owner = current.Owner
	return scope, nil
}

func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func warnIfCut(result search.Result) {
	if result.Total <= len(result.PullRequests) {
		return
	}
	fmt.Fprintf(os.Stderr, "GitHub search returns at most 1000 results: showing %d of %d pull requests\n",
		len(result.PullRequests), result.Total)
}

func textOutput() output.Text {
	terminal := term.FromEnv()
	out := output.Text{Out: os.Stdout, Color: terminal.IsColorEnabled(), Now: time.Now()}
	if !terminal.IsTerminalOutput() {
		return out
	}
	if width, _, err := terminal.Size(); err == nil {
		out.Width = width
	}
	return out
}

// repoList collects every --repo flag.
type repoList []string

func (r *repoList) String() string {
	return strings.Join(*r, ",")
}

func (r *repoList) Set(value string) error {
	if strings.Count(value, "/") != 1 {
		return fmt.Errorf("want owner/name, got %q", value)
	}
	*r = append(*r, value)
	return nil
}
