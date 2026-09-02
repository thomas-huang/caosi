package config

import (
	"bytes"
	"fmt"
	"unicode"
)

// StripJSONC removes // and /* */ comments and trailing commas so the result
// is strict JSON. Strings are left intact.
func StripJSONC(in []byte) ([]byte, error) {
	var out bytes.Buffer
	out.Grow(len(in))
	i := 0
	for i < len(in) {
		c := in[i]
		if c == '"' {
			str, next, err := readJSONString(in, i)
			if err != nil {
				return nil, err
			}
			out.Write(str)
			i = next
			continue
		}
		if c == '/' && i+1 < len(in) {
			switch in[i+1] {
			case '/':
				i += 2
				for i < len(in) && in[i] != '\n' {
					i++
				}
				continue
			case '*':
				i += 2
				for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
					i++
				}
				if i+1 >= len(in) {
					return nil, fmt.Errorf("unclosed block comment")
				}
				i += 2
				continue
			}
		}
		out.WriteByte(c)
		i++
	}
	return stripTrailingCommas(out.Bytes()), nil
}

func readJSONString(in []byte, i int) ([]byte, int, error) {
	start := i
	i++ // opening quote
	for i < len(in) {
		c := in[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == '"' {
			return in[start : i+1], i + 1, nil
		}
		i++
	}
	return nil, 0, fmt.Errorf("unclosed string")
}

func stripTrailingCommas(in []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(in))
	i := 0
	for i < len(in) {
		if in[i] == '"' {
			str, next, err := readJSONString(in, i)
			if err != nil {
				out.Write(in[i:])
				break
			}
			out.Write(str)
			i = next
			continue
		}
		if in[i] == ',' {
			j := i + 1
			for j < len(in) && unicode.IsSpace(rune(in[j])) {
				j++
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				i++
				continue
			}
		}
		out.WriteByte(in[i])
		i++
	}
	return out.Bytes()
}
