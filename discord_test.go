package main

import "testing"

func TestBuildContent(t *testing.T) {
	cases := []struct {
		msg   string
		count int
		want  string
	}{
		{"{count} new {if-singular:code}{if-plural:codes}", 1, "1 new code"},
		{"{count} new {if-singular:code}{if-plural:codes}", 3, "3 new codes"},
		{"<@&123> {count} new {if-singular:code}{if-plural:codes}!", 1, "<@&123> 1 new code!"},
		{"<@&123> {count} new {if-singular:code}{if-plural:codes}!", 2, "<@&123> 2 new codes!"},
		// Irregular plural / whole-word swap, not just an "s" suffix.
		{"{count} {if-singular:child}{if-plural:children}", 1, "1 child"},
		{"{count} {if-singular:child}{if-plural:children}", 4, "4 children"},
		// Non-English form (no plural distinction needed — same both ways).
		{"{count}件の{if-singular:コード}{if-plural:コード}", 2, "2件のコード"},
		{"no placeholders here", 5, "no placeholders here"},
	}
	for _, c := range cases {
		if got := buildContent(c.msg, c.count); got != c.want {
			t.Errorf("buildContent(%q, %d) = %q, want %q", c.msg, c.count, got, c.want)
		}
	}
}
