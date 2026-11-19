// Package github reads pull requests through the GitHub GraphQL API. It only reads.
package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/iasonS/gh-tickets/internal/pulls"
	"github.com/iasonS/gh-tickets/internal/search"
)

// Client is the search.Client that talks to GitHub. go-gh takes the host and the token
// from the gh login.
type Client struct {
	GraphQL *api.GraphQLClient
}

const ownerQuery = `query($login: String!) {
  repositoryOwner(login: $login) { __typename }
}`

// One search returns every field gh-tickets shows, so there is no request per pull request.
// Pages are small because GitHub answers 502 when one page takes longer than about ten
// seconds, and these fields are slow to compute on large organisations. The parents roughly
// double the time of a page, so only the stacked command asks for them.
const pullRequestsQuery = `query($query: String!, $after: String, $withParents: Boolean!) {
  search(query: $query, type: ISSUE, first: 25, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {
        number
        title
        url
        state
        isDraft
        createdAt
        mergedAt
        baseRefName
        headRefName
        author { login }
        reviewDecision
        latestOpinionatedReviews(first: 20, writersOnly: true) { nodes { state } }
        statusCheckRollup { state }
        mergeable
        repository { nameWithOwner defaultBranchRef { name } }
        baseRef @include(if: $withParents) {
          associatedPullRequests(first: 5, orderBy: {field: CREATED_AT, direction: DESC}) {
            nodes { number state baseRefName }
          }
        }
      }
    }
  }
}`

type ownerResponse struct {
	RepositoryOwner *struct {
		Typename string `json:"__typename"`
	} `json:"repositoryOwner"`
}

type searchResponse struct {
	Search struct {
		IssueCount int `json:"issueCount"`
		PageInfo   struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []node `json:"nodes"`
	} `json:"search"`
}

type node struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	State       string    `json:"state"`
	IsDraft     bool      `json:"isDraft"`
	CreatedAt   time.Time `json:"createdAt"`
	MergedAt    time.Time `json:"mergedAt"`
	BaseRefName string    `json:"baseRefName"`
	HeadRefName string    `json:"headRefName"`
	Author      *struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewDecision           string `json:"reviewDecision"`
	LatestOpinionatedReviews struct {
		Nodes []struct {
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	StatusCheckRollup *struct {
		State string `json:"state"`
	} `json:"statusCheckRollup"`
	Mergeable  string `json:"mergeable"`
	Repository struct {
		NameWithOwner    string `json:"nameWithOwner"`
		DefaultBranchRef *struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	} `json:"repository"`
	BaseRef *struct {
		AssociatedPullRequests struct {
			Nodes []struct {
				Number      int    `json:"number"`
				State       string `json:"state"`
				BaseRefName string `json:"baseRefName"`
			} `json:"nodes"`
		} `json:"associatedPullRequests"`
	} `json:"baseRef"`
}

// IsOrganization tells an organisation from a user.
func (c *Client) IsOrganization(ctx context.Context, login string) (bool, error) {
	var response ownerResponse
	variables := map[string]any{"login": login}
	if err := c.GraphQL.DoWithContext(ctx, ownerQuery, variables, &response); err != nil {
		return false, err
	}
	if response.RepositoryOwner == nil {
		return false, fmt.Errorf("no user or organisation %q on GitHub", login)
	}
	return response.RepositoryOwner.Typename == "Organization", nil
}

// PullRequests runs one search and reads every page of it. withParents adds, for each pull
// request, the pull requests whose head branch is its base branch.
func (c *Client) PullRequests(ctx context.Context, query string, withParents bool) (search.Result, error) {
	var result search.Result
	seen := map[string]bool{}
	variables := map[string]any{"query": query, "after": nil, "withParents": withParents}
	for {
		var page searchResponse
		if err := c.GraphQL.DoWithContext(ctx, pullRequestsQuery, variables, &page); err != nil {
			return search.Result{}, err
		}
		result.Total = page.Search.IssueCount
		for _, pr := range convertAll(page.Search.Nodes) {
			// Results move between pages when pull requests open during a long search, so one
			// can come twice.
			if seen[pr.URL] {
				continue
			}
			seen[pr.URL] = true
			result.PullRequests = append(result.PullRequests, pr)
		}
		if !page.Search.PageInfo.HasNextPage {
			return result, nil
		}
		variables["after"] = page.Search.PageInfo.EndCursor
	}
}

func convertAll(nodes []node) []pulls.PullRequest {
	var prs []pulls.PullRequest
	for _, n := range nodes {
		// A search result that is not a pull request comes back as an empty object.
		if n.Number == 0 {
			continue
		}
		prs = append(prs, convert(n))
	}
	return prs
}

func convert(n node) pulls.PullRequest {
	return pulls.PullRequest{
		Repo:          n.Repository.NameWithOwner,
		Number:        n.Number,
		Title:         n.Title,
		URL:           n.URL,
		Author:        author(n),
		Base:          n.BaseRefName,
		Head:          n.HeadRefName,
		DefaultBranch: defaultBranch(n),
		State:         strings.ToLower(n.State),
		Draft:         n.IsDraft,
		Review:        review(n),
		Checks:        checks(n),
		Mergeable:     mergeable(n.Mergeable),
		CreatedAt:     n.CreatedAt,
		MergedAt:      n.MergedAt,
		Parents:       parents(n),
	}
}

func author(n node) string {
	// A deleted account has no author; GitHub shows it as ghost.
	if n.Author == nil {
		return "ghost"
	}
	return n.Author.Login
}

func defaultBranch(n node) string {
	if n.Repository.DefaultBranchRef == nil {
		return ""
	}
	return n.Repository.DefaultBranchRef.Name
}

func review(n node) string {
	switch n.ReviewDecision {
	case "APPROVED":
		return pulls.Approved
	case "CHANGES_REQUESTED":
		return pulls.ChangesRequested
	case "REVIEW_REQUIRED":
		return pulls.Waiting
	}

	// GitHub gives no decision when the repository does not require reviews, so go by the
	// latest review of each reviewer instead.
	var states []string
	for _, r := range n.LatestOpinionatedReviews.Nodes {
		states = append(states, r.State)
	}
	if slices.Contains(states, "CHANGES_REQUESTED") {
		return pulls.ChangesRequested
	}
	if slices.Contains(states, "APPROVED") {
		return pulls.Approved
	}
	return pulls.Waiting
}

func checks(n node) string {
	if n.StatusCheckRollup == nil {
		return pulls.NoChecks
	}
	switch n.StatusCheckRollup.State {
	case "SUCCESS":
		return pulls.Passing
	case "FAILURE", "ERROR":
		return pulls.Failing
	case "PENDING", "EXPECTED":
		return pulls.Running
	}
	return pulls.NoChecks
}

func mergeable(state string) string {
	switch state {
	case "MERGEABLE":
		return pulls.Mergeable
	case "CONFLICTING":
		return pulls.Conflicting
	}
	return pulls.Unknown
}

func parents(n node) []pulls.Parent {
	// The base branch is gone, for example deleted after its own pull request merged.
	if n.BaseRef == nil {
		return nil
	}
	var found []pulls.Parent
	for _, p := range n.BaseRef.AssociatedPullRequests.Nodes {
		found = append(found, pulls.Parent{Number: p.Number, State: strings.ToLower(p.State), Base: p.BaseRefName})
	}
	return found
}
