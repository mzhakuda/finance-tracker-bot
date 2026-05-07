package events

import (
	"strings"
	"testing"
)

func TestParseAmount_Valid(t *testing.T) {
	cases := []struct {
		in           string
		def          string
		wantValue    float64
		wantCurrency string
	}{
		{"1", "USD", 1, "USD"},
		{"  42 ", "USD", 42, "USD"},
		{"3.14", "USD", 3.14, "USD"},
		{"3,14", "USD", 3.14, "USD"},
		{"1000.50", "EUR", 1000.50, "EUR"},
		{"0.01", "USD", 0.01, "USD"},
		{"12.50 EUR", "USD", 12.50, "EUR"},
		{"12.50EUR", "USD", 12.50, "EUR"},
		{"12.50 eur", "USD", 12.50, "EUR"},
		{"12,5 RUB", "USD", 12.5, "RUB"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			value, currency, err := parseAmount(tc.in, tc.def)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tc.wantValue || currency != tc.wantCurrency {
				t.Fatalf("got %v %s, want %v %s", value, currency, tc.wantValue, tc.wantCurrency)
			}
		})
	}
}

func TestParseAmount_Invalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
		def  string
		msg  string
	}{
		{"empty", "", "USD", "empty"},
		{"whitespace", "   ", "USD", "empty"},
		{"non-numeric", "abc", "USD", "not a number"},
		{"negative", "-5", "USD", "positive"},
		{"zero", "0", "USD", "positive"},
		{"missing-currency-default-empty", "12", "", "currency is empty"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseAmount(tc.in, tc.def)
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
	id, err := extractCategoryID(map[string]interface{}{dataKeyCategorySelected: "category_42"})
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
		{"wrong prefix", map[string]interface{}{dataKeyCategorySelected: "foo_42"}},
		{"no id", map[string]interface{}{dataKeyCategorySelected: "category_"}},
		{"non-numeric id", map[string]interface{}{dataKeyCategorySelected: "category_abc"}},
		{"negative id", map[string]interface{}{dataKeyCategorySelected: "category_-1"}},
		{"zero id", map[string]interface{}{dataKeyCategorySelected: "category_0"}},
		{"no separator", map[string]interface{}{dataKeyCategorySelected: "category"}},
		{"wrong type", map[string]interface{}{dataKeyCategorySelected: 42}},
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

func TestValidateCategoryName(t *testing.T) {
	reserved := func(s string) bool { return s == "Add spending" }

	if _, ok := validateCategoryName("Food", reserved); !ok {
		t.Fatalf("expected Food to be valid")
	}
	if _, ok := validateCategoryName("  Food  ", reserved); !ok {
		t.Fatalf("expected trimmed Food to be valid")
	}
	for _, in := range []string{"", "   ", "/list", "Add spending", strings.Repeat("a", 33)} {
		if _, ok := validateCategoryName(in, reserved); ok {
			t.Fatalf("expected %q to be invalid", in)
		}
	}
}

func TestValidateEmoji(t *testing.T) {
	if _, ok := validateEmoji("🍔"); !ok {
		t.Fatalf("expected emoji to be valid")
	}
	for _, in := range []string{"", "  ", "way-too-long-emoji-string"} {
		if _, ok := validateEmoji(in); ok {
			t.Fatalf("expected %q to be invalid", in)
		}
	}
}

func TestReadAmountFromState_FallsBackToRaw(t *testing.T) {
	v, c, err := readAmountFromState(map[string]interface{}{dataKeyAmountEntered: "10 EUR"}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 10 || c != "EUR" {
		t.Fatalf("got %v %s", v, c)
	}
}

func TestReadAmountFromState_PrefersValidated(t *testing.T) {
	v, c, err := readAmountFromState(map[string]interface{}{
		dataKeyAmountValue:    99.5,
		dataKeyAmountCurrency: "GBP",
		dataKeyAmountEntered:  "ignored",
	}, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 99.5 || c != "GBP" {
		t.Fatalf("got %v %s", v, c)
	}
}
