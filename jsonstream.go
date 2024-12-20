// Package jsonstreaœm provides a JSON tokenizer that reports line and
// column information for tokens. It optionally supports /* */ and // comment
// syntax as an extension to standard JSON.
package jsonstream

import (
	"fmt"
	"iter"
	"math"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Kind represents the kind of a JSON token.
type Kind int

const (
	// A '{' token
	ObjectStart Kind = iota
	// A '}' token
	ObjectEnd
	// A '[' token]
	ArrayStart
	// A ']' token
	ArrayEnd
	// A string
	String Kind = iota | primval
	// A number
	Number Kind = iota | primval
	// A true boolean value
	True Kind = iota | primval
	// A false boolean value
	False Kind = iota | primval
	// A null value
	Null Kind = iota | primval
	// A // or /* */ comment. If you need to distinguish between the two, you can
	// look at the second byte of the token's Value field.
	Comment Kind = iota
	// A parse error
	ErrorTrailingInput Kind = iota | isError
	// An unexpected EOF was encountered
	ErrorUnexpectedEOF
	// An unexpected token was encountered
	ErrorUnexpectedToken
	// There is a trailing comma in an object or array (not permitted by the JSON
	// standard).
	ErrorTrailingComma
	// There is a comma in an unexpected position (either immediately following '['
	// or '{' or immediately following another comma).
	ErrorUnexpectedComma
	// An unexpected character was encountered while tokenizing the input.
	ErrorUnexpectedCharacter
	// A numeric literal has leading zeros (not permitted by the JSON standard).
	// Tokens with this kind can also be treated as tokens of kind Number, if you
	// wish to be liberal in what you accept.
	ErrorLeadingZerosNotPermitted
	// A decimal point was not follwed by a digit.
	ErrorExpectedDigitAfterDecimalPoint
	// The 'e' (or 'E') in a number was not followed by a digit.
	ErrorExpectedDigitFollowingEInNumber
	// A bad "\uXXXX" escape sequence was encountered in a string.
	ErrorBadUnicodeEscape
	// A control character not permitted by the JSON standard was found inside a
	// string.
	ErrorIllegalControlCharInsideString
	// UTF-8 decoding failing inside a string.
	ErrorUTF8DecodingErrorInsideString
	colon
	comma
)

const (
	isError = (1 << 29)
	primval = (1 << 30)
)

// IsError returns true for Error* token kinds and false for all others.
func IsError(k Kind) bool {
	return k&isError != 0
}

func (k Kind) String() string {
	if IsError(k) {
		return "Error"
	}

	switch k {
	case ObjectStart:
		return "ObjectStart"
	case ObjectEnd:
		return "ObjectEnd"
	case ArrayStart:
		return "ArrayStart"
	case ArrayEnd:
		return "ArrayEnd"
	case String:
		return "String"
	case Number:
		return "Number"
	case True:
		return "True"
	case False:
		return "False"
	case Null:
		return "Null"
	case Comment:
		return "Comment"
	case colon:
		return "colon"
	case comma:
		return "comma"
	}
	return "<unknown Kind>"
}

// Parser is a streaming JSON parser. It is valid when default initialized.
type Parser struct {
	AllowComments       bool // Set to true to allow /* */ and // comments in the input
	AllowTrailingCommas bool // Set to true to allow trailing commas in arrays and objects (does not allow initial commas or multiple commas)
	errors              []Token
	decodeErrors        []error
}

// Token represents a JSON token.
type Token struct {
	Line     int    // the line number of the first character of the token
	Col      int    // the column of the first character of the token
	Start    int    // the start position of the token in the input (byte index)
	End      int    // the end position of the token in the input (byte index)
	Key      []byte // the key of the token, or nil if none (may be a sub-slice of the input)
	Kind     Kind   // the kind of token
	Value    []byte // the value of the token (may be a sub-slice of the input).
	ErrorMsg string // error message set if IsError(token.Kind) == true
	parser   *Parser
}

func appendDecodeError(t *Token, err error) {
	t.parser.decodeErrors = append(t.parser.decodeErrors, err)
}

func (t Token) String() string {
	if IsError(t.Kind) {
		return fmt.Sprintf("%v:%v Error: %v", t.Line, t.Col, t.ErrorMsg)
	}
	var key string
	if len(t.Key) > 0 {
		key = string(t.Key) + "="
	}
	return fmt.Sprintf("%v:%v %v %v%s", t.Line, t.Col, t.Kind, key, t.Value)
}

func (t Token) Error() string {
	return t.String()
}

// AsError returns an error value if the token is an error token or nil
// otherwise.
func (t Token) AsError() error {
	if IsError(t.Kind) {
		return fmt.Errorf("%w", t)
	}
	return nil
}

// AsBool returns the token's value as a bool. Its return value is defined only
// for tokens where Kind == True or Kind == False.
func (t *Token) AsBool() bool {
	if t.Kind != True && t.Kind != False {
		panic("jsonstream: AsBool called on non-boolean token")
	}
	return t.Kind == True
}

// AsString returns the token's value as a string. Its return value is defined
// only for tokens where Kind == String.
func (t *Token) AsString() string {
	if t.Kind != String {
		panic("jsonstream: AsString called on non-string token")
	}
	return string(t.Value)
}

// KeyAsString returns the token's associated object Key as a string.
func (t *Token) KeyAsString() string {
	if t.Key == nil {
		panic("jsonstream: KeyAsString called on token with no key")
	}
	return string(t.Key)
}

// AsFloat64 returns the token's value as a float64. Its return value is
// defined only for tokens where Kind == Number. The input is parsed using
// strconv.ParseFloat. If ParseFloat signals an error, a decode error is added
// to the associated Parser.
func (t *Token) AsFloat64() float64 {
	f, err := strconv.ParseFloat(string(t.Value), 64)
	if err != nil {
		appendDecodeError(t, err)
	}
	return f
}

// AsFloat32 returns the token's value as a float32. Its return value is
// defined only for tokens where Kind == Number. The input is parsed using
// strconv.ParseFloat. If ParseFloat signals an error, a decode error is added
// to the associated Parser.
func (t *Token) AsFloat32() float32 {
	f, err := strconv.ParseFloat(string(t.Value), 32)
	if err != nil {
		appendDecodeError(t, err)
		return float32(f)
	}
	return float32(f)
}

type intConversionError int

const (
	notAnInteger intConversionError = iota
	outOfRange
)

func (e intConversionError) Error() string {
	switch e {
	case notAnInteger:
		return "not an integer"
	case outOfRange:
		return "out of range"
	}
	return "unknown IntConversionError"
}

// IsNonIntegerDecodeError Returns true iff a decode error results from an
// attempt to parse a non-integer numeric value as an integer.
func IsNonIntegerDecodeError(e error) bool {
	e, ok := e.(intConversionError)
	return ok && e == notAnInteger
}

// IsOutOfRangeDecodeError returns true iff a decode error results from an
// attempt to parse a numeric value that is out of range.
func IsOutOfRangeDecodeError(e error) bool {
	e, ok := e.(intConversionError)
	return ok && e == outOfRange
}

// Removes the last decode error if it satisfies the predicate. This is useful
// with the supplied predicates IsNonIntegerDecodeError and
// IsOutOfRangeDecodeError. For example, if
// p.PopDecodeErrorIf(IsOutOfRangeDecodeError) is called immediately after
// AsInt(), then errors caused by out of range integers will be ignored.
func (p *Parser) PopDecodeErrorIf(predicate func(error) bool) {
	if len(p.decodeErrors) > 0 && predicate(p.decodeErrors[len(p.decodeErrors)-1]) {
		p.decodeErrors = p.decodeErrors[len(p.decodeErrors)-1:]
	}
}

// DecodeError returns the first decode error if any, or nil otherwise. A decode
// error is an error caused by invalid input to AsInt, AsInt32, AsInt64,
// AsFloat32, or AsFloat64.
func (p *Parser) DecodeError() error {
	if len(p.decodeErrors) == 0 {
		return nil
	}
	return (p.decodeErrors)[0]
}

// LastDecodeError returns the first decode error if any, or nil otherwise. A
// decode error is an error caused by invalid input to AsInt, AsInt32, AsInt64,
// AsFloat32, or AsFloat64.
func (p *Parser) LastDecodeError() error {
	if len(p.decodeErrors) == 0 {
		return nil
	}
	return p.decodeErrors[len(p.decodeErrors)-1]
}

// LastDecodeError returns a slice containing all decode errors in the order
// they occurred. A decode error is an error occurring in AsInt, AsInt32,
// AsInt64, AsFloat32, or AsFloat64.
func (p *Parser) DecodeErrors() []error {
	return p.decodeErrors
}

// Maximum integer value x s.t. all y s.t. 0 <= y <= x can be exactly
// represented as a float64. Also works for negative values (no two's complement
// asymmetry for floats).
// https://stackoverflow.com/a/1848762
const float64ExactIntMax = 9007199254740992

// AsInt returns the token's value as an int. Its return value is defined
// only for tokens where Kind == Number. If the value is not an integer or does
// not fit in an int, then a decode error is added to the associated Parser. A
// decode error is not added for in-range integer values specified
// using floating point syntax (e.g. '1.5e1', which evaluates to 15). If the
// decode error satisfies IsNotAnIntegerError(err) or IsOutOfRangeError(err)
// then the returned value approximates the value of the float as
// closely as possible. The function may therefore be used to parse floating
// point values as the nearest int value.
//
// For more on decode errors see the following methods of Parser: DecodeError(),
// LastDecodeError(), DecodeErrors(), PopDecodeErrorIf().
func (t *Token) AsInt() int {
	if math.MaxUint == 0xFFFFFFFF {
		return int(t.AsInt32())
	}
	if math.MaxUint == 0xFFFFFFFFFFFFFFFF {
		return int(t.AsInt64())
	}
	panic("unsupported int size")
}

// AsInt64 is like AsInt, but for int64.
func (t *Token) AsInt64() int64 {
	// As integer parsing is simple, we can typically avoid the conversion to
	// string needed to use strconv.Atoi. The exception is the case where an
	// integer value has been written using float syntax (e.g. 1.0, 1.5e3).

	if t.Kind != Number {
		panic("jsonstream: AsInt64 called on non-Number token")
	}

	var tot int64
	if t.Value[0] == '-' {
		for i := 1; i < len(t.Value); i++ {
			if t.Value[i] < '0' || t.Value[i] > '9' {
				goto slow_path
			}
			tot -= int64(t.Value[i] - '0')
			if tot > 0 {
				appendDecodeError(t, outOfRange)
				return math.MinInt64
			}
			if i+1 < len(t.Value) {
				tot *= 10
				if tot > 0 {
					appendDecodeError(t, outOfRange)
					return math.MinInt64
				}
			}
		}
	} else {
		for i := 0; i < len(t.Value); i++ {
			if t.Value[i] < '0' || t.Value[i] > '9' {
				goto slow_path
			}
			tot += int64(t.Value[i] - '0')
			if tot < 0 {
				appendDecodeError(t, outOfRange)
				return math.MaxInt64
			}
			if i+1 < len(t.Value) {
				tot *= 10
				if tot < 0 {
					appendDecodeError(t, outOfRange)
					return math.MaxInt64
				}
			}
		}
	}

	return tot

	// It contains some characters other than an optional '-' prefix and digits
	// 0-9. In this case we'll still parse it if it's a valid 64-bit float,
	// is integer valued, and fits in an int (e.g. 1.0, 1.5e3).
slow_path:
	f, err := strconv.ParseFloat(string(t.Value), 64)
	if err != nil {
		// This should always be an 'out of range' error, given that we know the
		// syntax is valid.
		if f >= 9.223372036854776e+18 {
			appendDecodeError(t, outOfRange)
			return math.MaxInt64
		}
		if f <= -9.223372036854776e+18 {
			appendDecodeError(t, outOfRange)
			return math.MinInt64
		}
		appendDecodeError(t, outOfRange)
		return int64(f)
	}
	if math.Floor(f) == f { // redundant with next check, but makes it possible to give distinct 'out of range' vs. 'not an int' errors
		if f >= -float64ExactIntMax && f <= float64ExactIntMax {
			return int64(f)
		}
		// If we get here, then the parsed value may not exactly correspond to the
		// written value.
		if f >= 9223372036854776000 {
			appendDecodeError(t, outOfRange)
			return math.MaxInt64
		}
		if f < -9223372036854776000 {
			appendDecodeError(t, outOfRange)
			return math.MinInt64
		}
		appendDecodeError(t, outOfRange)
		return int64(f)
	}

	rounded := math.Round(f)
	if rounded >= 9223372036854776000 {
		appendDecodeError(t, outOfRange)
		return math.MaxInt64
	}
	if rounded < -9223372036854776000 {
		appendDecodeError(t, outOfRange)
		return math.MinInt64
	}
	appendDecodeError(t, notAnInteger)
	return int64(rounded)
}

// AsInt32 is like AsInt, but for int32.
func (t *Token) AsInt32() int32 {
	if t.Kind != Number {
		panic("jsonstream: AsInt32 called on non-Number token")
	}

	var tot int32
	if t.Value[0] == '-' {
		for i := 1; i < len(t.Value); i++ {
			if t.Value[i] < '0' || t.Value[i] > '9' {
				goto slow_path
			}
			tot -= int32(t.Value[i] - '0')
			if tot > 0 {
				appendDecodeError(t, outOfRange)
				return math.MinInt32
			}
			if i+1 < len(t.Value) {
				tot *= 10
				if tot > 0 {
					appendDecodeError(t, outOfRange)
					return math.MinInt32
				}
			}
		}
	} else {
		for i := 0; i < len(t.Value); i++ {
			if t.Value[i] < '0' || t.Value[i] > '9' {
				goto slow_path
			}
			tot += int32(t.Value[i] - '0')
			if tot < 0 {
				appendDecodeError(t, outOfRange)
				return math.MaxInt32
			}
			if i+1 < len(t.Value) {
				tot *= 10
				if tot < 0 {
					appendDecodeError(t, outOfRange)
					return math.MaxInt32
				}
			}
		}
	}

	return tot

	// It contains some characters other than an optional '-' prefix and digits
	// 0-9. In this case we'll still parse it if it's a valid 64-bit float,
	// is integer valued, and fits in an int (e.g. 1.0, 1.5e3).
slow_path:
	f, err := strconv.ParseFloat(string(t.Value), 64)
	if err != nil {
		// This should always be an 'out of range' error, given that we know the
		// syntax is valid.
		if f >= 2.1474836e+09 {
			appendDecodeError(t, outOfRange)
			return math.MaxInt32
		}
		if f <= -2.1474836e+09 {
			appendDecodeError(t, outOfRange)
			return math.MinInt32
		}
		return int32(f)
	}
	if math.Floor(f) == f { // redundant with next check, but makes it possible to give distinct 'out of range' vs. 'not an int' errors
		if f >= -float64ExactIntMax && f <= float64ExactIntMax {
			if f > float64(math.MaxInt32) {
				appendDecodeError(t, outOfRange)
				return math.MaxInt32
			}
			if f < float64(math.MinInt32) {
				appendDecodeError(t, outOfRange)
				return math.MinInt32
			}
			return int32(f)
		}
		// If we get here, then the parsed value may not exactly correspond to the
		// written value.
		if f >= 2.1474836e+09 {
			appendDecodeError(t, outOfRange)
			return math.MaxInt32
		}
		if f <= -2.1474836e+09 {
			appendDecodeError(t, outOfRange)
			return math.MinInt32
		}
		return int32(f)
	}

	f = math.Round(f)
	if f > float64(math.MaxInt32) {
		appendDecodeError(t, outOfRange)
		return math.MaxInt32
	}
	if f < float64(math.MinInt32) {
		appendDecodeError(t, outOfRange)
		return math.MinInt32
	}
	appendDecodeError(t, notAnInteger)
	return int32(f)
}

func mkErr(errorKind Kind, line, col int, msg string) Token {
	return Token{
		Kind:     errorKind,
		Line:     line,
		Col:      col,
		ErrorMsg: msg,
	}
}

// a non-nil empty byte slice
var notNilEmptyByteSlice = []byte{}

// Tokenize returns an iter.Seq[Token] from a byte slice input.
func (p *Parser) Tokenize(inp []byte) iter.Seq[Token] {
	next_, stop := iter.Pull(rawTokenize(p, inp))

	var haltedOnComment bool

	next := func(yield func(Token) bool) (t Token, ok bool) {
		if !p.AllowComments {
			return next_()
		}
		for {
			t, ok = next_()
			if !ok {
				return
			}
			if t.Kind != Comment {
				return
			}
			if !yield(t) {
				ok = false
				// signal that any error caused by this comment token should be
				// suppressed because the consumer halted on the comment itself
				haltedOnComment = true
				return
			}
		}
	}

	var main func(yield func(Token) bool)
	var tokArray func(yield func(Token) bool) bool
	var tokObject func(yield func(Token) bool) bool

	main = func(yield func(Token) bool) {
		yieldErr := func(errorKind Kind, line, col int, msg string) bool {
			if !haltedOnComment {
				err := mkErr(errorKind, line, col, msg)
				p.errors = append(p.errors, err)
				return yield(err)
			}
			return true
		}

		for i := 0; ; i++ {
			t, ok := next(yield)
			if !ok {
				return
			}

			if i > 0 {
				yieldErr(ErrorTrailingInput, t.Line, t.Col, "Trailing input")
				return
			}

			switch t.Kind {
			case ObjectStart:
				if !yield(t) {
					return
				}
				if !tokObject(yield) {
					return
				}
			case ArrayStart:
				if !yield(t) {
					return
				}
				if !tokArray(yield) {
					return
				}
			case ObjectEnd, ArrayEnd, comma, colon:
				if !yieldErr(ErrorUnexpectedToken, t.Line, t.Col, "Unexpected token") {
					return
				}
			default:
				if !yield(t) {
					return
				}
			}
		}
	}

	tokArray = func(yield func(Token) bool) bool {
		yieldErr := func(errorKind Kind, line, col int, msg string) bool {
			if !haltedOnComment {
				return yield(mkErr(errorKind, line, col, msg))
			}
			return true
		}

		afterComma := false
		for {
			valtok, ok := next(yield)
			if !ok {
				yieldErr(ErrorUnexpectedEOF, valtok.Line, valtok.Col, "Unexpected EOF (expected closing ']')")
				return false
			}

			if valtok.Kind == ArrayEnd {
				if afterComma && !p.AllowTrailingCommas {
					if !yieldErr(ErrorTrailingComma, valtok.Line, valtok.Col, "Trailing ','") {
						return false
					}
				}
				return yield(valtok)
			}

			switch valtok.Kind {
			case ArrayStart:
				if !yield(valtok) {
					return false
				}
				if !tokArray(yield) {
					return false
				}
			case ObjectStart:
				if !yield(valtok) {
					return false
				}
				if !tokObject(yield) {
					return false
				}
			case String, Number, True, False, Null, ErrorLeadingZerosNotPermitted:
				if !yield(valtok) {
					return false
				}
			case comma:
				afterComma = true
				if !yieldErr(ErrorUnexpectedComma, valtok.Line, valtok.Col, "Unexpected ',' inside array") {
					return false
				}
				continue
			default:
				if !yieldErr(ErrorUnexpectedToken, valtok.Line, valtok.Col, "Unexpected token inside array") {
					return false
				}
			}

			t, ok := next(yield)
			if !ok {
				yieldErr(ErrorUnexpectedEOF, t.Line, t.Col, "Unexpected EOF inside array")
				return false
			}

			if t.Kind == ArrayEnd {
				return yield(t)
			}
			if t.Kind != comma {
				if !yieldErr(ErrorUnexpectedToken, t.Line, t.Col, "Unexpected token inside array (expecting ',')") {
					return false
				}
			}
			afterComma = true
		}
	}

	tokObject = func(yield func(Token) bool) bool {
		yieldErr := func(errorKind Kind, line, col int, msg string) bool {
			if !haltedOnComment {
				return yield(mkErr(errorKind, line, col, msg))
			}
			return true
		}

		afterComma := false
		for {
			keytok, ok := next(yield)
			if !ok {
				yieldErr(ErrorUnexpectedEOF, keytok.Line, keytok.Col, "Unexpected EOF (expected closing '}')")
				return false
			}

			if keytok.Kind == ObjectEnd {
				if afterComma && !p.AllowTrailingCommas {
					if !yieldErr(ErrorTrailingComma, keytok.Line, keytok.Col, "Trailing ','") {
						return false
					}
				}
				return yield(keytok)
			}

			if keytok.Kind != String {
				if keytok.Kind == comma {
					if !yieldErr(ErrorUnexpectedComma, keytok.Line, keytok.Col, "Unexpected ',' inside object (expecting key)") {
						return false
					}
				} else {
					if !yieldErr(ErrorUnexpectedToken, keytok.Line, keytok.Col, "Unexpected token inside object (expecting key)") {
						return false
					}
				}
				keytok.Value = notNilEmptyByteSlice // error recovery; set empty key
			}

			t, ok := next(yield)
			if !ok || t.Kind != colon {
				if !yieldErr(ErrorUnexpectedToken, t.Line, t.Col, "Unexpected token inside object (expecting ':')") {
					return false
				}
			}

			valtok, ok := next(yield)
			if !ok {
				yieldErr(ErrorUnexpectedEOF, t.Line, t.Col, "Unexpected EOF")
				return false
			}

			valtok.Key = keytok.Value
			// Hack to distinguish tokens that have no key from tokens that have an
			// empty key. This shouldn't matter to users of the library (as they
			// should keep track of this anyway) but we can use it to panic if
			// KeyAsString is called on a token with no key.
			if valtok.Key == nil {
				valtok.Key = notNilEmptyByteSlice
			}

			switch valtok.Kind {
			case ArrayStart:
				if !yield(valtok) {
					return false
				}
				if !tokArray(yield) {
					return false
				}
			case ObjectStart:
				if !yield(valtok) {
					return false
				}
				if !tokObject(yield) {
					return false
				}
			case String, Number, True, False, Null, ErrorLeadingZerosNotPermitted:
				if !yield(valtok) {
					return false
				}
			default:
				if !yieldErr(ErrorUnexpectedToken, t.Line, t.Col, "Unexpected token inside object") {
					return false
				}
			}

			t, ok = next(yield)
			if !ok {
				yieldErr(ErrorUnexpectedEOF, t.Line, t.Col, "Unexpected EOF")
				return false
			}

			if t.Kind == ObjectEnd {
				return yield(t)
			}
			if t.Kind != comma {
				if !yieldErr(ErrorUnexpectedToken, t.Line, t.Col, "Unexpected token") {
					return false
				}
			}
			afterComma = true
		}
	}

	return func(yield func(Token) bool) {
		defer stop()
		main(yield)
	}
}

func rawTokenize(p *Parser, inp []byte) iter.Seq[Token] {
	return func(yield func(Token) bool) {
		pos := 0
		lineStart := 0
		line := 1
		nextMustBeSep := false

		yieldErr := func(errorKind Kind, line, col int, msg string) bool {
			err := mkErr(errorKind, line, col, msg)
			p.errors = append(p.errors, err)
			return yield(err)
		}

	parseloop:
		for {
			if pos >= len(inp) {
				return
			}

			c := inp[pos]
			if nextMustBeSep {
				switch c {
				case ' ', '\r', '\n', '\t', '/', ':', ',', '[', ']', '{', '}':
					nextMustBeSep = false
				default:
					if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, "Unexpected character") {
						return
					}
					pos++
					continue parseloop
				}
			}

			switch c {
			case ' ', '\r', '\n', '\t':
				pos++
				if c == '\n' {
					line++
					lineStart = pos - 1
				}
				continue parseloop
			case '/':
				start := pos
				startLine := line
				startCol := pos - lineStart
				pos++
				if pos >= len(inp) {
					if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, "Unexpected '/'") {
						return
					}
					pos++
					continue parseloop
				}
				switch inp[pos] {
				case '*':
					for {
						pos++
						if pos >= len(inp) {
							yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF inside comment")
							return
						}
						if inp[pos] == '\n' {
							line++
							lineStart = pos
							pos++
						} else if inp[pos] == '*' {
							pos++
							if pos >= len(inp) {
								yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF inside /* ... */ comment")
								return
							}
							if inp[pos] == '/' {
								if !yield(Token{Line: startLine, Col: startCol, Start: start, End: pos, Kind: Comment, Value: inp[start : pos+1]}) {
									return
								}
								pos++
								continue parseloop
							}
						}
					}
				case '/':
					for {
						pos++
						if pos >= len(inp) {
							yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF inside // comment")
							return
						}
						if inp[pos] == '\n' {
							if !yield(Token{Line: startLine, Col: startCol, Start: start, End: pos - 1, Kind: Comment, Value: inp[start:pos]}) {
								return
							}
							pos++
							line++
							lineStart = pos - 1
							continue parseloop
						}
					}
				default:
					if !yieldErr(ErrorUnexpectedToken, line, pos-lineStart, "Unexpected '/'") {
						return
					}
					pos++
					continue parseloop
				}
			case ':':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: colon}) {
					return
				}
				pos++
			case ',':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: comma}) {
					return
				}
				pos++
			case '[':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: ArrayStart}) {
					return
				}
				pos++
			case '{':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: ObjectStart}) {
					return
				}
				pos++
			case ']':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: ArrayEnd}) {
					return
				}
				pos++
			case '}':
				if !yield(Token{Line: line, Col: pos - lineStart, Start: pos, End: pos, Kind: ObjectEnd}) {
					return
				}
				pos++
			case 't':
				start := pos
				startCol := pos - lineStart
				if pos+3 >= len(inp) || inp[pos+1] != 'r' || inp[pos+2] != 'u' || inp[pos+3] != 'e' {
					if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, "Unexpected 't'") {
						return
					}
					pos++
					continue parseloop
				}
				pos += 4
				nextMustBeSep = true
				if !yield(Token{Line: line, Col: startCol, Start: start, End: pos, Kind: True}) {
					return
				}
			case 'f':
				start := pos
				startCol := pos - lineStart
				if pos+4 >= len(inp) || inp[pos+1] != 'a' || inp[pos+2] != 'l' || inp[pos+3] != 's' || inp[pos+4] != 'e' {
					if !yieldErr(ErrorUnexpectedCharacter, line, startCol, "Unexpected 'f'") {
						return
					}
					pos++
					continue parseloop
				}
				pos += 5
				nextMustBeSep = true
				if !yield(Token{Line: line, Col: startCol, Start: start, End: pos, Kind: False}) {
					return
				}
			case 'n':
				start := pos
				startCol := pos - lineStart
				if pos+3 >= len(inp) || inp[pos+1] != 'u' || inp[pos+2] != 'l' || inp[pos+3] != 'l' {
					if !yieldErr(ErrorUnexpectedCharacter, line, startCol, "Unexpected 'n'") {
						return
					}
					pos++
					continue parseloop
				}
				pos += 4
				nextMustBeSep = true
				if !yield(Token{Line: line, Col: startCol, Start: start, End: pos, Kind: Null}) {
					return
				}
			case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				start := pos
				startCol := pos - lineStart
				if c == '-' {
					pos++
					if pos >= len(inp) {
						yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF")
						return
					}
					if inp[pos] < '0' || inp[pos] > '9' {
						if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, "Unexpected char after '-'") {
							return
						}
						pos++ // continue to parse number ignoring whatever this char was (after yielding error)
					}
				}
				firstDigitI := pos // we'll check later for a leading zero here
				pos++
				for pos < len(inp) && inp[pos] >= '0' && inp[pos] <= '9' {
					pos++
				}
				if pos < len(inp) && inp[pos] == '.' {
					pos++
					if pos >= len(inp) || inp[pos] < '0' || inp[pos] > '9' {
						if !yieldErr(ErrorExpectedDigitAfterDecimalPoint, line, pos-lineStart, "Expected digit after '.' in number") {
							return
						}
						pos++
						continue parseloop
					}
					for {
						pos++
						if pos >= len(inp) || inp[pos] < '0' || inp[pos] > '9' {
							break
						}
					}
				}
				if pos < len(inp) && (inp[pos] == 'e' || inp[pos] == 'E') {
					pos++
					if pos >= len(inp) {
						yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF")
						return
					}
					if inp[pos] == '+' || inp[pos] == '-' {
						pos++
					}
					if pos >= len(inp) {
						yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF")
						return
					}
					if inp[pos] < '0' || inp[pos] > '9' {
						if !yieldErr(ErrorExpectedDigitFollowingEInNumber, line, pos-lineStart, "Expected digit following 'e' in number") {
							return
						}
						pos++
						continue parseloop
					}
					pos++
					for pos < len(inp) && inp[pos] >= '0' && inp[pos] <= '9' {
						pos++
					}
				}
				nextMustBeSep = true
				numTok := Token{Line: line, Col: startCol, Start: start, End: pos - 1, Kind: Number, Value: inp[start:pos]}
				if inp[firstDigitI] == '0' && firstDigitI+1 < len(inp) && inp[firstDigitI+1] >= '0' && inp[firstDigitI+1] <= '9' {
					numTok.Kind = ErrorLeadingZerosNotPermitted
					numTok.ErrorMsg = "Leading zeros not permitted in numbers"
				}
				if !yield(numTok) {
					return
				}
			case '"':
				start := pos
				startCol := pos - lineStart
				pos++
				var val []byte
				canUseInpSlice := true
				for {
					if pos >= len(inp) {
						yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF in string")
						return
					}
					switch inp[pos] {
					case '"':
						if canUseInpSlice {
							canUseInpSlice = false
							val = inp[start+1 : pos]
						}
						if !yield(Token{Line: line, Col: startCol, Start: start, End: pos, Kind: String, Value: val}) {
							return
						}
						pos++
						continue parseloop
					case '\\':
						if canUseInpSlice {
							canUseInpSlice = false
							val = append(val, inp[start+1:pos]...)
						}
						pos++
						if pos >= len(inp) {
							yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF in string")
							return
						}
						switch inp[pos] {
						case '"', '\\', '/':
							val = append(val, inp[pos])
							pos++
						case 'b':
							val = append(val, '\b')
							pos++
						case 'f':
							val = append(val, '\f')
							pos++
						case 'n':
							val = append(val, '\n')
							pos++
						case 'r':
							val = append(val, '\r')
							pos++
						case 't':
							val = append(val, '\t')
							pos++
						case 'u':
							if pos+4 >= len(inp) {
								yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF")
								return
							}
							d1 := hexVal(inp[pos+1])
							d2 := hexVal(inp[pos+2])
							d3 := hexVal(inp[pos+3])
							d4 := hexVal(inp[pos+4])
							if d1 == -1 || d2 == -1 || d3 == -1 || d4 == -1 {
								if !yieldErr(ErrorBadUnicodeEscape, line, pos-lineStart, "Bad '\\uXXXX' escape in string") {
									return
								}
								d1, d2, d3, d4 = 0, 0, 0, 0 // error recovery
							}
							runeVal := d1*16*16*16 + d2*16*16 + d3*16 + d4

							if utf16.IsSurrogate(rune(runeVal)) && pos+10 < len(inp) && inp[pos+5] == '\\' && inp[pos+6] == 'u' {
								d21 := hexVal(inp[pos+7])
								d22 := hexVal(inp[pos+8])
								d23 := hexVal(inp[pos+9])
								d24 := hexVal(inp[pos+10])
								if d21 == -1 || d22 == -1 || d23 == -1 || d24 == -1 {
									if !yieldErr(ErrorBadUnicodeEscape, line, pos+7-lineStart, "Bad '\\uXXXX' escape in string") {
										return
									}
									d21, d22, d23, d24 = 0, 0, 0, 0 // error recovery
								}
								rune2Val := d21*16*16*16 + d22*16*16 + d23*16 + d24
								if utf16.IsSurrogate(rune(rune2Val)) {
									runes := utf16.Decode([]uint16{uint16(runeVal), uint16(rune2Val)})
									for _, r := range runes {
										val = utf8.AppendRune(val, r)
									}
									pos += 6
								} else {
									// append the first one; leave the second for the next loop
									// iteration
									val = utf8.AppendRune(val, rune(runeVal))
								}
							} else {
								val = utf8.AppendRune(val, rune(runeVal))
							}
							pos += 5
						default:
							if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, "Unexpected character after '\\' in string") {
								return
							}
							val = append(val, inp[pos])
							pos++
						}
					default:
						r, sz := utf8.DecodeRune(inp[pos:])
						// DEL is permitted according to
						// https://datatracker.ietf.org/doc/html/rfc7159
						if unicode.IsControl(r) && r != 0x7F {
							yieldErr(ErrorIllegalControlCharInsideString, line, pos-lineStart, "Illegal control char inside string")
							return
						}
						if r == utf8.RuneError {
							if sz == 0 {
								yieldErr(ErrorUnexpectedEOF, line, pos-lineStart, "Unexpected EOF inside string")
								return
							} else {
								if !yieldErr(ErrorUTF8DecodingErrorInsideString, line, pos-lineStart, "UTF-8 decoding error inside string") {
									return
								}
							}
						}
						if !canUseInpSlice {
							val = append(val, inp[pos:pos+sz]...)
						}
						pos += max(sz, 1) // may be 0 if there was a decoding error
					}
				}
			default:
				r, _ := utf8.DecodeRune(inp[pos:])
				if !yieldErr(ErrorUnexpectedCharacter, line, pos-lineStart, fmt.Sprintf("Unexpected char '%v'", r)) {
					return
				}
				pos++
			}
		}
	}
}

func hexVal(d byte) int {
	if d >= '0' && d <= '9' {
		return int(d) - '0'
	}
	if d >= 'a' && d <= 'f' {
		return int(d) - 'a' + 10
	}
	if d >= 'A' && d <= 'F' {
		return int(d) - 'A' + 10
	}
	return -1
}
