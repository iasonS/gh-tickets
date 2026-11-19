// Package pulls holds the pull request fields that gh-tickets shows.
package pulls

import (
	"slices"
	"strings"
	"time"
)

// States of a pull request.
const (
	Open   = "open"
	Merged = "merged"
	Closed = "closed"
)

// Review states.
const (
	Approved         = "approved"
	ChangesRequested = "changes requested"
	Waiting          = "waiting"
)

// Check states.
const (
	Passing  = "passing"
	Failing  = "failing"
	Running  = "running"
	NoChecks = "none"
)

// Merge states. GitHub works out whether a pull request can merge in the background, so a
// fresh read can say unknown.
const (
	Mergeable   = "mergeable"
	Conflicting = "conflicting"
	Unknown     = "unknown"
)

// PullRequest is one pull request as gh-tickets shows it.
type PullRequest struct {
	Repo          string    `json:"repo"` // owner/name
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	Author        string    `json:"author"`
	Base          string    `json:"base"`
	Head          string    `json:"head"`
	DefaultBranch string    `json:"defaultBranch"`
	State         string    `json:"state"`
	Draft         bool      `json:"draft"`
	Review        string    `json:"review"`
	Checks        string    `json:"checks"`
	Mergeable     string    `json:"mergeable"`
	CreatedAt     time.Time `json:"createdAt"`
	MergedAt      time.Time `json:"mergedAt,omitzero"`

	// Parents are the pull requests whose head branch is this one's base branch, newest
	// first. Only a stacked pull request has a real parent among them, so they stay out of
	// the JSON.
	Parents []Parent `json:"-"`
}

// Parent is a pull request whose head branch is another pull request's base branch.
type Parent struct {
	Number int
	State  string
	Base   string
}

// Sort orders pull requests by repository, then by number.
func Sort(prs []PullRequest) {
	slices.SortFunc(prs, func(a, b PullRequest) int {
		if a.Repo != b.Repo {
			return strings.Compare(a.Repo, b.Repo)
		}
		return a.Number - b.Number
	})
}
