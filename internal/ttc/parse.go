package ttc

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/xo/oracle/internal/tns"
)

// sqlInfo is what parseSQL finds in the text of a statement.
type sqlInfo struct {
	// kind is the kind of the statement, from its first keyword.
	kind StmtType
	// names holds the name of each bind, in order, once for each place it
	// appears. A name without quotes is in upper case.
	names []string
	// returning is a DML statement with RETURNING ... INTO.
	returning bool
	// call is a CALL, which is of the kind PLSQL and binds a name at each
	// place it appears, as SQL does.
	call bool
}

// parseSQL finds the kind of a statement, the names of its binds and whether
// it returns values into binds. It follows the parser of python-oracledb. A
// bind is a colon outside a comment, a string or a quoted name, followed by a
// name. A colon after a string is a part of a JSON constant, as in
// json_object('key':value), and is not a bind. A DDL statement has no binds,
// so the :new and :old of a trigger are not binds. CALL is PL/SQL, as
// python-oracledb reads it, but the server takes a bind for each place a name
// appears in it, as in SQL.
//
// go-ora removed the comments and the strings and took each colon and word
// that was left. It took a word in a q-string, as in q'[a ' :b]', and the
// colon of a JSON constant, and it took no quoted name and no name with a
// letter that is not ASCII.
//
// A string, a quoted name or a q-string with no end is an error. parseSQL
// returns what it found before it with the error.
func parseSQL(text string) (sqlInfo, error) {
	info := sqlInfo{kind: OTHERS}
	r := []rune(text)
	var first, returning, afterString bool
	var last rune
	for i := 0; i < len(r); i++ {
		ch := r[i]
		// a q that opens a q-string is not a word
		qString := (ch == 'q' || ch == 'Q') && i+1 < len(r) && r[i+1] == '\''
		switch {
		case unicode.IsLetter(ch) && !qString:
			j := i + 1
			for j < len(r) && isNameRune(r[j]) {
				j++
			}
			word := strings.ToUpper(string(r[i:j]))
			switch {
			case !first:
				first = true
				switch word {
				case "DECLARE", "BEGIN":
					info.kind = PLSQL
				case "CALL":
					info.kind, info.call = PLSQL, true
				case "SELECT", "WITH":
					info.kind = SELECT
				case "INSERT", "UPDATE", "DELETE", "MERGE":
					info.kind = DML
				case "CREATE", "ALTER", "DROP", "GRANT", "REVOKE", "ANALYZE", "AUDIT", "COMMENT", "TRUNCATE":
					return info, nil
				}
			case info.kind == DML && !returning && (word == "RETURNING" || word == "RETURN"):
				returning = true
			case returning && word == "INTO":
				info.returning = true
			}
			i, last, afterString = j-1, r[j-1], false
			continue
		case ch == '\'':
			var end int
			if last == 'q' || last == 'Q' {
				end = qStringEnd(r, i)
			} else {
				end = indexRune(r, i+1, '\'')
			}
			if end < 0 {
				return info, fmt.Errorf("the string at character %d has no closing quote: %w", i+1, tns.ErrInvalidValue)
			}
			i, last, afterString = end, '\'', true
			continue
		case ch == '"':
			end := indexRune(r, i+1, '"')
			if end < 0 {
				return info, fmt.Errorf("the quoted name at character %d has no closing quote: %w", i+1, tns.ErrInvalidValue)
			}
			i = end
		case ch == '-' && i+1 < len(r) && r[i+1] == '-':
			for i+1 < len(r) && r[i+1] != '\n' && r[i+1] != '\r' {
				i++
			}
		case ch == '/' && i+1 < len(r) && r[i+1] == '*':
			end := len(r) - 1
			for j := i + 2; j+1 < len(r); j++ {
				if r[j] == '*' && r[j+1] == '/' {
					end = j + 1
					break
				}
			}
			i = end
		case ch == ':' && !afterString:
			name, end, err := bindName(r, i)
			if err != nil {
				return info, err
			}
			if end > i {
				info.names = append(info.names, name)
				i = end
			}
		case unicode.IsSpace(ch):
			last = ch
			continue
		}
		last, afterString = r[i], false
	}
	return info, nil
}

// bindName reads the name of the bind whose colon is at r[i]. It returns the
// name and the index of its last rune, or i when no name follows the colon,
// as in :=. Spaces can come between the colon and the name. A name in quotes
// holds any runes and keeps its case. A name of digits is a position. Any
// other name starts with a letter and holds letters, digits, _, $ and #.
func bindName(r []rune, i int) (string, int, error) {
	j := i + 1
	for j < len(r) && unicode.IsSpace(r[j]) {
		j++
	}
	switch {
	case j >= len(r):
		return "", i, nil
	case r[j] == '"':
		end := indexRune(r, j+1, '"')
		if end < 0 {
			return "", i, fmt.Errorf("the quoted name of the bind at character %d has no closing quote: %w", i+1, tns.ErrInvalidValue)
		}
		return string(r[j+1 : end]), end, nil
	case unicode.IsDigit(r[j]):
		end := j
		for end+1 < len(r) && unicode.IsDigit(r[end+1]) {
			end++
		}
		return string(r[j : end+1]), end, nil
	case unicode.IsLetter(r[j]):
		end := j
		for end+1 < len(r) && isNameRune(r[end+1]) {
			end++
		}
		return strings.ToUpper(string(r[j : end+1])), end, nil
	}
	return "", i, nil
}

// qStringEnd returns the index of the quote that ends the q-string whose
// first quote is at r[i], or -1. The rune after that quote opens the
// q-string, and [, {, < and ( close with ], }, > and ).
func qStringEnd(r []rune, i int) int {
	if i+1 >= len(r) {
		return -1
	}
	sep := r[i+1]
	switch sep {
	case '[':
		sep = ']'
	case '{':
		sep = '}'
	case '<':
		sep = '>'
	case '(':
		sep = ')'
	}
	for j := i + 2; j+1 < len(r); j++ {
		if r[j] == sep && r[j+1] == '\'' {
			return j + 1
		}
	}
	return -1
}

// indexRune returns the index of the first c in r at or after i, or -1.
func indexRune(r []rune, i int, c rune) int {
	for ; i < len(r); i++ {
		if r[i] == c {
			return i
		}
	}
	return -1
}

// isNameRune reports whether c can follow the first letter of a name that
// is not in quotes.
func isNameRune(c rune) bool {
	return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '$' || c == '#'
}
