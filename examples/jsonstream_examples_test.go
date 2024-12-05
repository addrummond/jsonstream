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

func TestParseObjectWithStringValues(t *testing.T) {
	obj, err := parseObjectWithStringValues([]byte(`{"a":"bbb","cc":"dd","ee":"ff","":"gggg"}`))
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

func TestFindByPath(t *testing.T) {
	t.Run("simple case where equality holds", func(t *testing.T) {
		s, err := findByPath([]byte(`{"a": {"b": {"c": 1}}}`), []any{"a", "b", "c"})
		if err != nil {
			t.Fatalf("Error: %v", err)
		}
		if s != "1" {
			t.Errorf(`Expected "1", got %v`, s)
		}
	})

	t.Run("simple case where equality does not hold", func(t *testing.T) {
		_, err := findByPath([]byte(`{"a": {"bbbb": {"c": 1}}}`), []any{"a", "b", "c"})
		if err.Error() != "path not found" {
			t.Errorf(`Expected "path not found", got %v`, err.Error())
		}
	})
}

var intIs32Bit = (^uint(0))>>32 == 0
