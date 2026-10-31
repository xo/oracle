package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoAlias is an alias that tnsnames.ora does not name.
var ErrNoAlias = errors.New("no such alias in tnsnames.ora")

// LookupAlias returns the connect descriptor that the tnsnames.ora of the
// folder dir names alias, such as (DESCRIPTION=(ADDRESS=...)). An alias is
// found with no regard to case. The file can include others with IFILE.
func LookupAlias(dir, alias string) (string, error) {
	entries, err := readTNSNames(filepath.Join(dir, "tnsnames.ora"), 0)
	if err != nil {
		return "", err
	}
	if desc, ok := entries[strings.ToUpper(alias)]; ok {
		return desc, nil
	}
	return "", fmt.Errorf("finding %q in %s: %w", alias, dir, ErrNoAlias)
}

// readTNSNames reads the entries of the tnsnames.ora at name, keyed by the
// upper case of each alias. depth limits the IFILE chain.
func readTNSNames(name string, depth int) (map[string]string, error) {
	if depth > 8 {
		return nil, fmt.Errorf("reading %s: too many IFILE levels", name)
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("reading tnsnames.ora: %w", err)
	}
	entries := map[string]string{}
	src := stripComments(string(b))
	for pos := 0; pos < len(src); {
		eq := strings.IndexByte(src[pos:], '=')
		if eq < 0 {
			if strings.TrimSpace(src[pos:]) != "" {
				return nil, fmt.Errorf("reading %s: %q has no value", name, strings.TrimSpace(src[pos:]))
			}
			break
		}
		names := strings.TrimSpace(src[pos : pos+eq])
		pos += eq + 1
		for pos < len(src) && isSpace(src[pos]) {
			pos++
		}
		var value string
		if pos < len(src) && src[pos] == '(' {
			end, err := closing(src, pos)
			if err != nil {
				return nil, fmt.Errorf("reading %s, at %s: %w", name, names, err)
			}
			value, pos = src[pos:end+1], end+1
		} else {
			// a value with no parentheses, which IFILE has, runs to the end
			// of its line
			end := strings.IndexByte(src[pos:], '\n')
			if end < 0 {
				end = len(src) - pos
			}
			value, pos = strings.TrimSpace(src[pos:pos+end]), pos+end
		}
		if strings.EqualFold(names, "IFILE") {
			path := strings.Trim(value, `"'`)
			if !filepath.IsAbs(path) {
				path = filepath.Join(filepath.Dir(name), path)
			}
			included, err := readTNSNames(path, depth+1)
			if err != nil {
				return nil, err
			}
			for k, v := range included {
				if _, ok := entries[k]; !ok {
					entries[k] = v
				}
			}
			continue
		}
		for alias := range strings.SplitSeq(names, ",") {
			if alias = strings.ToUpper(strings.TrimSpace(alias)); alias != "" {
				entries[alias] = compact(value)
			}
		}
	}
	return entries, nil
}

// stripComments removes each comment, which runs from a # to the end of its
// line.
func stripComments(s string) string {
	var b strings.Builder
	for line := range strings.Lines(s) {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i] + "\n"
		}
		b.WriteString(line)
	}
	return b.String()
}

// closing returns the index of the parenthesis that closes the one at start.
func closing(s string, start int) (int, error) {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, errors.New("a parenthesis is not closed")
}

// compact removes the white space between the parentheses of a descriptor,
// which a file spreads over lines.
func compact(desc string) string {
	var b strings.Builder
	for line := range strings.Lines(desc) {
		b.WriteString(strings.TrimSpace(line))
	}
	return b.String()
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
