// SPDX-License-Identifier: AGPL-3.0-only

package db

import "strings"

// splitSQL divides a migration file into statements on semicolons that are
// outside quotes, dollar-quotes and comments.
func splitSQL(sql string) []string {
	var stmts []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if s != "" {
			stmts = append(stmts, s)
		}
	}
	for i := 0; i < len(sql); {
		switch sql[i] {
		case '-':
			if i+1 < len(sql) && sql[i+1] == '-' {
				i += 2
				for i < len(sql) && sql[i] != '\n' {
					i++
				}
				continue
			}
		case '/':
			if i+1 < len(sql) && sql[i+1] == '*' {
				i += 2
				for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
					i++
				}
				if i+1 < len(sql) {
					i += 2
				} else {
					i = len(sql)
				}
				continue
			}
		case '\'':
			i = copyQuoted(sql, i, &b, '\'')
			continue
		case '"':
			i = copyQuoted(sql, i, &b, '"')
			continue
		case '$':
			if tag, ok := dollarDelim(sql[i:]); ok {
				rest := sql[i+len(tag):]
				end := strings.Index(rest, tag)
				if end < 0 {
					b.WriteString(sql[i:])
					i = len(sql)
					continue
				}
				n := len(tag) + end + len(tag)
				b.WriteString(sql[i : i+n])
				i += n
				continue
			}
		case ';':
			flush()
			i++
			continue
		}
		b.WriteByte(sql[i])
		i++
	}
	flush()
	return stmts
}

func copyQuoted(sql string, i int, b *strings.Builder, quote byte) int {
	b.WriteByte(sql[i])
	i++
	for i < len(sql) {
		b.WriteByte(sql[i])
		if sql[i] == quote {
			if i+1 < len(sql) && sql[i+1] == quote {
				b.WriteByte(quote)
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

func dollarDelim(s string) (string, bool) {
	if len(s) < 2 || s[0] != '$' {
		return "", false
	}
	j := 1
	for j < len(s) && isDollarTagChar(s[j]) {
		j++
	}
	if j >= len(s) || s[j] != '$' {
		return "", false
	}
	return s[:j+1], true
}

func isDollarTagChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
