package main

import "strings"

func upper(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// codeList joins codes for a compact log line.
func codeList(codes []Code) string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = c.Code
	}
	return strings.Join(out, ", ")
}
