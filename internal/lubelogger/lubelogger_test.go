package lubelogger

import "testing"

func TestParseNumber(t *testing.T) {
	cases := map[string]float64{"42,25": 42.25, "149.99": 149.99, "1.234,56": 1234.56, "1,234.56": 1234.56, "261689": 261689, "6,4221311475409836": 6.4221311475409836, "1.234.567": 1234567, "": 0, "abc": 0, "12 345,5": 12345.5}
	for in, want := range cases {
		if got := ParseNumber(in); got != want {
			t.Errorf("ParseNumber(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	cases := map[string]string{"26. 5. 2022.": "2022-05-26", "2026-09-11": "2026-09-11", "5/26/2022": "2022-05-26", "26/05/2022": "2022-05-26", "13.01.2022": "2022-01-13", "2026-10-03T00:00:00": "2026-10-03", "": ""}
	for in, want := range cases {
		if got := ParseDate(in); got != want {
			t.Errorf("ParseDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCurrency(t *testing.T) {
	code, notes := Currency("[Pocket Homelab action: 460A]\n[Pocket Homelab currency: BAM]")
	if code != "BAM" || notes != "" {
		t.Fatalf("got %q %q", code, notes)
	}
	code, notes = Currency("plain note")
	if code != "" || notes != "plain note" {
		t.Fatalf("got %q %q", code, notes)
	}
}
