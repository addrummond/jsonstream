package jsonstream

import (
	"encoding/json"
	"fmt"
	"iter"
	"strings"
)

type TokenWithPath struct {
	Token Token
	Path  Path
}

func (twp TokenWithPath) String() string {
	return fmt.Sprintf("%v %v", twp.Token, twp.Path)
}

// Path represents a sequence of strings and integers >= 0 that gives the path
// to a value inside a JSON document. For example, the sequence {1, "foo", 0}
// is the path to document[1]["foo"][0].
type Path struct {
	end *pathNode
}

const notAnIndex int = -2

type pathNode struct {
	previous *pathNode
	index    int // = notAnIndex if key
	key      string
}

// PathToSlice converts a Path to a slice of int and string values.
func PathToSlice(p Path) []any {
	var result []any
	for n := p.end; n != nil; n = n.previous {
		if n.index == notAnIndex {
			result = append(result, n.key)
		} else {
			result = append(result, n.index)
		}
	}
	var i, j int
	for j = len(result) - 1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// SliceToPath converts a slice of int and string values to a Path.
func SliceToPath(elems []any) Path {
	var root *pathNode
	var pool []pathNode
	for i := len(elems) - 1; i >= 0; i-- {
		elem := elems[i]
		switch e := elem.(type) {
		case int:
			root = newPathNode(&pool, root, e, "")
		case string:
			root = newPathNode(&pool, root, notAnIndex, e)
		default:
			panic("SliceToPath: invalid element type; must be int or string")
		}
	}
	return Path{root}
}

// PathEquals returns true iff the given path is equivalent to the given
// sequence of int and string values.
func PathEquals(path Path, elems []any) bool {
	p := path.end
	for i := len(elems) - 1; i >= 0; i-- {
		if p == nil {
			return false
		}
		elem := elems[i]
		switch e := elem.(type) {
		case int:
			if p.index < 0 || p.index != e {
				return false
			}
		case string:
			if p.index >= 0 || p.key != e {
				return false
			}
		default:
			panic("PathEquals: invalid element type; must be int or string")
		}
		p = p.previous
	}
	return p == nil
}

// String() returns a string representation of the path. The string is a
// sequence of JavaScript indexation operators that can be used to access the
// value (e.g. [0]["foo"][1]]).
func (p Path) String() string {
	var sb strings.Builder
	var rec func(*pathNode)
	rec = func(p *pathNode) {
		if p == nil {
			return
		}
		rec(p.previous)
		if p.index == notAnIndex {
			kb, err := json.Marshal(p.key)
			if err != nil {
				panic("Error marshaling string in jsonstream.Path.String()")
			}
			k := string(kb)
			sb.WriteString(fmt.Sprintf("[%v]", k))
		} else {
			sb.WriteString(fmt.Sprintf("[%v]", p.index))
		}
	}
	rec(p.end)
	return sb.String()
}

func addIndex(pool *[]pathNode, p **pathNode, index int) {
	*p = newPathNode(pool, *p, index, "")
}

func addKeyLookup(p **pathNode, key string, pool *[]pathNode) {
	*p = newPathNode(pool, *p, notAnIndex, key)
}

// simple means of increasing memory locality
func newPathNode(pool *[]pathNode, previous *pathNode, index int, key string) *pathNode {
	if *pool == nil {
		p := make([]pathNode, 128)
		pool = &p
	}
	if len(*pool) == 0 {
		*pool = nil
		return newPathNode(pool, previous, index, key)
	}
	n := &(*pool)[0]
	n.previous = previous
	n.index = index
	n.key = key
	*pool = (*pool)[1:]
	return n
}

// WithPaths converts a sequence of Token values into a sequence of
// TokenWithPath values.
func WithPaths(tokens iter.Seq[Token]) iter.Seq[TokenWithPath] {
	var pool []pathNode
	return func(yield func(TokenWithPath) bool) {
		var currentPath *pathNode
		for t := range tokens {
			if t.Kind == ArrayEnd || t.Kind == ObjectEnd {
				currentPath = currentPath.previous
				continue
			}

			if currentPath != nil {
				if currentPath.index == notAnIndex {
					currentPath = newPathNode(&pool, currentPath.previous, notAnIndex, string(t.Key))
				} else {
					currentPath = newPathNode(&pool, currentPath.previous, currentPath.index+1, "")
				}
			}

			if !yield(TokenWithPath{t, Path{currentPath}}) {
				return
			}

			switch t.Kind {
			case ArrayStart:
				addIndex(&pool, &currentPath, -1)
			case ObjectStart:
				addKeyLookup(&currentPath, "", &pool)
			}
		}
	}
}
