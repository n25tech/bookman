package formats

import (
	"encoding/hex"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NaturalLess compares strings so that "page2" < "page10", case-insensitively.
func NaturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		ca, cb := chunk(a), chunk(b)
		a, b = a[len(ca):], b[len(cb):]
		da, db := isDigit(ca[0]), isDigit(cb[0])
		switch {
		case da && db:
			ta, tb := strings.TrimLeft(ca, "0"), strings.TrimLeft(cb, "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
			if len(ca) != len(cb) { // "01" before "1"
				return len(ca) > len(cb)
			}
		case ca != cb:
			return ca < cb
		}
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func chunk(s string) string {
	d := isDigit(s[0])
	i := 1
	for i < len(s) && isDigit(s[i]) == d {
		i++
	}
	return s[:i]
}

// NaturalSort sorts names in natural order.
func NaturalSort(names []string) {
	sort.SliceStable(names, func(i, j int) bool { return NaturalLess(names[i], names[j]) })
}

// TitleFromFilename turns "My_Book-2nd.pdf" into "My Book-2nd".
func TitleFromFilename(name string) string {
	base := name
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	base = strings.ReplaceAll(base, "_", " ")
	return strings.TrimFunc(base, unicode.IsSpace)
}

// Archive entry names are raw bytes: zip tools that predate the UTF-8 flag
// write CP437 or a local code page, and PostgreSQL text columns only take
// UTF-8. Such names are stored hex-encoded and decoded before reading.
const hexPrefix = "hex:"

// EncodeEntryName makes an entry name safe to store as text.
func EncodeEntryName(name string) string {
	if utf8.ValidString(name) && !strings.HasPrefix(name, hexPrefix) {
		return name
	}
	return hexPrefix + hex.EncodeToString([]byte(name))
}

// DecodeEntryName reverses EncodeEntryName.
func DecodeEntryName(stored string) string {
	if rest, ok := strings.CutPrefix(stored, hexPrefix); ok {
		if b, err := hex.DecodeString(rest); err == nil {
			return string(b)
		}
	}
	return stored
}

// CleanText replaces invalid UTF-8 (from file names or PDF metadata) so it can be stored.
func CleanText(s string) string { return strings.ToValidUTF8(s, "�") }
