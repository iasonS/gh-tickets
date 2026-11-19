// Package tickets finds the ticket key of a pull request and groups pull requests by it.
package tickets

import (
	"regexp"
	"slices"
	"strings"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

// DefaultPattern matches keys such as PROJ-123.
const DefaultPattern = `[A-Z][A-Z0-9]+-\d+`

// Ticket is one ticket key with its pull requests. The key is empty for the group of pull
// requests that have no key.
type Ticket struct {
	Key          string              `json:"ticket"`
	PullRequests []pulls.PullRequest `json:"pullRequests"`
}

// Detail is one ticket with its open pull requests and the ones merged lately.
type Detail struct {
	Key    string              `json:"ticket"`
	Open   []pulls.PullRequest `json:"open"`
	Merged []pulls.PullRequest `json:"merged"`
}

// Key returns the first key in the title, else the first key in the head branch, else "".
func Key(pr pulls.PullRequest, pattern *regexp.Regexp) string {
	if key := pattern.FindString(pr.Title); key != "" {
		return key
	}
	return pattern.FindString(pr.Head)
}

// Group puts pull requests under their ticket key. Keys come in sorted order and the pull
// requests without a key come last.
func Group(prs []pulls.PullRequest, pattern *regexp.Regexp) []Ticket {
	byKey := map[string][]pulls.PullRequest{}
	for _, pr := range prs {
		key := Key(pr, pattern)
		byKey[key] = append(byKey[key], pr)
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		if key != "" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if _, found := byKey[""]; found {
		keys = append(keys, "")
	}

	grouped := make([]Ticket, 0, len(keys))
	for _, key := range keys {
		pulls.Sort(byKey[key])
		grouped = append(grouped, Ticket{Key: key, PullRequests: byKey[key]})
	}
	return grouped
}

// ForKey returns the pull requests whose ticket key is key, ignoring case.
func ForKey(prs []pulls.PullRequest, pattern *regexp.Regexp, key string) []pulls.PullRequest {
	matching := []pulls.PullRequest{}
	for _, pr := range prs {
		if strings.EqualFold(Key(pr, pattern), key) {
			matching = append(matching, pr)
		}
	}
	pulls.Sort(matching)
	return matching
}
