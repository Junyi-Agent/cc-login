package listing

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Junyi-Agent/cc-login/internal/quota"
)

// Style is how the table is drawn.
type Style struct {
	Color bool   // ANSI colors (a terminal, and NO_COLOR unset)
	Theme string // "light", "dark" or "" (unknown): the accent of the default login
	Cols  int    // terminal width; 0 when unknown or not colored
}

// Theme reads the terminal theme: $CC_LOGIN_THEME (the zsh shim passes the
// zstyle ':cc-login:' theme there), else COLORFGBG ("FG;BG"): background 7 or
// 15 is light, any other number dark.
func Theme() string {
	if t := os.Getenv("CC_LOGIN_THEME"); t == "light" || t == "dark" {
		return t
	}

	bg := os.Getenv("COLORFGBG")
	if i := strings.LastIndexByte(bg, ';'); i >= 0 {
		bg = bg[i+1:]
	}

	if bg == "7" || bg == "15" {
		return "light"
	}
	if bg != "" && strings.Trim(bg, "0123456789") == "" {
		return "dark"
	}
	return ""
}

// Left is a duration in one unit: minutes under an hour (52m), hours under 2
// days (11h), days from there (3d).
func Left(s int64) string {
	switch {
	case s < 3600:
		return fmt.Sprintf("%dm", (s+59)/60)
	case s < 2*86400:
		return fmt.Sprintf("%dh", s/3600)
	}
	return fmt.Sprintf("%dd", s/86400)
}

// row is one formatted line of the table.
type row struct {
	name, email, plan, state, tone, rel, abs string
	q5, q5t, q7, q7t                         string
}

// footnotes are the lines under the table.
type footnotes struct {
	renew, stale, failed, token, cached []string
}

func strftime(layout string, sec int64) string {
	return time.Unix(sec, 0).Local().Format(layout)
}

// pct is the percent left of a window with used percent used, and its tone.
func pct(used int) (string, string) {
	left := 100 - used
	if used >= 100 {
		left = 0
	}

	tone := ""
	if left <= 20 {
		tone = "bad"
	} else if left <= 50 {
		tone = "warn"
	}

	return fmt.Sprintf("%d%%", left), tone
}

// format turns the logins into rows and collects the footnotes.
func format(logins []Login, now time.Time) ([]row, footnotes) {
	var rows []row
	var fn footnotes
	for _, l := range logins {
		rows = append(rows, formatOne(l, now, &fn))
	}
	return rows, fn
}

// formatOne formats login l and adds it to the footnotes it belongs in.
func formatOne(l Login, now time.Time, fn *footnotes) row {
	n := now.Unix()
	r := row{name: l.Name, email: l.Email, plan: l.Plan, rel: "-", q5: "-", q7: "-"}
	r.state, r.tone = Status(l, now)

	// EXPIRES: relative, and the date and time; the year only when it is not this year.
	if l.Expiry != 0 {
		t := time.Unix(l.Expiry, 0).Local()
		if t.Year() == now.Local().Year() {
			r.abs = t.Format("01-02 15:04")
		} else {
			r.abs = t.Format("2006-01-02 15:04")
		}
		if left := l.Expiry - n; left <= 0 {
			r.rel = Left(-left) + " ago"
		} else {
			r.rel = "in " + Left(left)
		}
	} else if l.Plan == "api" {
		r.rel = "never"
	}

	if r.tone != "ok" && l.Plan != "api" {
		fn.renew = append(fn.renew, l.Name)
	}

	// No quota for a login that is signed out or expired.
	if r.tone == "out" || r.tone == "bad" {
		return r
	}

	// A cached quota is starred, and the footnote says when it was cached.
	star := ""
	if l.Cached {
		star = "*"
		note := l.Name
		if l.CachedAt != 0 {
			layout := "15:04"
			if n-l.CachedAt >= 86400 {
				layout = "01-02 15:04"
			}
			note += " " + strftime(layout, l.CachedAt)
		}
		fn.cached = append(fn.cached, note)
	}

	switch {
	case l.QuotaOK:
		r.q5, r.q5t = quotaCell(l.Quota.FiveHour, star, "15:04", r.q5)
		r.q7, r.q7t = quotaCell(l.Quota.SevenDay, star, "01-02", r.q7)
	case l.QuotaWhy == "expired":
		r.q5, r.q7 = "?", "?"
		fn.stale = append(fn.stale, l.Name)
	case l.QuotaWhy == "error":
		r.q5, r.q7 = "?", "?"
		fn.failed = append(fn.failed, l.Name)
	case l.Token:
		fn.token = append(fn.token, l.Name)
	}

	return r
}

