package jsonstream

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"math/rand"
)

func TestJSONTestSuite(t *testing.T) {
	for filename, base64Contents := range jsonTestInputs {
		contents, err := base64.StdEncoding.DecodeString(base64Contents)
		if err != nil {
			t.Fatalf("Error decoding base64 input: %v", err)
		}

		succeeded := true
		var p Parser
		for t := range p.Tokenize(contents) {
			if IsError(t.Kind) {
				succeeded = false
				break
			}
		}

		if strings.HasPrefix(filename, "y_") && !succeeded || strings.HasPrefix(filename, "n_") && succeeded {
			t.Logf("Parsing %v", filename)
			t.Logf("contents %s", contents)
		}
		if strings.HasPrefix(filename, "y_") && !succeeded {
			t.Errorf("Expected %v to succeed", filename)
		} else if strings.HasPrefix(filename, "n_") && succeeded {
			t.Errorf("Expected %v to fail", filename)
		}
	}
}

func TestTokenize(t *testing.T) {
	t.Run("with comment stripping", func(t *testing.T) {
		const input = `
["xxx", ["ba\u0041r"], "yyy", [ /* a comment inside */ ] // a comment
, {"aaa": "bbb", "x": "y"}, "bbb", {"numeric": 1.4e-99 }, true, false, null ]
`

		const expectedTokSeq = `
{2:2 ArrayStart } |[|
{2:3 String xxx} |"xxx"|
{2:10 ArrayStart } |[|
{2:11 String baAr} |"ba\u0041r"|
{2:22 ArrayEnd } |]|
{2:25 String yyy} |"yyy"|
{2:32 ArrayStart } |[|
{2:34 Comment /* a comment inside */} |/* a comment inside */|
{2:57 ArrayEnd } |]|
{2:59 Comment // a comment} |// a comment|
{3:4 ObjectStart } |{|
{3:12 String aaa=bbb} |"bbb"|
{3:24 String x=y} |"y"|
{3:27 ObjectEnd } |}|
{3:30 String bbb} |"bbb"|
{3:37 ObjectStart } |{|
{3:49 Number numeric=1.4e-99} |1.4e-99|
{3:57 ObjectEnd } |}|
{3:60 True } |true|
{3:66 False } |false|
{3:73 Null } |null|
{3:78 ArrayEnd } |]|
`
		t.Logf("%v\n", tokSeq(input, allowComments, withCorrespondingSourceText))

		if strings.TrimSpace(expectedTokSeq) != strings.TrimSpace(tokSeq(input, allowComments, withCorrespondingSourceText)) {
			t.Fatalf("Unexpected token sequence")
		}
	})

	t.Run("without comment stripping", func(t *testing.T) {
		const input = `
["xxx", ["ba\u0041r"], "yyy", [ ]
, {"aaa": "bbb", "x": "y"}, "bbb", {"numeric": 1.4e-99 } ]
`

		const expectedTokSeq = `
{2:2 ArrayStart }
{2:3 String xxx}
{2:10 ArrayStart }
{2:11 String baAr}
{2:22 ArrayEnd }
{2:25 String yyy}
{2:32 ArrayStart }
{2:34 ArrayEnd }
{3:4 ObjectStart }
{3:12 String aaa=bbb}
{3:24 String x=y}
{3:27 ObjectEnd }
{3:30 String bbb}
{3:37 ObjectStart }
{3:49 Number numeric=1.4e-99}
{3:57 ObjectEnd }
{3:59 ArrayEnd }
`
		t.Logf("%v\n", tokSeq(input, allowComments, withoutCorrespondingSourceText))

		if strings.TrimSpace(expectedTokSeq) != strings.TrimSpace(tokSeq(input, disallowComments, withoutCorrespondingSourceText)) {
			t.Fatalf("Unexpected token sequence")
		}
	})

	t.Run("without comment stripping and comments", func(t *testing.T) {
		const input = `
["xxx", ["ba\u0041r"], "yyy", [ /* a comment inside */ ] // a comment
, {"aaa": "bbb", "x": "y"}, "bbb", {"numeric": 1.4e-99 } ]
`

		const expectedTokSeq = `
{2:2 ArrayStart }
{2:3 String xxx}
{2:10 ArrayStart }
{2:11 String baAr}
{2:22 ArrayEnd }
{2:25 String yyy}
{2:32 ArrayStart }
{2:34 Error: Unexpected token inside array}
{2:57 ArrayEnd }
{2:59 Error: Unexpected token inside array (expecting ',')}
{3:2 Error: Unexpected ',' inside array}
{3:4 ObjectStart }
{3:12 String aaa=bbb}
{3:24 String x=y}
{3:27 ObjectEnd }
{3:30 String bbb}
{3:37 ObjectStart }
{3:49 Number numeric=1.4e-99}
{3:57 ObjectEnd }
{3:59 ArrayEnd }
`

		t.Logf("%v\n", tokSeq(input, disallowComments, withoutCorrespondingSourceText))

		if strings.TrimSpace(expectedTokSeq) != strings.TrimSpace(tokSeq(input, disallowComments, withoutCorrespondingSourceText)) {
			t.Fatalf("Unexpected token sequence")
		}
	})

	t.Run("Error recovery", func(t *testing.T) {
		t.Run("Number with leading zeros", func(t *testing.T) {
			const input = `{"foo": 01, "bar": [02, -01, 3, 0e2]}`
			const expectedTokSeq = `
{1:1 ObjectStart }
{1:9 Error: Leading zeros not permitted in numbers}
{1:20 ArrayStart bar=}
{1:21 Error: Leading zeros not permitted in numbers}
{1:25 Error: Leading zeros not permitted in numbers}
{1:30 Number 3}
{1:33 Number 0e2}
{1:36 ArrayEnd }
{1:37 ObjectEnd }
`

			t.Logf("%v\n", tokSeq(input, allowComments, withoutCorrespondingSourceText))

			if strings.TrimSpace(expectedTokSeq) != strings.TrimSpace(tokSeq(input, disallowComments, withoutCorrespondingSourceText)) {
				t.Fatalf("Unexpected token sequence")
			}
		})
	})
}

