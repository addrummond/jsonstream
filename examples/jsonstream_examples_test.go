package examples

import (
	"errors"
	"reflect"
	"testing"

	"github.com/addrummond/jsonstream"
)

func parseIntArray(input []byte) ([]int, error) {
	state := 0
	ints := make([]int, 0)
	var p jsonstream.Parser
	for t := range p.Tokenize(input) {
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

func parseDictionaryWithStringValues(input []byte) (map[string]string, error) {
	state := 0
	var p jsonstream.Parser
	dict := make(map[string]string)
	for t := range p.Tokenize(input) {
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

func TestParseIntArray(t *testing.T) {
	var is []int64
	var err error
	var expected []int64
	if intIs32Bit {
		var is32 []int
		is32, err = parseIntArray([]byte("[ 1 , 22 , 65, 55, -0, 0, 1.0, 1.5e3, 111e4, 11, -765, -2147483648, 2147483647 ]"))
		for _, i := range is32 {
			is = append(is, int64(i))
		}
		expected = []int64{1, 22, 65, 55, -0, 0, 1.0, 1.5e3, 111e4, 11, -765, -2147483648, 2147483647}
	} else { // assume 64 bit
		var is64 []int
		is64, err = parseIntArray([]byte("[ 1 , 22 , 65, 55, -0, 0, 1.0, 1.5e3, 111e4, 11, -765, -9223372036854775808, 9223372036854775807 ]"))
		for _, i := range is64 {
			is = append(is, int64(i))
		}
		// make this an array of int64 and not int so that we won't get compiler
		// overflow errors/warnings on 32-bit systems
		expected = []int64{1, 22, 65, 55, -0, 0, 1.0, 1.5e3, 111e4, 11, -765, -9223372036854775808, 9223372036854775807}
	}
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	if !reflect.DeepEqual(is, expected) {
		t.Errorf("Expected %v, got %v", expected, is)
	}
}

func TestParseDictionaryWithStringValues(t *testing.T) {
	obj, err := parseDictionaryWithStringValues([]byte(`{"a":"bbb","cc":"dd","ee":"ff","":"gggg"}`))
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	expected := map[string]string{
		"a":  "bbb",
		"cc": "dd",
		"ee": "ff",
		"":   "gggg",
	}
	if !reflect.DeepEqual(obj, expected) {
		t.Errorf("Expected %v, got %v", expected, obj)
	}
}

var intIs32Bit = (^uint(0))>>32 == 0
