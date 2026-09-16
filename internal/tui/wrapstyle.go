package tui

import "strings"

// A wrapped row inherits nothing from the row above it: the terminal is handed
// each row separately, so a colour opened on one row is simply not in effect on
// the next. ansi.Wrap leaves the escape sequences where they fell, which means a
// style that spans a wrap boundary only paints its first row.
//
// It shows up worst on comments, and looks like a comment-specific bug, but the
// cause has nothing to do with comments: a comment is one long token, so its
// colour opens once at the start of the line and every continuation row falls
// outside it. Ordinary code is many short tokens, so most rows happen to begin
// with a fresh sequence and the damage is invisible.
//
// carryStyles re-opens, at the start of each continuation row, whatever was
// still active at the end of the row before. Rows are left open-ended, exactly
// as the first row already was — the closing reset at the end of the last row,
// and the per-row padding and truncation downstream, are unchanged.
func carryStyles(rows []string) []string {
	if len(rows) < 2 {
		return rows
	}
	out := make([]string, len(rows))
	active := ""
	for i, row := range rows {
		out[i] = active + row
		active = activeAfter(active, row)
	}
	return out
}

// activeAfter returns the SGR state left open once row has been written, given
// the state it started in.
//
// The model is accumulate-until-reset: sequences pile up and a reset (ESC[0m or
// its ESC[m shorthand) clears them. That is exactly what chroma and lipgloss
// emit. Attribute-scoped resets — 39 for foreground, 49 for background — are
// carried rather than interpreted, which can only re-open something the row's
// own sequences immediately override, never drop a colour.
func activeAfter(active, row string) string {
	for i := 0; i < len(row); i++ {
		if row[i] != 0x1b || i+1 >= len(row) || row[i+1] != '[' {
			continue
		}
		end := -1
		for j := i + 2; j < len(row); j++ {
			if c := row[j]; c >= 0x40 && c <= 0x7e {
				end = j
				break
			}
		}
		if end < 0 {
			break // truncated sequence; nothing more to read
		}
		seq := row[i : end+1]
		i = end
		if row[end] != 'm' {
			continue // not a style sequence (cursor moves and the like)
		}
		if isSGRReset(seq) {
			active = ""
			continue
		}
		active += seq
	}
	return active
}

// isSGRReset reports whether an SGR sequence clears all attributes: ESC[m,
// ESC[0m, or any parameter list whose every field is an explicit zero.
func isSGRReset(seq string) bool {
	params := seq[2 : len(seq)-1]
	if params == "" {
		return true
	}
	for _, p := range strings.Split(params, ";") {
		if strings.Trim(p, "0") != "" {
			return false
		}
	}
	return true
}
