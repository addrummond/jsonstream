package jsonstream

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSliceToPath(t *testing.T) {
	input := []any{1, "foo", 0}
	const expected = `[0]["foo"][1]`
	p := SliceToPath(input)
	if p.String() != expected {
		t.Errorf("Expected %v, got %v", expected, p.String())
	}
}

func TestPathToSlice(t *testing.T) {
	input := Path{
		end: &pathNode{
			index: 0,
			key:   "",
			previous: &pathNode{
				index: -2,
				key:   "foo",
				previous: &pathNode{
					index:    15,
					key:      "",
					previous: nil,
				},
			},
		},
	}
	sl := PathToSlice(input)
	if !reflect.DeepEqual(sl, []any{15, "foo", 0}) {
		t.Errorf(`Expected [15 foo 0], got %+v`, sl)
	}
}

func TestPathEquals(t *testing.T) {
	t.Run("simple case where equality holds", func(t *testing.T) {
		path := Path{
			end: &pathNode{
				index: 0,
				key:   "a",
				previous: &pathNode{
					index: -2,
					key:   "foo",
					previous: &pathNode{
						index:    15,
						key:      "b",
						previous: nil,
					},
				},
			},
		}
		if !PathEquals(path, []any{15, "foo", 0}) {
			t.Errorf("Expected paths to be equal")
		}
	})

	t.Run("compare empty path to empty slice", func(t *testing.T) {
		var path Path
		if !PathEquals(path, []any{}) {
			t.Errorf("Expected paths to be equal")
		}
	})

	t.Run("compare non-empty path to empty slice", func(t *testing.T) {
		path := Path{
			end: &pathNode{
				index:    0,
				key:      "a",
				previous: nil,
			},
		}
		if PathEquals(path, []any{}) {
			t.Errorf("Expected paths not to be equal")
		}
	})

	t.Run("compare non-empty slice to empty path", func(t *testing.T) {
		var path Path
		if PathEquals(path, []any{1}) {
			t.Errorf("Expected paths not to be equal")
		}
	})
}

func TestWithPath(t *testing.T) {
	input := []byte(`[1,2,3,[4,5,{"baz": 99, "foo": [{"bar": "amp", "x": {"yy": [999]}, "baz": "foo"}]}],5]`)
	const expected = `
1:0 ArrayStart  
1:1 Number 1 [0]
1:3 Number 2 [1]
1:5 Number 3 [2]
1:7 ArrayStart  [3]
1:8 Number 4 [3][0]
1:10 Number 5 [3][1]
1:12 ObjectStart  [3][2]
1:20 Number baz=99 [3][2]["baz"]
1:31 ArrayStart foo= [3][2]["foo"]
1:32 ObjectStart  [3][2]["foo"][0]
1:40 String bar=amp [3][2]["foo"][0]["bar"]
1:52 ObjectStart x= [3][2]["foo"][0]["x"]
1:59 ArrayStart yy= [3][2]["foo"][0]["x"]["yy"]
1:60 Number 999 [3][2]["foo"][0]["x"]["yy"][0]
1:74 String baz=foo [3][2]["foo"][0]["baz"]
1:84 Number 5 [4]
	`
	var p Parser
	toks := WithPaths(p.Tokenize(input))
	var out strings.Builder
	for tp := range toks {
		out.WriteString(fmt.Sprintf("%v\n", tp))
	}

	if strings.TrimSpace(out.String()) != strings.TrimSpace(expected) {
		t.Errorf("Expected %v, got %v", expected, out.String())
	}
}
