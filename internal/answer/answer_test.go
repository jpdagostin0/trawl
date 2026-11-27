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