func TestBadCommasInArrays(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		const input = "[]"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("commas A-ok", func(t *testing.T) {
		const input = "[1,2,3]"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("commas A-ok 1 elem", func(t *testing.T) {
		const input = "[1]"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("initial comma", func(t *testing.T) {
		const input = "[,1,2,3]"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma 3 elems", func(t *testing.T) {
		const input = "[1,2,3,]"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma 2 elems", func(t *testing.T) {
		const input = "[1,2,]"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma 1 elem", func(t *testing.T) {
		const input = "[1,]"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("comma only", func(t *testing.T) {
		const input = "[,]"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("random case", func(t *testing.T) {
		const input = "[ 1 , 22 , 55 ]"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("with option set, allows trailing commas in arrays", func(t *testing.T) {
		const input = `[1,[2,3,],4,]`
		if !succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("with option set, allows trailing commas but not multiple commas", func(t *testing.T) {
		const input = `[1,[2,,3,],4,]`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("with option set, allows trailing commas but not initial commas", func(t *testing.T) {
		const input = `[,1,[2,3],4]`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("with option set, allows trailing commas but not empty array with a single comma", func(t *testing.T) {
		const input = `[,]`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
}

func TestNestedArrays(t *testing.T) {
	t.Run("nested empty arrays", func(t *testing.T) {
		const input = `[[[[]]]]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("lots of nested empty arrays", func(t *testing.T) {
		const input = `[[[[[[[[[[[[[[]]]]]]]]]]]]]]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested array with empty array members", func(t *testing.T) {
		const input = `[[[[], []]]]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested array with various members", func(t *testing.T) {
		const input = `[[[[1], 2, [], 4]],9]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
}

func TestNestedObjects(t *testing.T) {
	t.Run("empty object", func(t *testing.T) {
		const input = `{ }`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested objects arrays", func(t *testing.T) {
		const input = `{"f":{"g":{}, "x":{}}}`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
}

func TestEmptyArrays(t *testing.T) {
	t.Run("simple case", func(t *testing.T) {
		const input = `[]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested 2", func(t *testing.T) {
		const input = `[[]]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested 2 with spaces", func(t *testing.T) {
		const input = `[ [ ] ]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested 3", func(t *testing.T) {
		const input = `[[[]]]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("nested 3 with spaces", func(t *testing.T) {
		const input = `[ [ [ ] ] ]`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
}

func TestNonStringKeysInObjects(t *testing.T) {
	t.Run("simple case", func(t *testing.T) {
		const input = `{1:2}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
}

func TestBadCommasInObjects(t *testing.T) {
	t.Run("commas A-ok", func(t *testing.T) {
		const input = `{"foo":1,"bar":2,"amp":3}`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("initial comma", func(t *testing.T) {
		const input = `{,"foo":1,"bar":2,"amp":3}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("multiple commas", func(t *testing.T) {
		const input = `{"foo":1,,"bar":2,"amp":3}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma 3 elems", func(t *testing.T) {
		const input = `{"foo":1,"bar":2,"amp":3,}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma 1 elem", func(t *testing.T) {
		const input = `{"foo":1,}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("comma only", func(t *testing.T) {
		const input = `{,}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma in middle of only entry", func(t *testing.T) {
		const input = `{"foo":,}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing comma in middle of entry", func(t *testing.T) {
		const input = `{"bar":"amp","foo":,}`
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("with option set, allows trailing commas in objects", func(t *testing.T) {
		const input = `{"foo": 1, "bar": {"blah": 4,},}`
		if !succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("with option set, allows trailing commas but not multiple commas", func(t *testing.T) {
		const input = `{"foo": 1,, "bar": {"blah": 4,},}`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("with option set, allows trailing commas but not initial commas", func(t *testing.T) {
		const input = `{,"foo": 1, "bar": {"blah": 4}}`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("with option set, allows trailing commas but not empty object with a single comma", func(t *testing.T) {
		const input = `{,}`
		if succeedsAllowingTrailingCommas(input) {
			t.Errorf("Expected to fail")
		}
	})
}

func TestEmptyObject(t *testing.T) {
	t.Run("empty object no spaces", func(t *testing.T) {
		const input = `{}`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("empty object spaces", func(t *testing.T) {
		const input = `{    }`
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
}

func TestNumericZeros(t *testing.T) {
	t.Run("0", func(t *testing.T) {
		const input = "0"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("-0", func(t *testing.T) {
		const input = "-0"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("-00", func(t *testing.T) {
		const input = "-00"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("00", func(t *testing.T) {
		const input = "00"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("01", func(t *testing.T) {
		const input = "01"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("-01", func(t *testing.T) {
		const input = "-01"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
}

func TestTrailingInput(t *testing.T) {
	t.Run("no trailing input", func(t *testing.T) {
		const input = "{}"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("trailing whitespace", func(t *testing.T) {
		const input = "{} \n\t\n"
		if !succeeds(input) {
			t.Errorf("Expected to succeed")
		}
	})
	t.Run("trailing non-whitespace", func(t *testing.T) {
		const input = "{}1"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
	t.Run("trailing whitespace followed by non-whitespace", func(t *testing.T) {
		const input = "{} \n\t1"
		if succeeds(input) {
			t.Errorf("Expected to fail")
		}
	})
}

const allowComments = 0
const disallowComments = 1

const withCorrespondingSourceText = 0
const withoutCorrespondingSourceText = 1

func tokSeq(inp string, comments, correspondingSourceText int) string {
	var sb strings.Builder
	i := 0

	var p Parser
	if comments == allowComments {
		p.AllowComments = true
	}

	for t := range p.Tokenize([]byte(inp)) {
		if i != 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("{%v}", t))
		if withCorrespondingSourceText == correspondingSourceText {
			sb.WriteString(fmt.Sprintf(" |%s|", inp[t.Start:t.End+1]))
		}
		i++
	}
	return sb.String()
}

func succeeds(inp string) bool {
	var p Parser
	for t := range p.Tokenize([]byte(inp)) {
		if IsError(t.Kind) {
			return false
		}
	}
	return true
}

func succeedsAllowingTrailingCommas(inp string) bool {
	var p Parser
	p.AllowTrailingCommas = true
	for t := range p.Tokenize([]byte(inp)) {
		if IsError(t.Kind) {
			return false
		}
	}
	return true
}

func TestAsInt64(t *testing.T) {
	t.Run("simple case", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte("123"), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors != nil || i != 123 {
			t.Errorf("Expected 123, got %v %v", i, p.decodeErrors)
		}
	})
	t.Run("int64 max", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte("9223372036854775807"), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors != nil || i != math.MaxInt64 {
			// int64 cast is needed for 32-bit builds
			t.Errorf("Expected %v, got %v %v", int64(math.MaxInt64), i, p.decodeErrors)
		}
	})
	t.Run("int64 min", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte("-9223372036854775808"), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors != nil || i != math.MinInt64 {
			// int64 cast is needed for 32-bit builds
			t.Errorf("Expected %v, got %v %v", int64(math.MinInt64), i, p.decodeErrors)
		}
	})
	t.Run("value too big to be represented by 64-bit float", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte(fmt.Sprintf("%v999", math.MaxFloat64)), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors == nil || !IsOutOfRangeDecodeError(p.decodeErrors[len(p.decodeErrors)-1]) || i != math.MaxInt64 {
			t.Errorf("Expected %v, got %v %v", int64(math.MaxInt64), i, p.decodeErrors)
		}
	})
	t.Run("value too small to be represented by 64-bit float", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte(fmt.Sprintf("-%v999", math.MaxFloat64)), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors == nil || !IsOutOfRangeDecodeError(p.decodeErrors[len(p.decodeErrors)-1]) || i != math.MinInt64 {
			// int64 cast is needed for 32-bit builds
			t.Errorf("Expected %v, got %v %v", int64(math.MinInt64), i, p.decodeErrors)
		}
	})
	t.Run("(int64_max - 5) written as float", func(t *testing.T) {
		// Although 922337203685477580.2e1 does fit in int max, when this string is
		// parsed as a 64-bit float, it is outside the range where all integers can
		// be exactly represented, and is greater than the largest
		// exactly-representable integer below 2^63. The function should therefore
		// return int64 max as for the case where the value exceeds the range of a
		// 64-bit float.
		var p Parser
		tok := &Token{Kind: Number, Value: []byte("922337203685477580.2e1"), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors == nil || !IsOutOfRangeDecodeError(p.decodeErrors[len(p.decodeErrors)-1]) || i != math.MaxInt64 {
			// int64 cast is needed for 32-bit builds
			t.Errorf("Expected %v, got %v %v", int64(math.MaxInt64), i, p.decodeErrors)
		}
	})
	t.Run("(int64_min + 5) written as float", func(t *testing.T) {
		var p Parser
		tok := &Token{Kind: Number, Value: []byte("-922337203685477580.2e1"), parser: &p}
		if i := tok.AsInt64(); p.decodeErrors == nil || !IsOutOfRangeDecodeError(p.decodeErrors[len(p.decodeErrors)-1]) || i != math.MinInt64 {
			// int64 cast is needed for 32-bit builds
			t.Errorf("Expected %v, got %v %v", int64(math.MinInt64), i, p.decodeErrors)
		}
	})
}

func TestSurrogatePairs(t *testing.T) {
	t.Run("treble clef from RFC8259", func(t *testing.T) {
		const input = `"\uD834\uDD1E"`
		var p Parser
		for tok := range p.Tokenize([]byte(input)) {
			if tok.Kind != String || tok.AsString() != "𝄞" {
				t.Fatalf("Expected 𝄞, got %v", tok.AsString())
			}
			return
		}
		t.Fatalf("Expected at least one token")
	})
	t.Run("multiple treble clefs", func(t *testing.T) {
		const input = `"\uD834\uDD1E\uD834\uDD1E\uD834\uDD1E\uD834\uDD1E\uD834\uDD1E"`
		var p Parser
		for tok := range p.Tokenize([]byte(input)) {
			if tok.Kind != String || tok.AsString() != "𝄞𝄞𝄞𝄞𝄞" {
				t.Fatalf("Expected 𝄞𝄞𝄞𝄞𝄞, got %v", tok.AsString())
			}
			return
		}
		t.Fatalf("Expected at least one token")
	})
	t.Run("treble clef surrogate pair with escape for 'A' interrupting", func(t *testing.T) {
		const input = `"\uD834\u0041\uDD1E"`
		var p Parser
		for tok := range p.Tokenize([]byte(input)) {
			if tok.Kind != String || !bytes.Equal(tok.Value, []byte{0xef, 0xbf, 0xbd, 0x41, 0xef, 0xbf, 0xbd}) {
				t.Fatalf(`Expected <replacement char>A<replacement char>, got %+v`, tok.Value)
			}
			return
		}
		t.Fatalf("Expected at least one token")
	})
	t.Run("sequence of treble clef surrogate pair with escape for 'A' interrupting", func(t *testing.T) {
		const input = `"\uD834\u0041\uDD1E\uD834\u0041\uDD1E\uD834\u0041\uDD1E"`
		var p Parser
		for tok := range p.Tokenize([]byte(input)) {
			if tok.Kind != String || !bytes.Equal(tok.Value, []byte{0xef, 0xbf, 0xbd, 0x41, 0xef, 0xbf, 0xbd, 0xef, 0xbf, 0xbd, 0x41, 0xef, 0xbf, 0xbd, 0xef, 0xbf, 0xbd, 0x41, 0xef, 0xbf, 0xbd}) {
				t.Fatalf(`Expected <replacement char>A<replacement char> 3 times, got %+v`, tok.Value)
			}
			return
		}
		t.Fatalf("Expected at least one token")
	})
	t.Run("sequence of 4 'A' characters specified via \\u escapes", func(t *testing.T) {
		const input = `"\u0041\u0041\u0041\u0041"`
		var p Parser
		for tok := range p.Tokenize([]byte(input)) {
			if tok.Kind != String || !bytes.Equal(tok.Value, []byte{0x41, 0x41, 0x41, 0x41}) {
				t.Fatalf(`Expected AAAA, got %+v`, tok.Value)
			}
			return
		}
		t.Fatalf("Expected at least one token")
	})
	t.Run("does not crash for bad \\u escapes following surrogate pairs", func(t *testing.T) {
		const input = `"\uD834\u!!04"`
		var p Parser
		error := false
		for tok := range p.Tokenize([]byte(input)) {
			if tok.AsError() != nil {
				error = true
			}
		}
		if !error {
			t.Fatalf("Expected error")
		}
	})
}

const fuzzIterations = 10000

// Check that parser doesn't panic or loop indefinitely on random input
func TestFuzz(t *testing.T) {
	rand := rand.New(rand.NewSource(123))

	t.Run("random bytes", func(t *testing.T) {
		for i := 0; i < fuzzIterations; i++ {
			a := make([]byte, i)
			rand.Read(a)
			var p Parser
			for range p.Tokenize(a) {
			}
		}
	})
	t.Run("random characters of interest", func(t *testing.T) {
		chars := "{}{}{}[][][],/:\"'0123456789.+-eEabc\\fn{}[],/:\"'0123456789.+-eEabc\\fn大日本國璽\n中华人民共和国مصرГосударственныйราชอาณาจักรไทย"
		var indices []int
		c := 0
		for c < len(chars) {
			r, sz := utf8.DecodeRuneInString(chars[c:])
			if r == utf8.RuneError {
				panic("Invalid character")
			}
			indices = append(indices, c)
			c += sz
		}

		for i := 0; i < fuzzIterations; i++ {
			a := make([]byte, i)
			j := 0
			for j < len(a) {
				idx := rand.Intn(len(indices))
				r, sz := utf8.DecodeRuneInString(chars[indices[idx]:])
				if r == utf8.RuneError {
					panic("Invalid character")
				}
				if sz > len(a)-j {
					break
				}
				j += utf8.EncodeRune(a[j:], r)
			}

			for len(a) > 0 && a[len(a)-1] == 0 {
				a = a[:len(a)-1]
			}

			var p Parser
			for range p.Tokenize(a) {
			}
		}
	})
}