// quotaCell is the cell of window w: the percent left, star, and the reset
// time in layout; empty is the cell when w has no value.
func quotaCell(w quota.Window, star, layout, empty string) (string, string) {
	if w.Used == nil {
		return empty, ""
	}

	cell, tone := pct(*w.Used)
	cell += star
	if w.Reset != 0 {
		cell += " " + strftime(layout, w.Reset)
	}

	return cell, tone
}

func width(s string) int { return utf8.RuneCountInString(s) }

// pad is zsh's ${(r:n:)s}: s cut or padded with spaces to n characters.
func pad(s string, n int) string {
	if w := width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	} else if w > n {
		return string([]rune(s)[:n])
	}
	return s
}

// colors are the ANSI codes of the table; all empty without color.
type colors struct {
	off, dim, bold, acc string
	tint                map[string]string // by tone
}

func newColors(st Style) colors {
	if !st.Color {
		return colors{tint: map[string]string{}}
	}

	c := colors{off: "\x1b[0m", dim: "\x1b[90m", bold: "\x1b[1m"}

	// The accent, in 256-color numbers: orange 130 on a light terminal,
	// 215 on a dark one, and 166, which reads on both, when unknown.
	switch st.Theme {
	case "light":
		c.acc = "\x1b[38;5;130m"
	case "dark":
		c.acc = "\x1b[38;5;215m"
	default:
		c.acc = "\x1b[38;5;166m"
	}

	c.tint = map[string]string{"ok": "\x1b[32m", "warn": "\x1b[33m", "bad": "\x1b[31m", "out": c.dim}
	return c
}

// widths are the column widths; date is 0 when the table has no date column.
type widths struct {
	name, email, plan, q5, q7, state, rel, date int
}

// columnWidths fits the columns to the rows and, when cols is not 0, to a
// terminal cols characters wide.
func columnWidths(rows []row, cols int) widths {
	w := widths{name: 4, email: 7, plan: 4, q5: 7, q7: 9, state: 10, rel: 7}
	for _, r := range rows {
		w.name, w.email, w.plan = max(w.name, width(r.name)), max(w.email, width(r.email)), max(w.plan, width(r.plan))
		w.rel, w.date = max(w.rel, width(r.rel)), max(w.date, width(r.abs))
		w.q5, w.q7 = max(w.q5, width(r.q5)), max(w.q7, width(r.q7))
	}

	// The marker column, then each column and the two spaces before the next;
	// the status column holds the dot and a space before the status.
	total := 2 + w.name + 2 + w.email + 2 + w.plan + 2 + w.q5 + 2 + w.q7 + 2 + 2 + w.state + 2 + w.rel
	if w.date > 0 {
		total += 2 + w.date
	}

	// Too wide: leave out the date and time first, then shorten the accounts.
	if cols > 0 && total > cols && w.date > 0 {
		total -= 2 + w.date
		w.date = 0
	}
	if cols > 0 && total > cols {
		w.email = max(w.email-(total-cols), 8)
	}

	return w
}

// Render prints the table of logins. current is the default login, "" for none.
// It returns the first error that writing to w gave.
func Render(w io.Writer, logins []Login, current string, now time.Time, st Style) error {
	out := &errWriter{w: w}
	if len(logins) == 0 {
		out.println("cc-login: no logins yet; add one with: cc-login add NAME")
		return out.err
	}

	c := newColors(st)
	cols := 0
	if st.Color {
		cols = st.Cols
	}
	rows, fn := format(logins, now)
	cw := columnWidths(rows, cols)

	out.printf("%s  %s  %s  %s  %s  %s    %s  EXPIRES%s\n", c.dim,
		pad("NAME", cw.name), pad("ACCOUNT", cw.email), pad("PLAN", cw.plan),
		pad("5H LEFT", cw.q5), pad("WEEK LEFT", cw.q7), pad("STATUS", cw.state), c.off)
	for _, r := range rows {
		out.println(renderRow(r, r.name == current, cw, c))
	}

	renderFootnotes(out, fn, current, c)
	return out.err
}

