// Package ui draws what cc-login prints: the table of logins, its JSON and
// TSV forms.
package ui

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Options say how to draw the table.
type Options struct {
	Width int       // the terminal's width; 0 for no limit
	Now   time.Time // the time the expiries and resets count from
}

// styles are the colors of the table.
type styles struct {
	dim, accent, ok, warn, bad lipgloss.Style
}

// newStyles makes the styles for renderer r. $CC_LOGIN_THEME (light or
// dark) overrides the background lipgloss detects.
func newStyles(r *lipgloss.Renderer) styles {
	switch os.Getenv("CC_LOGIN_THEME") {
	case "light":
		r.SetHasDarkBackground(false)
	case "dark":
		r.SetHasDarkBackground(true)
	}

	return styles{
		dim:    r.NewStyle().Foreground(lipgloss.Color("8")),
		accent: r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "215"}).Bold(true),
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
	colDate
	nCols
)

var headers = []string{"", "NAME", "ACCOUNT", "PLAN", "5H LEFT", "WEEK LEFT", "STATUS", "EXPIRES", ""}

// Table writes the table of logins and the notes under it. Colors follow
// the terminal w is (none when w is not a terminal, or with NO_COLOR).
func Table(w io.Writer, views []app.View, opt Options) error {
	if len(views) == 0 {
		_, err := fmt.Fprintln(w, "No logins yet; add one with: cc-login add NAME")
		return err
	}

	r := lipgloss.NewRenderer(w)
	s := newStyles(r)
	rows := make([][]cell, len(views))
	for i, v := range views {
		rows[i] = viewRow(v, s, opt.Now)
	}

	out := fit(rows, s, opt.Width)
	if _, err := fmt.Fprintln(w, out); err != nil {
		return err
	}

	return notes(w, views, s, opt.Now)
}

// fit draws the table within width: without the date column first, then
// with shorter accounts.
func fit(rows [][]cell, s styles, width int) string {
	out := draw(rows, s, true)
	if width <= 0 || lipgloss.Width(out) <= width {
		return out
	}

	out = draw(rows, s, false)
	over := lipgloss.Width(out) - width
	if over <= 0 {
		return out
	}

	longest := 0
	for _, r := range rows {
		longest = max(longest, lipgloss.Width(r[colAccount].text))
	}
	limit := max(longest-over, 8)
	for _, r := range rows {
		r[colAccount].text = truncate(r[colAccount].text, limit)
	}

	return draw(rows, s, false)
}

// draw renders the rows as a table with no borders and two spaces between
// columns.
func draw(rows [][]cell, s styles, withDate bool) string {
	n := nCols
	if !withDate {
		n = colDate
	}

	t := table.New().
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		Headers(headers[:n]...)

	for _, r := range rows {
		texts := make([]string, n)
		for c := range n {
			texts[c] = r[c].text
		}
		t.Row(texts...)
	}

	t.StyleFunc(func(row, col int) lipgloss.Style {
		var st lipgloss.Style
		if row == table.HeaderRow {
			st = s.dim
		} else {
			st = rows[row][col].style
		}

		if col < n-1 {
			st = st.PaddingRight(2)
		}
		if col == colMark {
			st = st.PaddingRight(1)
		}
		return st
	})

	lines := strings.Split(strings.TrimRight(t.Render(), "\n"), "\n")
	for i, l := range lines {
		lines[i] = trimEnd(l)
	}
	return strings.Join(lines, "\n")
}

// trailing is the padding lipgloss adds to the end of a row: spaces, and the
// color codes around them.
var trailing = regexp.MustCompile(`(?:[ ]|\x1b\[[0-9;]*m)+$`)

// trimEnd drops the padding at the end of line; a colored line still ends
// with a reset.
func trimEnd(line string) string {
	out := trailing.ReplaceAllString(line, "")
	if strings.Contains(out, "\x1b[") {
		out += "\x1b[0m"
	}
	return out
}

