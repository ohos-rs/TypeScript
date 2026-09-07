package packagejson

import (
	"bytes"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	json5 "github.com/titanous/json5"

	json "github.com/microsoft/TypeScript/tsc/internal/json"
)

// ParseJSON5 implements the OH package-manager branch from
// moduleNameResolver.ts, where oh-package.json5 is parsed with the JSON5
// package instead of TypeScript's strict package.json reader. Normalization is
// syntax-only and retains object insertion order, which is significant for
// conditional exports.
func ParseJSON5(data []byte) (Fields, error) {
	normalized, err := (&json5Normalizer{data: data}).normalize()
	if err != nil {
		return Fields{}, err
	}
	return Parse(normalized)
}

type json5Normalizer struct {
	data []byte
	pos  int
}

func (n *json5Normalizer) normalize() ([]byte, error) {
	var out bytes.Buffer
	if err := n.writeValue(&out); err != nil {
		return nil, err
	}
	n.skipTrivia()
	if n.pos != len(n.data) {
		return nil, n.syntaxError("unexpected content after JSON5 value")
	}
	return out.Bytes(), nil
}

func (n *json5Normalizer) writeValue(out *bytes.Buffer) error {
	n.skipTrivia()
	if n.pos >= len(n.data) {
		return n.syntaxError("expected JSON5 value")
	}
	switch n.data[n.pos] {
	case '{':
		return n.writeObject(out)
	case '[':
		return n.writeArray(out)
	case '\'', '"':
		return n.writePrimitive(out, n.scanString())
	default:
		start := n.pos
		for n.pos < len(n.data) {
			switch n.data[n.pos] {
			case ',', ']', '}':
				return n.writePrimitive(out, n.data[start:n.pos])
			case '/':
				if n.pos+1 < len(n.data) && (n.data[n.pos+1] == '/' || n.data[n.pos+1] == '*') {
					return n.writePrimitive(out, n.data[start:n.pos])
				}
			}
			n.pos++
		}
		return n.writePrimitive(out, n.data[start:n.pos])
	}
}

func (n *json5Normalizer) writeObject(out *bytes.Buffer) error {
	n.pos++
	out.WriteByte('{')
	n.skipTrivia()
	first := true
	for n.pos < len(n.data) && n.data[n.pos] != '}' {
		if !first {
			out.WriteByte(',')
		}
		key, err := n.readObjectKey()
		if err != nil {
			return err
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return err
		}
		out.Write(encodedKey)
		n.skipTrivia()
		if n.pos >= len(n.data) || n.data[n.pos] != ':' {
			return n.syntaxError("expected ':' after object key")
		}
		n.pos++
		out.WriteByte(':')
		if err := n.writeValue(out); err != nil {
			return err
		}
		n.skipTrivia()
		if n.pos >= len(n.data) {
			return n.syntaxError("unterminated object")
		}
		if n.data[n.pos] == ',' {
			n.pos++
			n.skipTrivia()
			if n.pos < len(n.data) && n.data[n.pos] == '}' {
				break
			}
		} else if n.data[n.pos] != '}' {
			return n.syntaxError("expected ',' or '}' in object")
		}
		first = false
	}
	if n.pos >= len(n.data) || n.data[n.pos] != '}' {
		return n.syntaxError("unterminated object")
	}
	n.pos++
	out.WriteByte('}')
	return nil
}

func (n *json5Normalizer) writeArray(out *bytes.Buffer) error {
	n.pos++
	out.WriteByte('[')
	n.skipTrivia()
	first := true
	for n.pos < len(n.data) && n.data[n.pos] != ']' {
		if !first {
			out.WriteByte(',')
		}
		if err := n.writeValue(out); err != nil {
			return err
		}
		n.skipTrivia()
		if n.pos >= len(n.data) {
			return n.syntaxError("unterminated array")
		}
		if n.data[n.pos] == ',' {
			n.pos++
			n.skipTrivia()
			if n.pos < len(n.data) && n.data[n.pos] == ']' {
				break
			}
		} else if n.data[n.pos] != ']' {
			return n.syntaxError("expected ',' or ']' in array")
		}
		first = false
	}
	if n.pos >= len(n.data) || n.data[n.pos] != ']' {
		return n.syntaxError("unterminated array")
	}
	n.pos++
	out.WriteByte(']')
	return nil
}

func (n *json5Normalizer) readObjectKey() (string, error) {
	n.skipTrivia()
	if n.pos >= len(n.data) {
		return "", n.syntaxError("expected object key")
	}
	start := n.pos
	if n.data[n.pos] == '\'' || n.data[n.pos] == '"' {
		raw := n.scanString()
		var key string
		if err := json5.Unmarshal(raw, &key); err != nil {
			return "", err
		}
		return key, nil
	}
	for n.pos < len(n.data) {
		if n.data[n.pos] == ':' || isJSON5SpaceAt(n.data, n.pos) ||
			(n.data[n.pos] == '/' && n.pos+1 < len(n.data) && (n.data[n.pos+1] == '/' || n.data[n.pos+1] == '*')) {
			break
		}
		n.pos++
	}
	if start == n.pos {
		return "", n.syntaxError("expected object key")
	}
	raw := n.data[start:n.pos]
	probe := make([]byte, 0, len(raw)+7)
	probe = append(probe, '{')
	probe = append(probe, raw...)
	probe = append(probe, ':', 'n', 'u', 'l', 'l', '}')
	var object map[string]any
	if err := json5.Unmarshal(probe, &object); err != nil {
		return "", err
	}
	for key := range object {
		return key, nil
	}
	return "", errors.New("JSON5 object key was not decoded")
}

func (n *json5Normalizer) scanString() []byte {
	start := n.pos
	quote := n.data[n.pos]
	n.pos++
	for n.pos < len(n.data) {
		current := n.data[n.pos]
		n.pos++
		if current == '\\' && n.pos < len(n.data) {
			if n.data[n.pos] == '\r' {
				n.pos++
				if n.pos < len(n.data) && n.data[n.pos] == '\n' {
					n.pos++
				}
			} else {
				n.pos++
			}
			continue
		}
		if current == quote {
			break
		}
	}
	return n.data[start:n.pos]
}

func (n *json5Normalizer) writePrimitive(out *bytes.Buffer, raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return n.syntaxError("expected JSON5 primitive")
	}
	var value any
	if err := json5.Unmarshal(raw, &value); err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	out.Write(encoded)
	return nil
}

func (n *json5Normalizer) skipTrivia() {
	for n.pos < len(n.data) {
		if isJSON5SpaceAt(n.data, n.pos) {
			_, size := utf8.DecodeRune(n.data[n.pos:])
			n.pos += size
			continue
		}
		if n.pos+1 >= len(n.data) || n.data[n.pos] != '/' {
			return
		}
		switch n.data[n.pos+1] {
		case '/':
			n.pos += 2
			for n.pos < len(n.data) && n.data[n.pos] != '\n' && n.data[n.pos] != '\r' {
				n.pos++
			}
		case '*':
			n.pos += 2
			for n.pos+1 < len(n.data) && !(n.data[n.pos] == '*' && n.data[n.pos+1] == '/') {
				n.pos++
			}
			if n.pos+1 < len(n.data) {
				n.pos += 2
			}
		default:
			return
		}
	}
}

func isJSON5SpaceAt(data []byte, pos int) bool {
	r, _ := utf8.DecodeRune(data[pos:])
	return r == '\ufeff' || unicode.IsSpace(r)
}

func (n *json5Normalizer) syntaxError(message string) error {
	return fmt.Errorf("%s at byte %d", message, n.pos)
}
