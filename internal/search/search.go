// Package search builds the GitHub search queries for pull requests and runs them.
package search

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

// Client reads from GitHub. Package github has the real one.
type Client interface {
	IsOrganization(ctx context.Context, login string) (bool, error)
	PullRequests(ctx context.Context, query string, withParents bool) (Result, error)
}

// Result is what one search found. Total can be larger than the number of pull requests,
// because GitHub returns at most 1000 results for one search.
type Result struct {
	PullRequests []pulls.PullRequest
	Total        int
}

// Scope says whose pull requests to search.
type Scope struct {
	Owner  string   // a user or organisation, used when Repos is empty
	Repos  []string // owner/name
	Mine   bool     // only pull requests you authored
	Review bool     // only pull requests waiting for your review
}

// Where turns a scope into search qualifiers, for example "org:acme author:@me".
func Where(ctx context.Context, client Client, scope Scope) (string, error) {
	qualifiers, err := placeQualifiers(ctx, client, scope)
	if err != nil {
		return "", err
	}
	if scope.Mine {
		qualifiers = append(qualifiers, "author:@me")
	}
	if scope.Review {
		qualifiers = append(qualifiers, "review-requested:@me")
	}
	return strings.Join(qualifiers, " "), nil
}

func placeQualifiers(ctx context.Context, client Client, scope Scope) ([]string, error) {
	if len(scope.Repos) > 0 {
		var qualifiers []string
		for _, repo := range scope.Repos {
			qualifiers = append(qualifiers, "repo:"+repo)
		}
		return qualifiers, nil
	}
	if scope.Owner == "" {
		return nil, errors.New("no owner or repository to search")
	}

	isOrganization, err := client.IsOrganization(ctx, scope.Owner)
	if err != nil {
		return nil, err
	}
	if isOrganization {
		return []string{"org:" + scope.Owner}, nil
	}
	return []string{"user:" + scope.Owner}, nil
}

// Open finds the open pull requests that match the qualifiers.
func Open(ctx context.Context, client Client, where string) (Result, error) {
	return client.PullRequests(ctx, "is:pr is:open "+where, false)
}

// OpenWithParents is Open plus, for each pull request, the pull requests whose head branch is
// its base branch. It takes about twice as long.
func OpenWithParents(ctx context.Context, client Client, where string) (Result, error) {
	return client.PullRequests(ctx, "is:pr is:open "+where, true)
}

// MergedSince finds the pull requests that match the qualifiers and were merged on or after
// the day of since, in UTC.
func MergedSince(ctx context.Context, client Client, where string, since time.Time) (Result, error) {
	day := since.UTC().Format("2006-01-02")
	return client.PullRequests(ctx, "is:pr is:merged merged:>="+day+" "+where, false)
}

// ParseSince reads a period such as "30d" (days) or "12h" (any Go duration).
func ParseSince(value string) (time.Duration, error) {
	period, err := parsePeriod(value)
	if err != nil {
		return 0, fmt.Errorf("--since %q: use days such as 30d or hours such as 12h", value)
	}
	if period <= 0 {
		return 0, fmt.Errorf("--since %q: must be more than zero", value)
	}
	return period, nil
}

func parsePeriod(value string) (time.Duration, error) {
	days, isDays := strings.CutSuffix(value, "d")
	if !isDays {
		return time.ParseDuration(value)
	}
	count, err := strconv.Atoi(days)
	if err != nil {
		return 0, err
	}
	return time.Duration(count) * 24 * time.Hour, nil
}
