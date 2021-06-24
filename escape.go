package zed

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

func QuotedName(name string) string {
	if !IsIdentifier(name) {
		name = QuotedString([]byte(name), false)
	}
	return name
}

const hexdigits = "0123456789abcdef"

func QuotedString(data []byte, bstr bool) string {
	var out []byte
	var start int
	out = append(out, '"')
	for i := 0; i < len(data); {
		r, l := utf8.DecodeRune(data[i:])
		if c := esc(r); c != 0 {
			out = append(out, data[start:i]...)
			out = append(out, '\\', c)
			i++
			start = i
			continue
		}
		if (r == utf8.RuneError && bstr) || !unicode.IsPrint(r) {
			out = append(out, data[start:i]...)
			// XXX format l chars
			c := data[i]
			out = append(out, '\\', 'x', hexdigits[c>>4], hexdigits[c&0xf])
			i++
			start = i
		} else {
			i += l
		}
	}
	out = append(out, data[start:]...)
	out = append(out, '"')
	return string(out)
}

func esc(r rune) byte {
	switch r {
	case '\\':
		return '\\'
	case '"':
		return '"'
	case '\b':
		return 'b'
	case '\f':
		return 'f'
	case '\n':
		return 'n'
	case '\r':
		return 'r'
	case '\t':
		return 't'
	}
	return 0
}

// UnescapeBstring replaces all the escaped characters defined in the
// for the zng spec for the bstring type with their unescaped equivalents.
func UnescapeBstring(data []byte) []byte {
	if bytes.IndexByte(data, '\\') < 0 {
		return data
	}
	var buf []byte
	i := 0
	for i < len(data) {
		c := data[i]
		if c == '\\' && len(data[i:]) >= 2 {
			var n int
			c, n = parseBstringEscape(data[i:])
			i += n
		} else {
			i++
		}
		buf = append(buf, c)
	}
	return buf
}

func parseBstringEscape(data []byte) (byte, int) {
	if len(data) >= 4 && data[1] == 'x' {
		v1 := Unhex(data[2])
		v2 := Unhex(data[3])
		if v1 <= 0xf || v2 <= 0xf {
			return v1<<4 | v2, 4
		}
	} else if len(data) >= 2 && data[1] == '\\' {
		return data[1], 2
	}

	// Not a valid escape sequence, just leave it alone.
	return data[0], 1
}

func Unhex(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10
	}
	return 255
}
