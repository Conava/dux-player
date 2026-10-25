package library

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestTestFailureCodes(t *testing.T) {
	_, badAddr := source.ServerURL("ftp://x")
	_, creds := source.ServerURL("http://u:p@x")
	tests := []struct {
		err  error
		code string
	}{
		{fmt.Errorf("ping: %w", source.ErrSignInRefused), CodeSignInRefused},
		{fmt.Errorf("%w (%w)", errors.New("server error 41"), source.ErrPasswordNeedsHTTPS), CodePasswordNeedsHTTPS},
		{badAddr, CodeBadAddress},
		{creds, CodeCredentialsInAddress},
		{&url.Error{Op: "Get", URL: "x", Err: source.ErrRedirected}, CodeRedirected},
		{&url.Error{Op: "Get", URL: "x", Err: &tls.CertificateVerificationError{Err: errors.New("unknown authority")}}, CodeCertificateInvalid},
		{&url.Error{Op: "Get", URL: "x", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, CodeServerUnreachable},
		{&url.Error{Op: "Get", URL: "x", Err: &net.DNSError{Err: "no such host", Name: "x"}}, CodeServerUnreachable},
		{errors.New("ping: not an (Open)Subsonic response"), CodeTestFailed},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			r := testFailure(tt.err)
			assert.Equal(t, tt.code, r.Code, tt.err.Error())
			assert.NotEmpty(t, r.Message)
		})
	}
}
