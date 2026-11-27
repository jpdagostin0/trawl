package answer

import (
	"regexp"
	"strconv"
	"strings"

	units "github.com/bcicen/go-units"
)

var (
	// "10 km in miles", "72 f to c", "5.5 pounds into kg"
	convertRe = regexp.MustCompile(`(?i)^(?:convert\s+)?(-?\d+(?:\.\d+)?(?:e-?\d+)?)\s*([a-z°µ][a-z°µ0-9 ]*?)\s+(?:in|to|into|as)\s+([a-z°µ][a-z°µ0-9 ]*?)\??$`)
	// "how many feet in a mile"
	howManyRe = regexp.MustCompile(`(?i)^how\s+many\s+([a-z°µ][a-z°µ0-9 ]*?)\s+(?:are\s+)?in\s+(?:a\s+|an\s+|one\s+|(\d+(?:\.\d+)?)\s+)?([a-z°µ][a-z°µ0-9 ]*?)\??$`)
)

// unitAliases covers spellings go-units doesn't know.
var unitAliases = map[string]string{
	"f": "fahrenheit", "°f": "fahrenheit", "degf": "fahrenheit", "degrees fahrenheit": "fahrenheit",
	"c": "celsius", "°c": "celsius", "degc": "celsius", "degrees celsius": "celsius", "centigrade": "celsius",
	"k":  "kelvin",
	"lb": "pound", "lbs": "pound",
	"feet": "foot", "ft": "foot",
	"inches": "inch", "in": "inch",
	"miles": "mile", "mi": "mile",
	"oz": "ounce",
	// US customary, not imperial
	"gallon": "fluid gallon", "gallons": "fluid gallon", "gal": "fluid gallon",
	"quart": "fluid quart", "quarts": "fluid quart", "qt": "fluid quart",
	"pint": "fluid pint", "pints": "fluid pint", "pt": "fluid pint",
}

// exactAliases are case-sensitive: lowercase "gb" means bytes, as on Google,
// while "Gb" stays gigabits.
var exactAliases = map[string]string{
	"kb": "kilobyte", "mb": "megabyte", "gb": "gigabyte", "tb": "terabyte", "pb": "petabyte",
}

func findUnit(s string) (units.Unit, bool) {
	s = strings.TrimSpace(s)
	if a, ok := exactAliases[s]; ok {
		s = a
	} else if a, ok := unitAliases[strings.ToLower(s)]; ok {
		s = a
	}
	u, err := units.Find(s)
	return u, err == nil
}

func convert(q string) *Answer {
	var num, from, to string
	if m := convertRe.FindStringSubmatch(q); m != nil {
		num, from, to = m[1], m[2], m[3]
	} else if m := howManyRe.FindStringSubmatch(q); m != nil {
		num, from, to = m[2], m[3], m[1]
		if num == "" {
			num = "1"
		}
	} else {
		return nil
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return nil
	}
	fu, ok1 := findUnit(from)
	tu, ok2 := findUnit(to)
	if !ok1 || !ok2 || fu.Name == tu.Name {
		return nil
	}
	out, err := units.ConvertFloat(v, fu, tu)
	if err != nil {
		return nil
	}
	res := formatNum(roundSig(out.Float()))
	if res == "" {
		return nil
	}
	return &Answer{Kind: "convert", Expr: formatNum(v) + " " + label(fu, v), Result: res + " " + label(tu, out.Float())}
}

func label(u units.Unit, v float64) string {
	if v == 1 {
		return u.Name
	}
	return u.PluralName()
}

// roundSig keeps 9 significant digits, enough for conversions and it hides
// float noise like 0.30000000000000004.
func roundSig(v float64) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 9, 64), 64)
	return f
}
