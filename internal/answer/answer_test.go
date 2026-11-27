package answer

import "testing"

func TestCalc(t *testing.T) {
	for q, want := range map[string]string{
		"5 + 5":                     "5 + 5 = 10",
		"5+5":                       "5 + 5 = 10",
		"what is 12*12?":            "12 * 12 = 144",
		"2^10":                      "2 ^ 10 = 1024",
		"sqrt(2)":                   "sqrt(2) = 1.41421356237",
		"7 % 3":                     "7 % 3 = 1",
		"1/3":                       "1 / 3 = 0.333333333333",
		"5 x 6":                     "5 * 6 = 30",
		"99999999999 * 99999999999": "99999999999 * 99999999999 = 9.9999999998 x 10^21",
		"2 * pi":                    "2 * pi = 6.28318530718",
	} {
		a := For(q)
		if a == nil || a.Kind != "calc" || a.Expr+" = "+a.Result != want {
			t.Errorf("For(%q) = %+v, want %q", q, a, want)
		}
	}
	for _, q := range []string{"2024", "pi", "golang generics", "2024-11-27", "covid-19", `"a" + "b"`, "1..1000000", "1 == 1", "len([1,2])", "1/0"} {
		if a := For(q); a != nil {
			t.Errorf("For(%q) = %+v, want nil", q, a)
		}
	}
}

func TestConvert(t *testing.T) {
	for q, want := range map[string]string{
		"10 km in miles":          "10 kilometers = 6.21371192 miles",
		"32 f to c":               "32 fahrenheit = 0 celsius",
		"how many feet in a mile": "1 mile = 5280 feet",
		"1 kg to lbs":             "1 kilogram = 2.20462262 pounds",
		"1 gb in mb":              "1 gigabyte = 1000 megabytes",
		"1 gallon in liters":      "1 fluid gallon = 3.78541178 liters",
	} {
		a := For(q)
		if a == nil || a.Kind != "convert" || a.Expr+" = "+a.Result != want {
			t.Errorf("For(%q) = %+v, want %q", q, a, want)
		}
	}
	if a := For("python in rust"); a != nil {
		t.Errorf("got %+v", a)
	}
}

func TestIntegral(t *testing.T) {
	for q, want := range map[string]string{
		"integrate x over 1, 5":                  "∫ x dx from 1 to 5 = 12",
		"integrate x from 1 to 5":                "∫ x dx from 1 to 5 = 12",
		"integral of x^2 dx from 0 to 3":         "∫ x ^ 2 dx from 0 to 3 = 9",
		"∫ 3x^2 from 0 to 2":                     "∫ 3 * x ^ 2 dx from 0 to 2 = 8",
		"integrate sin(t) dt over [0, pi]":       "∫ sin(t) dt from 0 to pi = 2",
		"integral of 1/x between 1 and e":        "∫ 1 / x dx from 1 to e = 1",
		"integrate exp(-x^2) from -inf to inf":   "∫ exp(-x ^ 2) dx from -inf to inf = 1.77245385091",
		"integrate sqrt(x) from 0 to 1":          "∫ sqrt(x) dx from 0 to 1 = 0.6666667",
		"integrate x over 1 to 5":                "∫ x dx from 1 to 5 = 12",
		"integrate x from 5 to 1":                "∫ x dx from 5 to 1 = -12",
		"what is the integral of 2 from 0 to 10": "∫ 2 dx from 0 to 10 = 20",
	} {
		a := For(q)
		if a == nil || a.Kind != "integral" || a.Expr+" = "+a.Result != want {
			t.Errorf("For(%q) = %+v, want %q", q, a, want)
		}
	}
	for _, q := range []string{"integrate x", "intel from 2 to 5", "integral of cost from 1 to 5", "integrate x*y from 0 to 1", "integrate 1/x from 0 to 1", "international flights from boston to nyc"} {
		if a := For(q); a != nil {
			t.Errorf("For(%q) = %+v, want nil", q, a)
		}
	}
}
