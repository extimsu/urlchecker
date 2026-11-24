package target

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// splitLine splits a target line into the target, its key=value options and
// its trailing {label="value", ...} block:
//
//	icmp://10.0.0.1 size=1400 df=true {node="worker-1"}
func splitLine(line string) (tgt string, opts [][2]string, labels map[string]string, err error) {
	line = strings.TrimSpace(line)
	if strings.HasSuffix(line, "}") {
		i := strings.LastIndex(line, "{")
		if i < 0 {
			return "", nil, nil, fmt.Errorf("unbalanced } in %q", line)
		}
		if labels, err = parseLabels(line[i+1 : len(line)-1]); err != nil {
			return "", nil, nil, err
		}
		line = strings.TrimSpace(line[:i])
	}
	fields, err := splitFields(line)
	if err != nil {
		return "", nil, nil, err
	}
	if len(fields) == 0 {
		return "", nil, nil, fmt.Errorf("empty target")
	}
	for _, f := range fields[1:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return "", nil, nil, fmt.Errorf("option %q: want key=value", f)
		}
		opts = append(opts, [2]string{strings.ToLower(k), v})
	}
	return fields[0], opts, labels, nil
}

// splitFields splits on spaces; a double-quoted value may contain spaces and
// \" or \\ escapes: body="status: ok".
func splitFields(s string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inQuote, escaped, have := false, false, false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case inQuote && r == '\\':
			escaped = true
		case r == '"':
			inQuote, have = !inQuote, true
		case !inQuote && unicode.IsSpace(r):
			if have {
				fields = append(fields, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quote in %q", s)
	}
	if have {
		fields = append(fields, cur.String())
	}
	return fields, nil
}

var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// reservedLabels are set by urlchecker on urlchecker_target_info.
var reservedLabels = map[string]bool{"protocol": true, "target": true}

// parseLabels reads `name="value", other="value"` (Prometheus style).
func parseLabels(s string) (map[string]string, error) {
	labels := map[string]string{}
	s = strings.TrimSpace(s)
	for s != "" {
		name, rest, ok := strings.Cut(s, "=")
		name = strings.TrimSpace(name)
		if !ok || !labelName.MatchString(name) || strings.HasPrefix(name, "__") {
			return nil, fmt.Errorf("invalid label name %q", name)
		}
		if reservedLabels[name] {
			return nil, fmt.Errorf("label %q is reserved", name)
		}
		if _, dup := labels[name]; dup {
			return nil, fmt.Errorf("label %q set twice", name)
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, `"`) {
			return nil, fmt.Errorf("label %s: value must be double-quoted", name)
		}
		var val strings.Builder
		i, closed := 1, false
		for ; i < len(rest); i++ {
			c := rest[i]
			if c == '\\' && i+1 < len(rest) {
				i++
				val.WriteByte(rest[i])
				continue
			}
			if c == '"' {
				closed = true
				break
			}
			val.WriteByte(c)
		}
		if !closed {
			return nil, fmt.Errorf("label %s: unterminated value", name)
		}
		labels[name] = val.String()
		s = strings.TrimSpace(rest[i+1:])
		if s != "" {
			if !strings.HasPrefix(s, ",") {
				return nil, fmt.Errorf("labels: want a comma after %s", name)
			}
			s = strings.TrimSpace(s[1:])
		}
	}
	return labels, nil
}
