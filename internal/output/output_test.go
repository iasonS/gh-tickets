package output

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/text"

	"github.com/iasonS/gh-tickets/internal/pulls"
	"github.com/iasonS/gh-tickets/internal/stacks"
	"github.com/iasonS/gh-tickets/internal/tickets"
)

var now = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

var (
	retry = pulls.PullRequest{
		Repo: "acme/api", Number: 41, Title: "PROJ-12 Retry webhooks", Author: "dana",
		Base: "main", Head: "proj-12-retry", Review: pulls.Approved, Checks: pulls.Passing,
		Mergeable: pulls.Mergeable, CreatedAt: now.Add(-48 * time.Hour),
		Parents: []pulls.Parent{{Number: 90, State: pulls.Open, Base: "prod"}},
	}
	queue = pulls.PullRequest{
		Repo: "acme/infra", Number: 7, Title: "Queue for retries", Author: "lee",
		Base: "main", Head: "PROJ-12-queue", Draft: true, Review: pulls.Waiting, Checks: pulls.Running,
		Mergeable: pulls.Unknown, CreatedAt: now.Add(-5 * time.Hour),
	}
	eslint = pulls.PullRequest{
		Repo: "acme/web", Number: 3, Title: "Bump eslint", Author: "bot",
		Base: "main", Head: "deps/eslint", Review: pulls.Waiting, Checks: pulls.Failing,
		Mergeable: pulls.Conflicting, CreatedAt: now.Add(-12 * 24 * time.Hour),
	}
	grouped = []tickets.Ticket{
		{Key: "PROJ-12", PullRequests: []pulls.PullRequest{retry, queue}},
		{Key: "", PullRequests: []pulls.PullRequest{eslint}},
	}
)

func write(color bool, do func(Text)) string {
	var out bytes.Buffer
	do(Text{Out: &out, Color: color, Now: now})
	return out.String()
}

