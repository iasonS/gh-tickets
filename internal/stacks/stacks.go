// Package stacks finds pull requests that are stacked on another pull request's branch.
package stacks

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/iasonS/gh-tickets/internal/pulls"
)

// DefaultReleaseBranches are the branches that pull requests normally target.
var DefaultReleaseBranches = []string{"main", "master", "dev", "develop", "prod", "production"}

// Stack is one stacked pull request and the chain of open pull requests below it.
type Stack struct {
	PullRequest pulls.PullRequest `json:"pullRequest"`
	Bottom      string            `json:"bottom"` // the branch that the lowest pull request in the chain targets
	Chain       []int             `json:"chain"`  // pull request numbers from the lowest up to this one
	Problem     string            `json:"problem,omitempty"`
}

// Find returns the open pull requests whose base is neither their repository's default
// branch nor a release branch.
func Find(open []pulls.PullRequest, releaseBranches []string) []Stack {
	byNumber := map[string]pulls.PullRequest{}
	for _, pr := range open {
		byNumber[id(pr.Repo, pr.Number)] = pr
	}

	sorted := slices.Clone(open)
	pulls.Sort(sorted)
	found := []Stack{}
	for _, pr := range sorted {
		if !isStacked(pr, releaseBranches) {
			continue
		}
		bottom, chain := chainBelow(pr, byNumber, releaseBranches)
		found = append(found, Stack{PullRequest: pr, Bottom: bottom, Chain: chain, Problem: problem(pr)})
	}
	return found
}

func isStacked(pr pulls.PullRequest, releaseBranches []string) bool {
	return pr.Base != pr.DefaultBranch && !slices.Contains(releaseBranches, pr.Base)
}

// chainBelow follows open parents down until it reaches a pull request that targets a
// release branch, has no open parent, or has a parent outside the search results.
func chainBelow(pr pulls.PullRequest, open map[string]pulls.PullRequest, releaseBranches []string) (string, []int) {
	chain := []int{pr.Number}
	current := pr
	// A step limit, because two branches can target each other.
	for step := 0; step < len(open); step++ {
		if !isStacked(current, releaseBranches) {
			break
		}
		parent, found := openParent(current)
		if !found {
			break
		}
		chain = append([]int{parent.Number}, chain...)
		next, known := open[id(current.Repo, parent.Number)]
		if !known {
			return parent.Base, chain
		}
		current = next
	}
	return current.Base, chain
}

func openParent(pr pulls.PullRequest) (pulls.Parent, bool) {
	for _, parent := range pr.Parents {
		if parent.State == pulls.Open {
			return parent, true
		}
	}
	return pulls.Parent{}, false
}

// problem flags a pull request whose parent is merged or closed: merging it now would
// merge into the parent's old branch instead of the branch the parent went to.
func problem(pr pulls.PullRequest) string {
	if _, found := openParent(pr); found {
		return ""
	}
	for _, parent := range pr.Parents {
		if parent.State == pulls.Merged || parent.State == pulls.Closed {
			return fmt.Sprintf("parent #%d is %s: retarget the base before merging", parent.Number, parent.State)
		}
	}
	return ""
}

func id(repo string, number int) string {
	return repo + "#" + strconv.Itoa(number)
}
