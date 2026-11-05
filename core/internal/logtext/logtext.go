// Package logtext keeps secrets and huge values out of the logs.
package logtext

import (
	"net/url"
	"regexp"
	"strings"
)

const maxLen = 500

var urlPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// Clean makes text safe to log: stream URLs carry server tokens, so each URL keeps only its
// scheme and host.
func Clean(s string) string {
	s = urlPattern.ReplaceAllStringFunc(strings.TrimSpace(s), func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "<url>"
		}
		return u.Scheme + "://" + u.Hostname() + "/…"
	})
	if len(s) > maxLen {
		s = strings.ToValidUTF8(s[:maxLen], "") + "…"
	}
	return s
}
