package tickets

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

var pattern = regexp.MustCompile(DefaultPattern)

func TestKey(t *testing.T) {
	cases := []struct {
		name, title, head, want string
	}{
		{"title first", "PROJ-12 Retry webhooks", "OPS-7-retry", "PROJ-12"},
		{"head branch when the title has none", "Retry webhooks", "feature/PROJ-12-retry", "PROJ-12"},
		{"none", "Bump eslint", "deps/eslint", ""},
		{"lower case is not a key by default", "Retry webhooks", "proj-12-retry", ""},
		{"first of two keys", "PROJ-12 and PROJ-13", "", "PROJ-12"},
	}
	for _, c := range cases {
		got := Key(pulls.PullRequest{Title: c.title, Head: c.head}, pattern)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestKeyWithOwnPattern(t *testing.T) {
	own := regexp.MustCompile(`[a-z]+-\d+`)
	got := Key(pulls.PullRequest{Title: "Retry webhooks", Head: "proj-12-retry"}, own)
	if got != "proj-12" {
		t.Fatalf("got %q, want proj-12", got)
	}
}

func TestGroupSortsKeysAndPutsNoTicketLast(t *testing.T) {
	prs := []pulls.PullRequest{
		{Repo: "acme/web", Number: 7, Title: "Bump eslint", Head: "deps/eslint"},
		{Repo: "acme/infra", Number: 12, Title: "Queue for retries", Head: "PROJ-12-queue"},
		{Repo: "acme/api", Number: 41, Title: "PROJ-12 Retry webhooks", Head: "retry"},
		{Repo: "acme/api", Number: 3, Title: "ABC-1 Fix login", Head: "fix"},
	}
	want := []string{
		"ABC-1: acme/api#3",
		"PROJ-12: acme/api#41 acme/infra#12",
		": acme/web#7",
	}
	if got := summary(Group(prs, pattern)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGroupOfNothingIsEmpty(t *testing.T) {
	if got := Group(nil, pattern); got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want an empty list", got)
	}
}

func TestForKeyIgnoresCase(t *testing.T) {
	prs := []pulls.PullRequest{
		{Repo: "acme/infra", Number: 12, Title: "Queue", Head: "PROJ-12-queue"},
		{Repo: "acme/api", Number: 41, Title: "PROJ-12 Retry", Head: "retry"},
		{Repo: "acme/api", Number: 50, Title: "PROJ-120 Other", Head: "other"},
	}
	got := ForKey(prs, pattern, "proj-12")
	if len(got) != 2 || got[0].Number != 41 || got[1].Number != 12 {
		t.Fatalf("got %+v, want acme/api#41 and acme/infra#12", got)
	}
}

func summary(grouped []Ticket) []string {
	var lines []string
	for _, ticket := range grouped {
		var refs []string
		for _, pr := range ticket.PullRequests {
			refs = append(refs, pr.Repo+"#"+strconv.Itoa(pr.Number))
		}
		lines = append(lines, ticket.Key+": "+strings.Join(refs, " "))
	}
	return lines
}
