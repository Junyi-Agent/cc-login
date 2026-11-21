// Package ui draws what cc-login list prints: the table of logins
// (table.go) and its JSON (format.go).
package ui

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// styles are the colors of the table.
type styles struct {
	dim, accent, ok, warn, bad lipgloss.Style
}

// newStyles makes the styles for renderer r.
func newStyles(r *lipgloss.Renderer) styles {
	return styles{
		dim:    r.NewStyle().Foreground(lipgloss.Color("8")),
		accent: r.NewStyle().Foreground(lipgloss.Color("208")).Bold(true),
		ok:     r.NewStyle().Foreground(lipgloss.Color("2")),
		warn:   r.NewStyle().Foreground(lipgloss.Color("3")),
		bad:    r.NewStyle().Foreground(lipgloss.Color("1")),
	}
}

// tone is the style of a status, or of a quota cell.
func (s styles) tone(st app.Status) lipgloss.Style {
	switch st {
	case app.SignedIn:
		return s.ok
	case app.Expiring:
		return s.warn
	case app.Expired:
		return s.bad
	case app.SignedOut:
	}
	return s.dim
}

// cell is one table cell and its style.
type cell struct {
	text  string
	style lipgloss.Style
}

// columns of the table.
const (
	colMark = iota
	colName
	colAccount
	colPlan
	colFive
	colWeek
	colStatus
	colExpires
	nCols
)

var headers = []string{"", "NAME", "ACCOUNT", "PLAN", "5H LEFT", "WEEK LEFT", "STATUS", "EXPIRES"}

// Table writes the table of logins and the notes under it; expiries and
// resets count from now. Colors follow the terminal w is (none when w is
// not a terminal, or with NO_COLOR).
func Table(w io.Writer, views []app.View, now time.Time) error {
	if len(views) == 0 {
		_, err := fmt.Fprintln(w, "No logins yet; add one with: cc-login add NAME")
		return err
	}

	r := lipgloss.NewRenderer(w)
	s := newStyles(r)
	rows := make([][]cell, len(views))
	for i, v := range views {
		rows[i] = viewRow(v, s, now)
	}

	if _, err := fmt.Fprintln(w, draw(rows, s)); err != nil {
		return err
	}

	return notes(w, views, s)
}

// draw lays out the rows under the headers: each column as wide as its
// widest cell, two spaces between columns, one after the default mark.
func draw(rows [][]cell, s styles) string {
	head := make([]cell, nCols)
	for c := range nCols {
		head[c] = cell{text: headers[c], style: s.dim}
	}
	all := append([][]cell{head}, rows...)

	widths := make([]int, nCols)
	for _, r := range all {
		for c := range nCols {
			widths[c] = max(widths[c], lipgloss.Width(r[c].text))
		}
	}

	lines := make([]string, len(all))
	for i, r := range all {
		var b strings.Builder
		for c := range nCols {
			if r[c].text != "" {
				b.WriteString(r[c].style.Render(r[c].text))
			}

			gap := 2
			if c == colMark {
				gap = 1
			}
			b.WriteString(strings.Repeat(" ", widths[c]-lipgloss.Width(r[c].text)+gap))
		}
		lines[i] = strings.TrimRight(b.String(), " ")
	}
	return strings.Join(lines, "\n")
}

// viewRow is the row of login v.
func viewRow(v app.View, s styles, now time.Time) []cell {
	row := make([]cell, nCols)
	row[colName] = cell{text: v.Name}
	row[colAccount] = cell{text: cmp.Or(v.Account, "-")}
	row[colPlan] = cell{text: cmp.Or(v.Plan, "-")}

	if v.Default {
		row[colMark] = cell{text: "▸", style: s.accent}
		row[colName].style = s.accent
	}

	row[colFive], row[colWeek] = quotaCells(v, s)

	mark := "●"
	if v.Status == app.SignedOut {
		mark = "○"
	}
	row[colStatus] = cell{text: mark + " " + statusText(v), style: s.tone(v.Status)}

	row[colExpires] = expiryCell(v, s, now)
	return row
}

// statusText is the STATUS column: a token login that is signed in says token.
func statusText(v app.View) string {
	if v.Kind == state.Token && v.Status.Usable() {
		return "token"
	}
	return strings.ReplaceAll(v.Status.String(), "-", " ")
}

// quotaCells are the 5-hour and weekly cells: the percent left and when the
// window resets; "?" when reading the quota failed.
func quotaCells(v app.View, s styles) (cell, cell) {
	switch {
	case v.Quota != nil:
		return windowCell(v.Quota.FiveHour, "15:04", s), windowCell(v.Quota.SevenDay, "01-02", s)
	case v.QuotaErr != nil:
		return cell{text: "?"}, cell{text: "?"}
	}
	return cell{text: "-"}, cell{text: "-"}
}

// windowCell is one quota window: red at 20% left or less, yellow at 50%.
func windowCell(w *anthropic.Window, layout string, s styles) cell {
	left, ok := w.Left()
	if !ok {
		return cell{text: "-"}
	}

	c := cell{text: fmt.Sprintf("%d%%", left)}
	if w.ResetsAt != nil {
		c.text += " " + w.ResetsAt.Local().Format(layout)
	}

	switch {
	case left <= 20:
		c.style = s.bad
	case left <= 50:
		c.style = s.warn
	}
	return c
}

// expiryCell is when the sign-in or token expires, relative to now (list
// --json has the time).
func expiryCell(v app.View, s styles, now time.Time) cell {
	switch {
	case v.ExpiresAt.IsZero() && v.Kind == state.APIKey:
		return cell{text: "never"}
	case v.ExpiresAt.IsZero():
		return cell{text: "-"}
	}

	c := cell{text: "in " + Ago(v.ExpiresAt.Sub(now))}
	if !v.ExpiresAt.After(now) {
		c.text = Ago(now.Sub(v.ExpiresAt)) + " ago"
	}
	if v.Status != app.SignedIn {
		c.style = s.tone(v.Status)
	}
	return c
}

// Ago is a duration in one unit: minutes under an hour (52m), hours under
// two days (11h), days from there (3d).
func Ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int((d+time.Minute-1)/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
}

// notes writes the lines under the table: what to do about logins that
// need it, and why a quota is unknown.
func notes(w io.Writer, views []app.View, s styles) error {
	var renew, quota []string
	hasDefault := false

	for _, v := range views {
		hasDefault = hasDefault || v.Default
		if v.Status != app.SignedIn && v.Kind != state.APIKey {
			renew = append(renew, v.Name)
		}
		if v.QuotaErr != nil {
			quota = append(quota, s.dim.Render("Quota unknown for "+v.Name+": "+v.QuotaErr.Error()))
		}
	}

	var lines []string
	switch len(renew) {
	case 0:
	case 1:
		lines = append(lines, s.dim.Render("Sign in again with:")+" cc-login renew "+renew[0])
	default:
		lines = append(lines, s.dim.Render("Sign in again with:")+" cc-login renew NAME")
	}
	lines = append(lines, quota...)
	if !hasDefault {
		lines = append(lines, s.dim.Render("No default login: claude through cc-login does not start. Choose one with:")+" cc-login use NAME")
	}

	if len(lines) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "\n  %s\n", strings.Join(lines, "\n  "))
	return err
}
