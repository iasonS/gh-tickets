package search

import (
	"context"
	"slices"
	"testing"
	"time"
)

type fakeClient struct {
	organizations map[string]bool
	queries       []string
}

func (f *fakeClient) IsOrganization(_ context.Context, login string) (bool, error) {
	return f.organizations[login], nil
}

func (f *fakeClient) PullRequests(_ context.Context, query string, withParents bool) (Result, error) {
	if withParents {
		query += " (with parents)"
	}
	f.queries = append(f.queries, query)
	return Result{}, nil
}

func TestWhere(t *testing.T) {
	cases := []struct {
		name  string
		scope Scope
		want  string
	}{
		{"organisation", Scope{Owner: "acme"}, "org:acme"},
		{"user", Scope{Owner: "dana"}, "user:dana"},
		{"repositories", Scope{Owner: "acme", Repos: []string{"acme/api", "acme/infra"}}, "repo:acme/api repo:acme/infra"},
		{"mine", Scope{Owner: "acme", Mine: true}, "org:acme author:@me"},
		{"review", Scope{Owner: "acme", Review: true}, "org:acme review-requested:@me"},
	}
	client := &fakeClient{organizations: map[string]bool{"acme": true}}
	for _, c := range cases {
		got, err := Where(context.Background(), client, c.scope)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWhereWithoutOwnerOrRepository(t *testing.T) {
	if _, err := Where(context.Background(), &fakeClient{}, Scope{}); err == nil {
		t.Fatal("an empty scope was accepted")
	}
}

func TestOpenAndMergedQueries(t *testing.T) {
	client := &fakeClient{}
	since := time.Date(2026, 10, 20, 23, 30, 0, 0, time.FixedZone("UTC-2", -2*60*60))
	if _, err := Open(context.Background(), client, "org:acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := MergedSince(context.Background(), client, "org:acme", since); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWithParents(context.Background(), client, "org:acme"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"is:pr is:open org:acme",
		"is:pr is:merged merged:>=2026-10-21 org:acme",
		"is:pr is:open org:acme (with parents)",
	}
	if !slices.Equal(client.queries, want) {
		t.Fatalf("got %q, want %q", client.queries, want)
	}
}

func TestParseSince(t *testing.T) {
	good := map[string]time.Duration{"30d": 30 * 24 * time.Hour, "12h": 12 * time.Hour, "1d": 24 * time.Hour}
	for value, want := range good {
		got, err := ParseSince(value)
		if err != nil || got != want {
			t.Errorf("%s: got %v, %v; want %v", value, got, err, want)
		}
	}
	for _, value := range []string{"", "d", "thirty", "0d", "-2d", "3w"} {
		if _, err := ParseSince(value); err == nil {
			t.Errorf("%q was accepted", value)
		}
	}
}
