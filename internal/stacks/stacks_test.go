package stacks

import (
	"reflect"
	"testing"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

func pr(number int, base, head string, parents ...pulls.Parent) pulls.PullRequest {
	return pulls.PullRequest{
		Repo: "acme/api", Number: number, Base: base, Head: head,
		DefaultBranch: "trunk", State: pulls.Open, Parents: parents,
	}
}

func open(number int, base string) pulls.Parent {
	return pulls.Parent{Number: number, State: pulls.Open, Base: base}
}

func TestPullRequestsOnTheDefaultOrAReleaseBranchAreNotStacked(t *testing.T) {
	prs := []pulls.PullRequest{pr(1, "trunk", "a"), pr(2, "dev", "b"), pr(3, "prod", "dev")}
	if got := Find(prs, DefaultReleaseBranches); len(got) != 0 {
		t.Fatalf("got %+v, want nothing", got)
	}
}

func TestChainFollowsOpenParents(t *testing.T) {
	prs := []pulls.PullRequest{
		pr(44, "b", "c", open(43, "a")),
		pr(41, "trunk", "a"),
		pr(43, "a", "b", open(41, "trunk")),
	}
	want := []Stack{
		{PullRequest: prs[2], Bottom: "trunk", Chain: []int{41, 43}},
		{PullRequest: prs[0], Bottom: "trunk", Chain: []int{41, 43, 44}},
	}
	if got := Find(prs, DefaultReleaseBranches); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestChainStopsAtAReleaseBranchThatHasItsOwnPullRequest(t *testing.T) {
	// dev is the head of an open release pull request into prod; that is not a stack.
	prs := []pulls.PullRequest{
		pr(41, "dev", "a", open(90, "prod")),
		pr(43, "a", "b", open(41, "dev")),
	}
	got := Find(prs, DefaultReleaseBranches)
	if len(got) != 1 || got[0].Bottom != "dev" || !reflect.DeepEqual(got[0].Chain, []int{41, 43}) {
		t.Fatalf("got %+v, want dev ← #41 ← #43", got)
	}
}

func TestParentOutsideTheResultsEndsTheChain(t *testing.T) {
	prs := []pulls.PullRequest{pr(44, "b", "c", open(43, "a"))}
	got := Find(prs, DefaultReleaseBranches)
	if len(got) != 1 || got[0].Bottom != "a" || !reflect.DeepEqual(got[0].Chain, []int{43, 44}) {
		t.Fatalf("got %+v, want a ← #43 ← #44", got)
	}
}

func TestMergedOrClosedParentIsFlagged(t *testing.T) {
	prs := []pulls.PullRequest{
		pr(50, "x", "y", pulls.Parent{Number: 48, State: pulls.Merged, Base: "trunk"}),
		pr(60, "p", "q", pulls.Parent{Number: 58, State: pulls.Closed, Base: "trunk"}),
	}
	got := Find(prs, DefaultReleaseBranches)
	want := []string{
		"parent #48 is merged: retarget the base before merging",
		"parent #58 is closed: retarget the base before merging",
	}
	if len(got) != 2 || got[0].Problem != want[0] || got[1].Problem != want[1] {
		t.Fatalf("got %+v, want problems %q", got, want)
	}
	if got[0].Bottom != "x" || !reflect.DeepEqual(got[0].Chain, []int{50}) {
		t.Fatalf("got %+v, want x ← #50", got[0])
	}
}

func TestOpenParentWinsOverAnOlderMergedOne(t *testing.T) {
	prs := []pulls.PullRequest{
		pr(50, "x", "y", pulls.Parent{Number: 48, State: pulls.Merged, Base: "trunk"}, open(49, "trunk")),
	}
	got := Find(prs, DefaultReleaseBranches)
	if len(got) != 1 || got[0].Problem != "" {
		t.Fatalf("got %+v, want no problem", got)
	}
}

func TestBranchesThatTargetEachOtherDoNotLoop(t *testing.T) {
	prs := []pulls.PullRequest{
		pr(1, "a", "b", open(2, "b")),
		pr(2, "b", "a", open(1, "a")),
	}
	if got := Find(prs, DefaultReleaseBranches); len(got) != 2 {
		t.Fatalf("got %+v, want both pull requests", got)
	}
}
