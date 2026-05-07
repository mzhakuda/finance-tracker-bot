package events

import (
	"strings"
	"testing"
)

func TestParseAmount_Valid(t *testing.T) {
	cases := map[string]float64{
		"1":         1,
		"  42 ":     42,
		"3.14":      3.14,
		"3,14":      3.14,
		"1000.50":   1000.50,
		"0.01":      0.01,
	}
	for input, expected := range cases {
		input := input
		expected := expected
		t.Run(input, func(t *testing.T) {
			got, err := parseAmount(map[string]interface{}{"AmountEntered": input})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != expected {
				t.Fatalf("got %v, want %v", got, expected)
			}
		})
	}
}

func TestParseAmount_Invalid(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
		msg  string
	}{
		{"missing", map[string]interface{}{}, "not provided"},
		{"empty", map[string]interface{}{"AmountEntered": ""}, "empty"},
		{"whitespace", map[string]interface{}{"AmountEntered": "   "}, "empty"},
		{"non-numeric", map[string]interface{}{"AmountEntered": "abc"}, "not a number"},
		{"negative", map[string]interface{}{"AmountEntered": "-5"}, "positive"},
		{"zero", map[string]interface{}{"AmountEntered": "0"}, "positive"},
		{"wrong type", map[string]interface{}{"AmountEntered": 42}, "not provided"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAmount(tc.data)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected error containing %q, got %q", tc.msg, err.Error())
			}
		})
	}
}

func TestExtractCategoryID_Valid(t *testing.T) {
	id, err := extractCategoryID(map[string]interface{}{"CategorySelected": "category_42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("got %d, want 42", id)
	}
}

func TestExtractCategoryID_Invalid(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
	}{
		{"missing", map[string]interface{}{}},
		{"wrong prefix", map[string]interface{}{"CategorySelected": "foo_42"}},
		{"no id", map[string]interface{}{"CategorySelected": "category_"}},
		{"non-numeric id", map[string]interface{}{"CategorySelected": "category_abc"}},
		{"negative id", map[string]interface{}{"CategorySelected": "category_-1"}},
		{"zero id", map[string]interface{}{"CategorySelected": "category_0"}},
		{"no separator", map[string]interface{}{"CategorySelected": "category"}},
		{"wrong type", map[string]interface{}{"CategorySelected": 42}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := extractCategoryID(tc.data); err == nil {
				t.Fatalf("expected error for %v", tc.data)
			}
		})
	}
}

func TestStringField(t *testing.T) {
	got, ok := stringField(map[string]interface{}{"k": "v"}, "k")
	if !ok || got != "v" {
		t.Fatalf("got %q, ok=%v", got, ok)
	}
	if _, ok := stringField(map[string]interface{}{"k": 42}, "k"); ok {
		t.Fatalf("expected ok=false for non-string value")
	}
	if _, ok := stringField(map[string]interface{}{}, "missing"); ok {
		t.Fatalf("expected ok=false for missing key")
	}
	if _, ok := stringField(map[string]interface{}{"k": nil}, "k"); ok {
		t.Fatalf("expected ok=false for nil value")
	}
}

func TestUnmarshalUserData(t *testing.T) {
	got, err := unmarshalUserData("")
	if err != nil || len(got) != 0 {
		t.Fatalf("expected empty map, got %v err=%v", got, err)
	}

	got, err = unmarshalUserData(`{"a":"b"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["a"] != "b" {
		t.Fatalf("got %v, want a=b", got)
	}

	if _, err := unmarshalUserData("not json"); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}
