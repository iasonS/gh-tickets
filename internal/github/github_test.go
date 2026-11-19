package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

// fakeGitHub answers GraphQL requests with the given bodies, in order, and keeps the
// variables of each request.
type fakeGitHub struct {
	bodies    []string
	variables []map[string]any
}

func (f *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	var request struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, err
	}
	f.variables = append(f.variables, request.Variables)
	body := f.bodies[len(f.variables)-1]
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func newClient(t *testing.T, fake *fakeGitHub) *Client {
	t.Helper()
	graphQL, err := api.NewGraphQLClient(api.ClientOptions{Host: "github.com", AuthToken: "test", Transport: fake})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{GraphQL: graphQL}
}

const firstPage = `{"data": {"search": {
  "issueCount": 2,
  "pageInfo": {"hasNextPage": true, "endCursor": "c1"},
  "nodes": [
    {"number": 41, "title": "PROJ-12 Retry webhooks", "url": "https://github.com/acme/api/pull/41",
     "state": "OPEN", "isDraft": false, "createdAt": "2026-11-17T10:00:00Z", "mergedAt": null,
     "baseRefName": "main", "headRefName": "proj-12-retry", "author": {"login": "dana"},
     "reviewDecision": "APPROVED", "latestOpinionatedReviews": {"nodes": []},
     "statusCheckRollup": {"state": "SUCCESS"}, "mergeable": "MERGEABLE",
     "repository": {"nameWithOwner": "acme/api", "defaultBranchRef": {"name": "main"}},
     "baseRef": {"associatedPullRequests": {"nodes": [{"number": 90, "state": "OPEN", "baseRefName": "prod"}]}}},
    {}
  ]}}}`

const secondPage = `{"data": {"search": {
  "issueCount": 2,
  "pageInfo": {"hasNextPage": false, "endCursor": "c2"},
  "nodes": [
    {"number": 41, "title": "moved down a page", "url": "https://github.com/acme/api/pull/41"},
    {"number": 12, "title": "Queue for retries", "url": "https://github.com/acme/infra/pull/12",
     "state": "OPEN", "isDraft": true, "createdAt": "2026-11-19T07:00:00Z", "mergedAt": null,
     "baseRefName": "gone", "headRefName": "PROJ-12-queue", "author": null,
     "reviewDecision": null,
     "latestOpinionatedReviews": {"nodes": [{"state": "APPROVED"}, {"state": "CHANGES_REQUESTED"}]},
     "statusCheckRollup": null, "mergeable": "UNKNOWN",
     "repository": {"nameWithOwner": "acme/infra", "defaultBranchRef": {"name": "main"}},
     "baseRef": null}
  ]}}}`

func TestPullRequestsReadsEveryPage(t *testing.T) {
	fake := &fakeGitHub{bodies: []string{firstPage, secondPage}}
	result, err := newClient(t, fake).PullRequests(context.Background(), "is:pr is:open org:acme", true)
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.variables) != 2 || fake.variables[0]["after"] != nil || fake.variables[1]["after"] != "c1" {
		t.Fatalf("pages were not requested one after the other: %v", fake.variables)
	}
	if fake.variables[0]["query"] != "is:pr is:open org:acme" || fake.variables[0]["withParents"] != true {
		t.Fatalf("query or withParents not passed on: %v", fake.variables[0])
	}
	if result.Total != 2 || len(result.PullRequests) != 2 {
		t.Fatalf("got total %d and %d pull requests, want 2 and 2", result.Total, len(result.PullRequests))
	}

	want := pulls.PullRequest{
		Repo: "acme/api", Number: 41, Title: "PROJ-12 Retry webhooks", URL: "https://github.com/acme/api/pull/41",
		Author: "dana", Base: "main", Head: "proj-12-retry", DefaultBranch: "main", State: pulls.Open,
		Review: pulls.Approved, Checks: pulls.Passing, Mergeable: pulls.Mergeable,
		CreatedAt: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
		Parents:   []pulls.Parent{{Number: 90, State: pulls.Open, Base: "prod"}},
	}
	if got := result.PullRequests[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	second := result.PullRequests[1]
	if second.Author != "ghost" || !second.Draft || second.Review != pulls.ChangesRequested ||
		second.Checks != pulls.NoChecks || second.Mergeable != pulls.Unknown || second.Parents != nil {
		t.Fatalf("missing values were not read as such: %+v", second)
	}
}

func TestIsOrganization(t *testing.T) {
	fake := &fakeGitHub{bodies: []string{
		`{"data": {"repositoryOwner": {"__typename": "Organization"}}}`,
		`{"data": {"repositoryOwner": {"__typename": "User"}}}`,
		`{"data": {"repositoryOwner": null}}`,
	}}
	client := newClient(t, fake)

	if isOrg, err := client.IsOrganization(context.Background(), "acme"); err != nil || !isOrg {
		t.Fatalf("organisation: got %v, %v", isOrg, err)
	}
	if isOrg, err := client.IsOrganization(context.Background(), "dana"); err != nil || isOrg {
		t.Fatalf("user: got %v, %v", isOrg, err)
	}
	if _, err := client.IsOrganization(context.Background(), "nobody"); err == nil {
		t.Fatal("an unknown owner was accepted")
	}
}

func TestReview(t *testing.T) {
	cases := []struct {
		decision string
		reviews  []string
		want     string
	}{
		{"APPROVED", nil, pulls.Approved},
		{"CHANGES_REQUESTED", nil, pulls.ChangesRequested},
		{"REVIEW_REQUIRED", []string{"APPROVED"}, pulls.Waiting},
		{"", []string{"APPROVED"}, pulls.Approved},
		{"", []string{"APPROVED", "CHANGES_REQUESTED"}, pulls.ChangesRequested},
		{"", nil, pulls.Waiting},
	}
	for _, c := range cases {
		var n node
		n.ReviewDecision = c.decision
		for _, state := range c.reviews {
			n.LatestOpinionatedReviews.Nodes = append(n.LatestOpinionatedReviews.Nodes, struct {
				State string `json:"state"`
			}{state})
		}
		if got := review(n); got != c.want {
			t.Errorf("%q %v: got %q, want %q", c.decision, c.reviews, got, c.want)
		}
	}
}

func TestChecks(t *testing.T) {
	cases := map[string]string{
		"SUCCESS": pulls.Passing, "FAILURE": pulls.Failing, "ERROR": pulls.Failing,
		"PENDING": pulls.Running, "EXPECTED": pulls.Running,
	}
	for state, want := range cases {
		var n node
		n.StatusCheckRollup = &struct {
			State string `json:"state"`
		}{state}
		if got := checks(n); got != want {
			t.Errorf("%s: got %q, want %q", state, got, want)
		}
	}
	if got := checks(node{}); got != pulls.NoChecks {
		t.Errorf("no rollup: got %q, want %q", got, pulls.NoChecks)
	}
}
