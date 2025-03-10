package jsonstream

import (
	"encoding/json"
	"testing"
)

var input = []byte(`
[
	[1, 2, "foo", {
		"key1": {
			"key2": [
				"foo",
				"bar日本国amp\u0055\n\fblahblah",
				"amp"
			]
		},
		"key2": [
			1e45,
			-55,
			9999,
			"foobaramp"
		]
	}],
	[1, 2, "foo", {
		"key1": {
			"key2": [
				"foo",
				"bar日本国amp\u0055\n\fblahblah",
				"amp"
			]
		},
		"key2": [
			1e45,
			-55,
			9999,
			"foobaramp"
		]
	}],
	[1, 2, "foo", {
		"key1": {
			"key2": [
				"foo",
				"bar日本国amp\u0055\n\fblahblah",
				"amp"
			]
		},
		"key2": [
			1e45,
			-55,
			9999,
			"foobaramp"
		]
	}]
]
`)

func BenchmarkStdlib(b *testing.B) {
	// This isn't a fair benchmark as the stdlib is also constructing a parse
	// tree, but it's useful to check that jsonstream is not performing
	// pathologically badly.

	for range b.N {
		var j any
		err := json.Unmarshal(input, &j)
		if err != nil {
			b.Fatalf("Unexpected Unmarshal error: %v\n", err)
		}
	}
}

func BenchmarkJsonstream(b *testing.B) {
	for range b.N {
		var p Parser
		for t := range p.Tokenize(input) {
			if IsError(t.Kind) {
				b.Fatalf("Unexpected Tokenize error: %+v\n", t)
			}
			if t.Kind == Number {
				t.AsInt()
			} else if t.Kind == String {
				t.AsString()
			}
		}
	}
}

// Notes on benchmarking:
//
// Run just the jsonstream benchmark with profiling:
//   go test -run '^$' -bench BenchmarkJsonstream -benchmem -cpuprofile profile.out
// View the profiling data:
//   go tool pprof profile.out
//     (inside pprof) top # view basic perf data