// renderRow is the line of row r; isDefault marks it as the default login.
func renderRow(r row, isDefault bool, cw widths, c colors) string {
	var b strings.Builder

	if isDefault {
		b.WriteString(c.acc + "▸" + c.off + " " + c.bold + c.acc + pad(r.name, cw.name) + c.off)
	} else {
		b.WriteString("  " + pad(r.name, cw.name))
	}

	email := r.email
	if width(email) > cw.email {
		email = string([]rune(email)[:cw.email-1]) + "…"
	}
	b.WriteString("  " + pad(email, cw.email) + "  " + pad(r.plan, cw.plan) + "  ")

	for _, q := range []struct {
		cell, tone string
		n          int
	}{{r.q5, r.q5t, cw.q5}, {r.q7, r.q7t, cw.q7}} {
		if q.tone != "" {
			b.WriteString(c.tint[q.tone] + pad(q.cell, q.n) + c.off + "  ")
		} else {
			b.WriteString(pad(q.cell, q.n) + "  ")
		}
	}

	if r.tone == "out" {
		b.WriteString(c.dim + "○" + c.off + " ")
	} else {
		b.WriteString(c.tint[r.tone] + "●" + c.off + " ")
	}
	b.WriteString(pad(r.state, cw.state) + "  ")

	cell := r.rel
	if cw.date > 0 && r.abs != "" {
		cell = pad(cell, cw.rel)
	}
	if r.tone == "ok" {
		b.WriteString(cell)
	} else {
		b.WriteString(c.tint[r.tone] + cell + c.off)
	}
	if cw.date > 0 && r.abs != "" {
		b.WriteString("  " + c.dim + r.abs + c.off)
	}

	return b.String()
}

// renderFootnotes prints the lines under the table, after an empty line,
// when there is any.
func renderFootnotes(out *errWriter, fn footnotes, current string, c colors) {
	if len(fn.renew)+len(fn.stale)+len(fn.failed)+len(fn.token)+len(fn.cached) == 0 && current != "" {
		return
	}
	out.println()

	switch {
	case len(fn.renew) == 1:
		out.printf("  %sSign in again with:%s cc-login renew %s\n", c.dim, c.off, fn.renew[0])
	case len(fn.renew) > 1:
		out.printf("  %sSign in again with:%s cc-login renew NAME\n", c.dim, c.off)
	}
	if len(fn.stale) > 0 {
		out.printf("  %sQuota unknown for %s: the access token expired; a claude started with the login refreshes it:%s cc-login run NAME\n", c.dim, strings.Join(fn.stale, ", "), c.off)
	}
	if len(fn.failed) > 0 {
		out.printf("  %sQuota request failed for %s%s\n", c.dim, strings.Join(fn.failed, ", "), c.off)
	}
	if len(fn.cached) > 0 {
		out.printf("  %s* the quota as a claude last saw it (quota cache): %s%s\n", c.dim, strings.Join(fn.cached, ", "), c.off)
	}
	if len(fn.token) > 0 {
		out.printf("  %sQuota unknown for %s: a setup-token token cannot read it; see it in claude with%s /usage\n", c.dim, strings.Join(fn.token, ", "), c.off)
	}
	if current == "" {
		out.printf("  %sNo default login, so new claude launches start signed out. Choose one with:%s cc-login use NAME\n", c.dim, c.off)
	}
}

// errWriter writes until the first error and keeps it.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, a ...any) {
	if e.err == nil {
		_, e.err = fmt.Fprintf(e.w, format, a...)
	}
}

func (e *errWriter) println(a ...any) {
	if e.err == nil {
		_, e.err = fmt.Fprintln(e.w, a...)
	}
}
