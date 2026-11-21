package listing

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

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

func format(logins []Login, now time.Time) ([]row, footnotes) {
	var rows []row
	var fn footnotes
	n := now.Unix()
	year := now.Local().Year()
	for _, l := range logins {
		r := row{name: l.Name, email: l.Email, plan: l.Plan, rel: "-", q5: "-", q7: "-"}
		r.state, r.tone = Status(l, now)
		if l.Expiry != 0 {
			t := time.Unix(l.Expiry, 0).Local()
			if t.Year() == year {
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
		if r.tone != "out" && r.tone != "bad" {
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
				if w := l.Quota.FiveHour; w.Used != nil {
					r.q5, r.q5t = pct(*w.Used)
					r.q5 += star
					if w.Reset != 0 {
						r.q5 += " " + strftime("15:04", w.Reset)
					}
				}
				if w := l.Quota.SevenDay; w.Used != nil {
					r.q7, r.q7t = pct(*w.Used)
					r.q7 += star
					if w.Reset != 0 {
						r.q7 += " " + strftime("01-02", w.Reset)
					}
				}
			case l.QuotaWhy == "expired":
				r.q5, r.q7 = "?", "?"
				fn.stale = append(fn.stale, l.Name)
			case l.QuotaWhy == "error":
				r.q5, r.q7 = "?", "?"
				fn.failed = append(fn.failed, l.Name)
			case l.Token:
				fn.token = append(fn.token, l.Name)
			}
		}
		rows = append(rows, r)
	}
	return rows, fn
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

// Render prints the table of logins. current is the default login, "" for none.
// It returns the first error that writing to w gave.
func Render(w io.Writer, logins []Login, current string, now time.Time, st Style) error {
	out := &errWriter{w: w}
	if len(logins) == 0 {
		out.println("cc-login: no logins yet; add one with: cc-login add NAME")
		return out.err
	}
	var off, dim, bold, acc string
	tint := map[string]string{}
	cols := 0
	if st.Color {
		off, dim, bold = "\x1b[0m", "\x1b[90m", "\x1b[1m"
		// The accent, in 256-color numbers: orange 130 on a light terminal,
		// 215 on a dark one, and 166, which reads on both, when unknown.
		switch st.Theme {
		case "light":
			acc = "\x1b[38;5;130m"
		case "dark":
			acc = "\x1b[38;5;215m"
		default:
			acc = "\x1b[38;5;166m"
		}
		cols = st.Cols
		tint = map[string]string{"ok": "\x1b[32m", "warn": "\x1b[33m", "bad": "\x1b[31m", "out": dim}
	}
	rows, fn := format(logins, now)

	wn, we, wp, ws, wr, wd, w5, w7 := 4, 7, 4, 10, 7, 0, 7, 9
	for _, r := range rows {
		wn, we, wp = max(wn, width(r.name)), max(we, width(r.email)), max(wp, width(r.plan))
		wr, wd = max(wr, width(r.rel)), max(wd, width(r.abs))
		w5, w7 = max(w5, width(r.q5)), max(w7, width(r.q7))
	}
	// The marker column, then each column and the two spaces before the next;
	// the status column holds the dot and a space before the status.
	total := 2 + wn + 2 + we + 2 + wp + 2 + w5 + 2 + w7 + 2 + 2 + ws + 2 + wr
	if wd > 0 {
		total += 2 + wd
	}
	// Too wide: leave out the date and time first, then shorten the accounts.
	if cols > 0 && total > cols && wd > 0 {
		total -= 2 + wd
		wd = 0
	}
	if cols > 0 && total > cols {
		we = max(we-(total-cols), 8)
	}

	out.printf("%s  %s  %s  %s  %s  %s    %s  EXPIRES%s\n", dim,
		pad("NAME", wn), pad("ACCOUNT", we), pad("PLAN", wp), pad("5H LEFT", w5), pad("WEEK LEFT", w7), pad("STATUS", ws), off)
	for _, r := range rows {
		var b strings.Builder
		email := r.email
		if width(email) > we {
			email = string([]rune(email)[:we-1]) + "…"
		}
		if r.name == current {
			b.WriteString(acc + "▸" + off + " " + bold + acc + pad(r.name, wn) + off)
		} else {
			b.WriteString("  " + pad(r.name, wn))
		}
		b.WriteString("  " + pad(email, we) + "  " + pad(r.plan, wp) + "  ")
		for _, q := range [][3]string{{r.q5, r.q5t, ""}, {r.q7, r.q7t, "7"}} {
			n := w5
			if q[2] == "7" {
				n = w7
			}
			if q[1] != "" {
				b.WriteString(tint[q[1]] + pad(q[0], n) + off + "  ")
			} else {
				b.WriteString(pad(q[0], n) + "  ")
			}
		}
		if r.tone == "out" {
			b.WriteString(dim + "○" + off + " ")
		} else {
			b.WriteString(tint[r.tone] + "●" + off + " ")
		}
		b.WriteString(pad(r.state, ws) + "  ")
		cell := r.rel
		if wd > 0 && r.abs != "" {
			cell = pad(cell, wr)
		}
		if r.tone == "ok" {
			b.WriteString(cell)
		} else {
			b.WriteString(tint[r.tone] + cell + off)
		}
		if wd > 0 && r.abs != "" {
			b.WriteString("  " + dim + r.abs + off)
		}
		out.println(b.String())
	}

	if len(fn.renew)+len(fn.stale)+len(fn.failed)+len(fn.token)+len(fn.cached) > 0 || current == "" {
		out.println()
		switch {
		case len(fn.renew) == 1:
			out.printf("  %sSign in again with:%s cc-login renew %s\n", dim, off, fn.renew[0])
		case len(fn.renew) > 1:
			out.printf("  %sSign in again with:%s cc-login renew NAME\n", dim, off)
		}
		if len(fn.stale) > 0 {
			out.printf("  %sQuota unknown for %s: the access token expired; a claude started with the login refreshes it:%s cc-login run NAME\n", dim, strings.Join(fn.stale, ", "), off)
		}
		if len(fn.failed) > 0 {
			out.printf("  %sQuota request failed for %s%s\n", dim, strings.Join(fn.failed, ", "), off)
		}
		if len(fn.cached) > 0 {
			out.printf("  %s* the quota as a claude last saw it (quota cache): %s%s\n", dim, strings.Join(fn.cached, ", "), off)
		}
		if len(fn.token) > 0 {
			out.printf("  %sQuota unknown for %s: a setup-token token cannot read it; see it in claude with%s /usage\n", dim, strings.Join(fn.token, ", "), off)
		}
		if current == "" {
			out.printf("  %sNo default login, so new claude launches start signed out. Choose one with:%s cc-login use NAME\n", dim, off)
		}
	}
	return out.err
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
