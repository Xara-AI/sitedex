package product

import "testing"

func TestParsePriceString(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{"19.99", 19.99, true},
		{"19,99", 19.99, true},
		{"1,234.56", 1234.56, true},
		{"1.234,56", 1234.56, true},
		{"$79.00", 79.00, true},
		{"129,90 lei", 129.90, true},
		{"  42  ", 42, true},
		{"", 0, false},
		{"free", 0, false},
		{"call for price", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parsePriceString(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parsePriceString(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("parsePriceString(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPriceField_NumericAndString(t *testing.T) {
	if v, ok := priceField(19.99); !ok || v != 19.99 {
		t.Errorf("priceField(float64) = %v, %v", v, ok)
	}
	if v, ok := priceField("19.99"); !ok || v != 19.99 {
		t.Errorf("priceField(string) = %v, %v", v, ok)
	}
	if _, ok := priceField(nil); ok {
		t.Error("priceField(nil) should not be ok")
	}
	if _, ok := priceField(true); ok {
		t.Error("priceField(bool) should not be ok")
	}
}

func TestAvailabilityField(t *testing.T) {
	cases := []struct {
		in   string
		want Availability
	}{
		{"https://schema.org/InStock", InStock},
		{"http://schema.org/OutOfStock", OutOfStock},
		{"InStock", InStock},
		{"in stock", InStock},
		{"out of stock", OutOfStock},
		{"sold out", OutOfStock},
		{"oos", OutOfStock},
		{"PreOrder", InStock},
		{"LimitedAvailability", InStock},
		{"Discontinued", OutOfStock},
		{"", Unknown},
		{"SomeWeirdValue", Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := availabilityField(tc.in); got != tc.want {
				t.Errorf("availabilityField(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCurrencyFromSymbol(t *testing.T) {
	cases := map[string]string{
		"$19.99":     "USD",
		"€19.99":     "EUR",
		"£19.99":     "GBP",
		"129,90 lei": "RON",
		"129,90 RON": "RON",
		"no symbol":  "",
	}
	for in, want := range cases {
		if got := currencyFromSymbol(in); got != want {
			t.Errorf("currencyFromSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}