// viewRow is the row of login v.
func viewRow(v app.View, s styles, now time.Time) []cell {
	row := make([]cell, nCols)
	row[colName] = cell{text: v.Name}
	row[colAccount] = cell{text: orDash(v.Account)}
	row[colPlan] = cell{text: orDash(v.Plan)}

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

	row[colExpires], row[colDate] = expiryCells(v, s, now)
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
// window resets; a cached quota is starred, an unknown one is "?".
func quotaCells(v app.View, s styles) (cell, cell) {
	switch {
	case v.QuotaSource == app.QuotaNone && (v.QuotaGap == app.GapStale || v.QuotaGap == app.GapError):
		return cell{text: "?"}, cell{text: "?"}
	case v.QuotaSource == app.QuotaNone:
		return cell{text: "-"}, cell{text: "-"}
	}

	star := ""
	if v.QuotaSource == app.QuotaCache {
		star = "*"
	}
	return windowCell(v.Quota.FiveHour, star, "15:04", s), windowCell(v.Quota.SevenDay, star, "01-02", s)
}

// windowCell is one quota window: red at 20% left or less, yellow at 50%.
func windowCell(w quota.Window, star, layout string, s styles) cell {
	if w.Used == nil {
		return cell{text: "-"}
	}

	c := cell{text: fmt.Sprintf("%d%%%s", w.Left(), star)}
	if !w.ResetsAt.IsZero() {
		c.text += " " + w.ResetsAt.Local().Format(layout)
	}

	switch left := w.Left(); {
	case left <= 20:
		c.style = s.bad
	case left <= 50:
		c.style = s.warn
	}
	return c
}

// expiryCells are when the sign-in or token expires: relative, and the
// date and time (with the year when it is not this year).
func expiryCells(v app.View, s styles, now time.Time) (cell, cell) {
	tone := s.tone(v.Status)
	if v.Status == app.SignedIn {
		tone = lipgloss.NewStyle()
	}

	switch {
	case v.ExpiresAt.IsZero() && v.Kind == state.APIKey:
		return cell{text: "never"}, cell{}
	case v.ExpiresAt.IsZero():
		return cell{text: "-"}, cell{}
	}

	rel := "in " + Ago(v.ExpiresAt.Sub(now))
	if !v.ExpiresAt.After(now) {
		rel = Ago(now.Sub(v.ExpiresAt)) + " ago"
	}

	t := v.ExpiresAt.Local()
	layout := "2006-01-02 15:04"
	if t.Year() == now.Local().Year() {
		layout = "01-02 15:04"
	}

	return cell{text: rel, style: tone}, cell{text: t.Format(layout), style: s.dim}
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

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// truncate cuts s to n characters, ending with "…" when it cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// notes writes the lines under the table: what to do about logins that
// need it, and where unknown or cached quotas come from.
func notes(w io.Writer, views []app.View, s styles, now time.Time) error {
	var renew, stale, failed, token, cached []string
	hasDefault := false

	for _, v := range views {
		hasDefault = hasDefault || v.Default
		if v.Status != app.SignedIn && v.Kind != state.APIKey {
			renew = append(renew, v.Name)
		}

		switch {
		case v.QuotaSource == app.QuotaCache:
			cached = append(cached, v.Name+cachedAt(v.CachedAt, now))
		case v.QuotaGap == app.GapStale:
			stale = append(stale, v.Name)
		case v.QuotaGap == app.GapError:
			failed = append(failed, v.Name)
		case v.QuotaGap == app.GapToken:
			token = append(token, v.Name)
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
	if len(stale) > 0 {
		lines = append(lines, s.dim.Render("Quota unknown for "+strings.Join(stale, ", ")+": the access token expired; a claude run with the login refreshes it:")+" cc-login run NAME")
	}
	if len(failed) > 0 {
		lines = append(lines, s.dim.Render("Quota request failed for "+strings.Join(failed, ", ")))
	}
	if len(cached) > 0 {
		lines = append(lines, s.dim.Render("* the quota as claude last saw it (quota cache): "+strings.Join(cached, ", ")))
	}
	if len(token) > 0 {
		lines = append(lines, s.dim.Render("Quota unknown for "+strings.Join(token, ", ")+": a setup-token token cannot read it; see it in claude with")+" /usage")
	}
	if !hasDefault {
		lines = append(lines, s.dim.Render("No default login: claude through cc-login does not start. Choose one with:")+" cc-login use NAME")
	}

	if len(lines) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "\n  %s\n", strings.Join(lines, "\n  "))
	return err
}

// cachedAt is " 14:05", or " 11-20 14:05" a day or more ago; "" when unknown.
func cachedAt(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}

	layout := "15:04"
	if now.Sub(at) >= 24*time.Hour {
		layout = "01-02 15:04"
	}
	return " " + at.Local().Format(layout)
}
