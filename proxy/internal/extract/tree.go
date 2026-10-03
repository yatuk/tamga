package extract

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// node is one JSON value with its position in the body it was parsed from.
// Strings are not decoded until asked for: a request can carry megabytes of
// base64 that nothing will ever read.
type node struct {
	kind       byte // 'o' object, 'a' array, 's' string, 'x' any other scalar
	start, end int  // body[start:end] is the value, quotes included for a string
	keys       []string
	vals       []*node // object values, parallel to keys; array elements
}

var errMalformed = errors.New("extract: malformed JSON")

// parseTree reads src into a tree. It expects valid JSON and checks it,
// because the offsets it records are only meaningful when it is.
func parseTree(src []byte) (*node, error) {
	// Invalid UTF-8 would make a string's bytes and its decoded text differ
	// in ways the offsets cannot express. Such a body is scanned raw instead.
	if !utf8.Valid(src) || !json.Valid(src) {
		return nil, errMalformed
	}
	p := treeParser{b: src}
	n, ok := p.value(0)
	if !ok {
		return nil, errMalformed
	}
	return n, nil
}

type treeParser struct {
	b []byte
	i int
}

func (p *treeParser) skipSpace() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

func (p *treeParser) value(depth int) (*node, bool) {
	if depth > maxDepth {
		return nil, false
	}
	p.skipSpace()
	if p.i >= len(p.b) {
		return nil, false
	}
	switch p.b[p.i] {
	case '{':
		return p.object(depth)
	case '[':
		return p.array(depth)
	case '"':
		return p.str()
	default:
		start := p.i
		for p.i < len(p.b) {
			switch p.b[p.i] {
			case ',', '}', ']', ' ', '\t', '\r', '\n':
				return &node{kind: 'x', start: start, end: p.i}, p.i > start
			}
			p.i++
		}
		return &node{kind: 'x', start: start, end: p.i}, p.i > start
	}
}

func (p *treeParser) str() (*node, bool) {
	start := p.i
	for i := start + 1; i < len(p.b); i++ {
		switch p.b[i] {
		case '\\':
			i++
		case '"':
			p.i = i + 1
			return &node{kind: 's', start: start, end: p.i}, true
		}
	}
	return nil, false
}

func (p *treeParser) object(depth int) (*node, bool) {
	n := &node{kind: 'o', start: p.i}
	p.i++ // {
	for {
		p.skipSpace()
		if p.i >= len(p.b) {
			return nil, false
		}
		if p.b[p.i] == '}' {
			p.i++
			n.end = p.i
			return n, true
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		if p.b[p.i] != '"' {
			return nil, false
		}
		key, ok := p.str()
		if !ok {
			return nil, false
		}
		p.skipSpace()
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return nil, false
		}
		p.i++
		val, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		n.keys = append(n.keys, decodeKey(p.b[key.start:key.end]))
		n.vals = append(n.vals, val)
	}
}

func (p *treeParser) array(depth int) (*node, bool) {
	n := &node{kind: 'a', start: p.i}
	p.i++ // [
	for {
		p.skipSpace()
		if p.i >= len(p.b) {
			return nil, false
		}
		if p.b[p.i] == ']' {
			p.i++
			n.end = p.i
			return n, true
		}
		if p.b[p.i] == ',' {
			p.i++
			continue
		}
		val, ok := p.value(depth + 1)
		if !ok {
			return nil, false
		}
		n.vals = append(n.vals, val)
	}
}

// get returns the value of key in an object node, nil when absent.
func (n *node) get(key string) *node {
	if n == nil || n.kind != 'o' {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

// text decodes a string node. It returns "" for anything else.
func (n *node) text(src []byte) string {
	if n == nil || n.kind != 's' {
		return ""
	}
	raw := src[n.start:n.end]
	inner := raw[1 : len(raw)-1]
	for _, c := range inner {
		if c == '\\' {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return s
			}
			return string(inner)
		}
	}
	return string(inner)
}
