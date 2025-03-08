[![GoDoc](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://pkg.go.dev/github.com/addrummond/jsonstream)
[![License](http://img.shields.io/badge/license-mit-blue.svg?style=flat-square)](https://raw.githubusercontent.com/addrummond/jsonstream/master/LICENSE)

# JSONStream

JSONStream is a streaming JSON parser for Go. It's useful if you want to search
through a JSON input stream without parsing all of it, or if you want precise
control over how the input stream is parsed.

Streaming parsers are more
difficult to use than parsers which automatically construct a data structure
from the input. This library is not recommended for general-purpose JSON
processing.

## Key features and design decisions

* Simple [iterator](https://tip.golang.org/doc/go1.23#iterators)-based API.
* Line and column info for all tokens.
* Extensive test suite (including fuzz tests and the
  [JSONTestSuite](https://github.com/nst/JSONTestSuite)).
* Choice of behavior for numeric literals outside the range of `float64` or
  `int`.
* Optional support for JavaScript-style comments and trailing commas.
* Reports errors for all invalid JSON. You need only verify that the JSON
  has the required structure.
* Simple path API that can be used to search for values at a given path.
* Assumes UTF-8 input.
* Surrogate pair escape sequences decoded correctly (e.g. `"\uD834\uDD1E"`
  decodes to UTF-8 `[]byte{0xf0, 0x9d, 0x84, 0x9e}`, i.e. '𝄞').

## Usage

Create a parser:

```go
var p jsonstream.Parser
```

Optionally change the default configuration (default is strict JSON):

```go
p.AllowComments = true
p.AllowTrailingCommas = true
```

Call the `Tokenize` method with a byte slice to obtain an
[iterator](https://pkg.go.dev/iter) over a sequence of tokens:

```go
for tok := range p.Tokenize(input) {
	...
}
```

If you would prefer to pull tokens one-by-one rather than looping, you can use
[`iter.Pull`](https://pkg.go.dev/iter#hdr-Pulling_Values).

Errors are reported via error tokens, for which `IsError(token.Kind)` is true
and `token.AsError()` returns a non-nil `error` value. These tokens have their
`ErrorMsg` field set. JSONStream does not automatically halt on errors.

JSONStream always yields at least one error token for any input that is not
valid JSON. This includes input with mismatched `{}[]`.

### Parsing numeric values

The JSON standard specifies only the syntactic format of numeric literals. The
interpretation of very large and very small values may therefore vary.
JSONStream does not automatically parse numeric literals and so does not
force any particular handling of out of range literals or other edge cases.

The convenience methods `AsInt`, `AsInt32`, `AsInt64`, `AsFloat32`, and
`AsFloat64` are provided for parsing numeric values. These methods add decode
errors to the associated `Parser` object if a value is out of range. Decode
errors can be accessed and manipulated via the `PopDecodeErrorIf`,
`DecodeError`, and `LastDecodeError` methods of `Parser`.

If none of the `As*` methods has the desired behavior, the `Value` field of a
`Token` struct may be accessed directly in order to implement custom parsing of
numeric values.

### Parsing arrays

The sequence of tokens for the array `[1,2,3]` is as follows:

```go
Token{Kind: ArrayStart, ...}
Token{Kind: Number, Value: []byte("1"), ...}
Token{Kind: Number, Value: []byte("2"), ...}
Token{Kind: Number, Value: []byte("3"), ...}
Token{Kind: ArrayEnd, ...}
```

### Parsing objects

Within an object each token represents a value. The associated key is
obtained via the `Key` field. The sequence of tokens for the object
`{"foo": "bar", "baz": "amp"}` is as follows:

```go
Token{Kind: ObjectStart, ...}
Token{Kind: String, Key: []byte("foo"), Value: []byte("bar"), ...}
Token{Kind: String, Key: []byte("baz"), Value: []byte("amp"), ...}
Token{Kind: ObjectEnd, ...}
```

The `KeyAsString` method can be used to obtain a token's key as a string.

### Source position information

Each token has `Line` and `Col` fields for the start of the token, and `Start`
and `End` fields giving the indices of the first and last byte of the token in
the input.

## Performance

JSONStream is written in a simple and straightforward style. It should perform
acceptably for most purposes, but it is not intended to be an ultra high
performance parsing library (such as e.g.
[json-iterator](https://github.com/json-iterator/go)). Informal benchmarking
suggests that performance is a little better than `encoding/json` (though much
depends on whether and how you construct a parsed representation of the input).

## Examples

### Parse an array of integers

```go
import (
	"errors"
	"github.com/addrummond/jsonstream"
)

func parseIntArray(input []byte) ([]int, error) {
	state := 0
	ints := make([]int, 0)
	var p jsonstream.Parser
	for t := range p.Tokenize(input) {
		if err := t.AsError(); err != nil {
			return nil, err
		}

		if state == 0 {
			state++
			if t.Kind != jsonstream.ArrayStart {
				return nil, errors.New("Expected opening '['")
			}
			continue
		}

		if t.Kind == jsonstream.ArrayEnd {
			return ints, nil
		}
		if t.Kind == jsonstream.Number {
			ints = append(ints, t.AsInt())
			continue
		}

		return nil, errors.New("Expected integer or closing ']'")
	}

	return ints, p.DecodeError()
}
```

### Parse an object with string values

```go
import (
	"errors"
	"github.com/addrummond/jsonstream"
)

func parseObjectWithStringValues(input []byte) (map[string]string, error) {
	state := 0
	var p jsonstream.Parser
	dict := make(map[string]string)
	for t := range p.Tokenize(input) {
		if err := t.AsError(); err != nil {
			return nil, err
		}

		if state == 0 {
			state++
			if t.Kind != jsonstream.ObjectStart {
				return nil, errors.New("Expected opening '{'")
			}
			continue
		}

		if t.Kind == jsonstream.ObjectEnd {
			return dict, nil
		}
		if t.Kind == jsonstream.String {
			dict[t.KeyAsString()] = t.AsString()
			continue
		}

		return nil, errors.New("Expected string or closing '}'")
	}

	return dict, p.DecodeError()
}
```

### Search for a value at a given path

```go
import (
	"errors"
	"github.com/addrummond/jsonstream"
)

// Example call:
//
//	findByPath(
//	  []byte(`{"a": {"b": {"c": 1}}}`),
//	  []any{"a", "b", "c"}
//	) // returns "1", nil
func findByPath(input []byte, path []any) (string, error) {
	var p jsonstream.Parser
	for twp := range jsonstream.WithPaths(p.Tokenize(input)) {
		if err := twp.Token.AsError(); err != nil {
			return "", err
		}
		if jsonstream.PathEquals(twp.Path, path) {
			return string(twp.Token.Value), nil
		}
	}
	return "", errors.New("path not found")
}
```