func TestTickets(t *testing.T) {
	got := write(false, func(text Text) { text.Tickets(grouped) })
	want := `PROJ-12
  acme/api#41   PROJ-12 Retry webhooks  dana  main ← proj-12-retry         approved  passing  mergeable    2d
  acme/infra#7  Queue for retries       lee   main ← PROJ-12-queue  draft  waiting   running  unknown      5h

No ticket
  acme/web#3    Bump eslint             bot   main ← deps/eslint           waiting   failing  conflicting  12d
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestColourKeepsTheColumnsAligned(t *testing.T) {
	plain := write(false, func(text Text) { text.Tickets(grouped) })
	coloured := write(true, func(text Text) { text.Tickets(grouped) })

	if strings.Contains(plain, "\033[") {
		t.Fatal("plain output has colour codes")
	}
	if !strings.Contains(coloured, green+"approved"+reset) || !strings.Contains(coloured, red+"conflicting"+reset) {
		t.Fatalf("coloured output lacks colours:\n%q", coloured)
	}
	codes := regexp.MustCompile("\033\\[[0-9;]*m")
	if stripped := codes.ReplaceAllString(coloured, ""); stripped != plain {
		t.Fatalf("without its colours the output differs:\n%s\nwant\n%s", stripped, plain)
	}
}

func TestDetail(t *testing.T) {
	merged := pulls.PullRequest{
		Repo: "acme/infra", Number: 5, Title: "PROJ-12 Add queue", Author: "lee",
		Base: "main", Head: "proj-12-queue", State: pulls.Merged, MergedAt: now.Add(-3 * 24 * time.Hour),
	}
	detail := tickets.Detail{Key: "PROJ-12", Open: []pulls.PullRequest{retry}, Merged: []pulls.PullRequest{merged}}
	got := write(false, func(text Text) { text.Detail(detail, "30d") })
	want := `PROJ-12
Open
  acme/api#41  PROJ-12 Retry webhooks  dana  main ← proj-12-retry    approved  passing  mergeable  2d

Merged in the last 30d
  acme/infra#5  PROJ-12 Add queue  lee  main ← proj-12-queue  merged 3d ago
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDetailWithNothing(t *testing.T) {
	got := write(false, func(text Text) { text.Detail(tickets.Detail{Key: "PROJ-99"}, "7d") })
	want := "PROJ-99\nOpen\n  none\n\nMerged in the last 7d\n  none\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStacks(t *testing.T) {
	tests := pulls.PullRequest{
		Repo: "acme/api", Number: 43, Title: "Retry tests", Author: "dana", Base: "proj-12-retry",
		Head: "proj-12-tests", Review: pulls.Waiting, Checks: pulls.Running, Mergeable: pulls.Mergeable,
		CreatedAt: now.Add(-time.Hour),
	}
	alarms := pulls.PullRequest{
		Repo: "acme/infra", Number: 9, Title: "Alarms", Author: "lee", Base: "old-base", Head: "alarms",
		Review: pulls.Approved, Checks: pulls.Passing, Mergeable: pulls.Mergeable, CreatedAt: now.Add(-time.Hour),
	}
	list := []stacks.Stack{
		{PullRequest: tests, Bottom: "main", Chain: []int{41, 43}},
		{PullRequest: alarms, Bottom: "old-base", Chain: []int{9}, Problem: "parent #8 is merged: retarget the base before merging"},
	}

	lines := strings.Split(strings.TrimSuffix(write(false, func(text Text) { text.Stacks(list) }), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[0], "  acme/api#43 ") || !strings.HasPrefix(lines[2], "  acme/infra#9 ") {
		t.Fatalf("pull request lines missing:\n%s", strings.Join(lines, "\n"))
	}
	if lines[1] != "      main ← #41 ← #43" {
		t.Fatalf("got chain %q", lines[1])
	}
	if lines[3] != "      old-base ← #9  parent #8 is merged: retarget the base before merging" {
		t.Fatalf("got chain %q", lines[3])
	}
}

func TestEmptyLists(t *testing.T) {
	if got := write(false, func(text Text) { text.Tickets(nil) }); got != "No open pull requests.\n" {
		t.Fatalf("tickets: got %q", got)
	}
	if got := write(false, func(text Text) { text.Stacks(nil) }); got != "No stacked pull requests.\n" {
		t.Fatalf("stacks: got %q", got)
	}
}

func TestLongTitleIsShortened(t *testing.T) {
	long := retry
	long.Title = strings.Repeat("word ", 20)
	got := write(false, func(text Text) {
		text.Tickets([]tickets.Ticket{{Key: "PROJ-12", PullRequests: []pulls.PullRequest{long}}})
	})
	if strings.Contains(got, long.Title) || !strings.Contains(got, "...") {
		t.Fatalf("title not shortened:\n%s", got)
	}
}

func TestTitlesShrinkToFitTheTerminal(t *testing.T) {
	// The columns other than the title take 88 characters here.
	fits := func(width int) string {
		var out bytes.Buffer
		Text{Out: &out, Now: now, Width: width}.Tickets(grouped)
		return out.String()
	}

	if got := fits(110); !strings.Contains(got, "PROJ-12 Retry webhooks  dana") {
		t.Fatalf("title shortened although it fits:\n%s", got)
	}
	if got := fits(100); !strings.Contains(got, "PROJ-12 Retry web...  dana") {
		t.Fatalf("title not shortened to the 20 characters left:\n%s", got)
	}
	if got := fits(40); !strings.Contains(got, "PROJ-12 Retry web...  dana") {
		t.Fatalf("title shorter than the 20 character floor:\n%s", got)
	}
	for _, line := range strings.Split(fits(110), "\n") {
		if text.DisplayWidth(line) > 110 {
			t.Fatalf("line wider than 110: %q", line)
		}
	}
}

func TestJSON(t *testing.T) {
	var out bytes.Buffer
	if err := JSON(&out, grouped); err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded[0]["ticket"] != "PROJ-12" || decoded[1]["ticket"] != "" {
		t.Fatalf("unexpected tickets: %s", out.String())
	}
	first := decoded[0]["pullRequests"].([]any)[0].(map[string]any)
	if first["review"] != "approved" || first["repo"] != "acme/api" {
		t.Fatalf("unexpected pull request: %v", first)
	}
	if _, found := first["parents"]; found {
		t.Fatal("parents are in the JSON")
	}
	if _, found := first["mergedAt"]; found {
		t.Fatal("an unmerged pull request has mergedAt")
	}
}

func TestAge(t *testing.T) {
	cases := map[time.Duration]string{
		-time.Minute:        "0m",
		59 * time.Minute:    "59m",
		time.Hour:           "1h",
		47 * time.Hour:      "47h",
		48 * time.Hour:      "2d",
		12 * 24 * time.Hour: "12d",
	}
	for since, want := range cases {
		if got := age(since); got != want {
			t.Errorf("%v: got %q, want %q", since, got, want)
		}
	}
}
