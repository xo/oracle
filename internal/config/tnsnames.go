package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNoAlias is an alias that tnsnames.ora does not name.
var ErrNoAlias = errors.New("no such alias in tnsnames.ora")

// LookupAlias returns the connect descriptor that the tnsnames.ora of the
// folder dir names alias, such as (DESCRIPTION=(ADDRESS=...)). An alias is
// found with no regard to case. The file can include others with IFILE, and
// an entry that comes later, in the file or in a file that it includes,
// replaces an earlier one, as in python-oracledb. An entry can be an easy
// connect string, host:port/service, which becomes a descriptor.
func LookupAlias(dir, alias string) (string, error) {
	entries := map[string]string{}
	if err := readTNSNames(filepath.Join(dir, "tnsnames.ora"), entries, nil); err != nil {
		return "", err
	}
	desc, ok := entries[strings.ToUpper(alias)]
	if !ok {
		return "", fmt.Errorf("finding %q in %s: %w", alias, dir, ErrNoAlias)
	}
	if strings.HasPrefix(desc, "(") {
		return desc, nil
	}
	return easyConnect(desc)
}

// readTNSNames reads the entries of the tnsnames.ora at name into entries,
// keyed by the upper case of each alias. chain holds the files that include
// this one, so that a file that includes itself, at any depth, is an error.
func readTNSNames(name string, entries map[string]string, chain []string) error {
	abs, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	if slices.Contains(chain, abs) {
		return fmt.Errorf("reading %s: IFILE includes a file that includes it", name)
	}
	chain = append(chain, abs)
	b, err := os.ReadFile(name)
	if err != nil {
		return fmt.Errorf("reading tnsnames.ora: %w", err)
	}
	src := stripComments(string(b))
	for pos := 0; pos < len(src); {
		eq := strings.IndexByte(src[pos:], '=')
		if eq < 0 {
			if strings.TrimSpace(src[pos:]) != "" {
				return fmt.Errorf("reading %s: %q has no value", name, strings.TrimSpace(src[pos:]))
			}
			break
		}
		names := strings.TrimSpace(src[pos : pos+eq])
		pos += eq + 1
		for pos < len(src) && isSpace(src[pos]) {
			pos++
		}
		var value string
		names = aliasNames(names)
		if pos < len(src) && src[pos] == '(' {
			end, err := closing(src, pos)
			if err != nil {
				return fmt.Errorf("reading %s, at %s: %w", name, names, err)
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
			if err := readTNSNames(path, entries, chain); err != nil {
				return err
			}
			continue
		}
		for alias := range strings.SplitSeq(names, ",") {
			if alias = strings.ToUpper(strings.TrimSpace(alias)); alias != "" && !strings.ContainsAny(alias, " \t") {
				entries[alias] = compact(value)
			}
		}
	}
	return nil
}

// aliasNames returns the names of an entry from the text before its =. The
// names can run over lines, each ending in a comma, and a line before them
// that does not end in one is not part of the entry, as python-oracledb
// skips text that is no entry.
func aliasNames(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	start := len(lines) - 1
	for start > 0 && strings.HasSuffix(strings.TrimSpace(lines[start-1]), ",") {
		start--
	}
	return strings.Join(lines[start:], "\n")
}

// easyConnect returns the descriptor of an easy connect string,
// [tcp://|tcps://]host[:port][/service], with port 1521 when it names none.
func easyConnect(s string) (string, error) {
	protocol := "TCP"
	if p, rest, ok := strings.Cut(s, "://"); ok {
		protocol, s = strings.ToUpper(p), rest
	}
	s, _, _ = strings.Cut(s, "?")
	hostPort, service, _ := strings.Cut(s, "/")
	host, port := hostPort, "1521"
	if strings.HasPrefix(hostPort, "[") {
		// an IPv6 address
		end := strings.IndexByte(hostPort, ']')
		if end < 0 {
			return "", fmt.Errorf("reading the easy connect string %q: the address is not closed", s)
		}
		host = hostPort[1:end]
		if p, ok := strings.CutPrefix(hostPort[end+1:], ":"); ok {
			port = p
		}
	} else if h, p, ok := strings.Cut(hostPort, ":"); ok {
		host, port = h, p
	}
	if host == "" {
		return "", fmt.Errorf("reading the easy connect string %q: it names no host", s)
	}
	desc := fmt.Sprintf("(DESCRIPTION=(ADDRESS=(PROTOCOL=%s)(HOST=%s)(PORT=%s))", protocol, host, port)
	if service != "" {
		desc += fmt.Sprintf("(CONNECT_DATA=(SERVICE_NAME=%s))", service)
	}
	return desc + ")", nil
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
