package api

import "testing"

func TestPlainPageLinks(t *testing.T) {
	cases := map[string]string{
		"see [[p. 42]]":                 "see p. 42",
		"[[42]] and [[Page 7]] [[p12]]": "p. 42 and p. 7 p. 12",
		"[ch. 3](#page=10) end":         "ch. 3 (p. 10) end",
		"[[not a page]] [x](https://e)": "[[not a page]] [x](https://e)",
	}
	for in, want := range cases {
		if got := plainPageLinks(in); got != want {
			t.Errorf("plainPageLinks(%q) = %q, want %q", in, got, want)
		}
	}
}
