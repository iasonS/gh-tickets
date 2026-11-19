// Package output writes tickets, stacks and pull requests as aligned text or as JSON.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/text"

	"github.com/iasonS/gh-tickets/internal/pulls"
	"github.com/iasonS/gh-tickets/internal/stacks"
	"github.com/iasonS/gh-tickets/internal/tickets"
)

const (
	indent        = "  "
	gap           = "  "
	titleColumn   = 1
	maxTitleWidth = 50
	minTitleWidth = 20
	branchWidth   = 20
)

const (
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	reset  = "\033[0m"
)

var stateColors = map[string]string{
	pulls.Approved:         green,
	pulls.ChangesRequested: red,
	pulls.Waiting:          yellow,
	pulls.Passing:          green,
	pulls.Failing:          red,
	pulls.Running:          yellow,
	pulls.NoChecks:         dim,
	pulls.Mergeable:        green,
	pulls.Conflicting:      red,
	pulls.Unknown:          dim,
}

// JSON writes v as indented JSON.
func JSON(out io.Writer, v any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(v)
}

// Text writes aligned lines, coloured when Color is set. With a Width, titles are shortened
// further so that the lines fit it.
type Text struct {
	Out   io.Writer
	Color bool
	Now   time.Time
	Width int // of the terminal; 0 means no limit
}

// Tickets writes each ticket with one line per open pull request.
func (t Text) Tickets(list []tickets.Ticket) {
	if len(list) == 0 {
		fmt.Fprintln(t.Out, "No open pull requests.")
		return
	}

	var rows [][]string
	for i, ticket := range list {
		if i > 0 {
			rows = append(rows, []string{""})
		}
		rows = append(rows, []string{t.paint(bold, ticketName(ticket.Key))})
		rows = append(rows, t.openRows(ticket.PullRequests)...)
	}
	t.writeTable(rows)
}

// Detail writes one ticket: its open pull requests, then the ones merged within since.
func (t Text) Detail(detail tickets.Detail, since string) {
	fmt.Fprintln(t.Out, t.paint(bold, detail.Key))
	t.section("Open", t.openRows(detail.Open))
	fmt.Fprintln(t.Out)
	t.section("Merged in the last "+since, t.mergedRows(detail.Merged))
}

// Stacks writes each stacked pull request with the chain below it.
func (t Text) Stacks(list []stacks.Stack) {
	if len(list) == 0 {
		fmt.Fprintln(t.Out, "No stacked pull requests.")
		return
	}

	var rows [][]string
	for _, stack := range list {
		rows = append(rows, t.openRow(stack.PullRequest))
		rows = append(rows, []string{"      " + chainText(stack) + t.problemText(stack)})
	}
	t.writeTable(rows)
}

func (t Text) section(title string, rows [][]string) {
	fmt.Fprintln(t.Out, title)
	if len(rows) == 0 {
		fmt.Fprintln(t.Out, "  none")
		return
	}
	t.writeTable(rows)
}

// writeTable writes rows with aligned columns. A row of one cell is a heading or a note and
// is written as it is.
func (t Text) writeTable(rows [][]string) {
	shortenTitles(rows, t.titleWidth(rows))
	widths := columnWidths(rows)
	for _, row := range rows {
		if len(row) == 1 {
			fmt.Fprintln(t.Out, row[0])
			continue
		}
		t.writeRow(row, widths)
	}
}

// titleWidth is what is left of the width once the other columns are in, between
// minTitleWidth and maxTitleWidth.
func (t Text) titleWidth(rows [][]string) int {
	if t.Width <= 0 {
		return maxTitleWidth
	}
	widths := columnWidths(rows)
	others := len(indent) + len(gap)*(len(widths)-1)
	for i, width := range widths {
		if i != titleColumn {
			others += width
		}
	}
	return min(maxTitleWidth, max(minTitleWidth, t.Width-others))
}

func (t Text) openRows(prs []pulls.PullRequest) [][]string {
	var rows [][]string
	for _, pr := range prs {
		rows = append(rows, t.openRow(pr))
	}
	return rows
}

func (t Text) openRow(pr pulls.PullRequest) []string {
	return []string{
		t.paint(bold, reference(pr)),
		pr.Title,
		pr.Author,
		branches(pr),
		t.paint(dim, draft(pr)),
		t.paint(stateColors[pr.Review], pr.Review),
		t.paint(stateColors[pr.Checks], pr.Checks),
		t.paint(stateColors[pr.Mergeable], pr.Mergeable),
		t.paint(dim, age(t.Now.Sub(pr.CreatedAt))),
	}
}

func (t Text) mergedRows(prs []pulls.PullRequest) [][]string {
	var rows [][]string
	for _, pr := range prs {
		rows = append(rows, []string{
			t.paint(bold, reference(pr)),
			pr.Title,
			pr.Author,
			branches(pr),
			t.paint(dim, "merged "+age(t.Now.Sub(pr.MergedAt))+" ago"),
		})
	}
	return rows
}

func (t Text) writeRow(row []string, widths []int) {
	padded := make([]string, len(row))
	for i, cell := range row {
		padded[i] = text.PadRight(widths[i], cell)
	}
	fmt.Fprintln(t.Out, strings.TrimRight(indent+strings.Join(padded, gap), " "))
}

func (t Text) problemText(stack stacks.Stack) string {
	if stack.Problem == "" {
		return ""
	}
	return "  " + t.paint(red, stack.Problem)
}

func (t Text) paint(color, s string) string {
	if !t.Color || color == "" || s == "" {
		return s
	}
	return color + s + reset
}

func shortenTitles(rows [][]string, width int) {
	for _, row := range rows {
		if len(row) > titleColumn {
			row[titleColumn] = text.Truncate(width, row[titleColumn])
		}
	}
}

// columnWidths measures the widest cell of each column, leaving out rows of one cell. Colour
// codes take no width.
func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		if len(row) == 1 {
			continue
		}
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], text.DisplayWidth(cell))
		}
	}
	return widths
}

func chainText(stack stacks.Stack) string {
	parts := []string{stack.Bottom}
	for _, number := range stack.Chain {
		parts = append(parts, "#"+strconv.Itoa(number))
	}
	return strings.Join(parts, " ← ")
}

func reference(pr pulls.PullRequest) string {
	return pr.Repo + "#" + strconv.Itoa(pr.Number)
}

func branches(pr pulls.PullRequest) string {
	return text.Truncate(branchWidth, pr.Base) + " ← " + text.Truncate(branchWidth, pr.Head)
}

func draft(pr pulls.PullRequest) string {
	if pr.Draft {
		return "draft"
	}
	return ""
}

func ticketName(key string) string {
	if key == "" {
		return "No ticket"
	}
	return key
}

func age(since time.Duration) string {
	since = max(since, 0)
	if since < time.Hour {
		return strconv.Itoa(int(since.Minutes())) + "m"
	}
	if since < 48*time.Hour {
		return strconv.Itoa(int(since.Hours())) + "h"
	}
	return strconv.Itoa(int(since.Hours())/24) + "d"
}
