// Package magiclink builds the passwordless login links this service hands to
// members, plus the values that back them in the "MagicToken" table.
//
// The frontend owns the other half of the contract. It serves
// `GET /api/auth/magic/<shortCode>`, looks the row up by "shortCode", stamps
// "usedAt" and forwards to /login, which finishes the sign-in. Two consequences
// shape everything here:
//
//   - The URL carries the short code and nothing else. The member's address
//     used to travel in the query string; it no longer does, because a login
//     URL ends up in mail archives, proxy logs and shared screenshots.
//   - A link that points at /login directly no longer authenticates. The
//     frontend requires "usedAt" to be set and only its magic route sets it, so
//     a guessed short code cannot be redeemed by hand.
//
// A password-reset link is the same URL with `?next=reset`, which tells the
// frontend to leave "usedAt" alone — there the reset handler is what claims the
// row. This service mints login links only, so it never emits that parameter.
//
// The functions here are pure. Each slice owns its own INSERT: the row shape is
// small and the two callers disagree about what to do when it fails.
package magiclink

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

const (
	// shortCodeLen is the length of the value that appears in the URL.
	shortCodeLen = 12
	// shortCodeAlphabet drops the characters a member could misread when
	// copying a code out of an email (I, O, 0, 1). Its length must stay a
	// divisor of 256 — see NewShortCode.
	shortCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// rawTokenBytes is the entropy behind the token itself, which never
	// appears in a link — only its digest is stored, and the frontend
	// resolves the row by short code.
	rawTokenBytes = 32
)

// NewShortCode returns the value that goes in the URL.
//
// Twelve characters over a 32-symbol alphabet is 60 bits, comfortably above the
// 48 the frontend contract asks for.
//
// The alphabet's length divides 256, so the modulo below is unbiased — the
// rejection sampling this would otherwise need buys nothing at this size.
func NewShortCode() (string, error) {
	b := make([]byte, shortCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("magiclink: generate short code: %w", err)
	}
	for i, v := range b {
		b[i] = shortCodeAlphabet[v%byte(len(shortCodeAlphabet))]
	}
	return string(b), nil
}

// NewRawToken returns the secret the "MagicToken" row is built from. Callers
// store HashToken of it and, for the legacy "User"."magicToken" column, its
// bcrypt hash.
func NewRawToken() (string, error) {
	b := make([]byte, rawTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("magiclink: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns what goes in "MagicToken"."token".
//
// It is a plain sha256 hex digest rather than a bcrypt hash because the value
// has to match byte-for-byte what the frontend computes for the same token; a
// salted hash would never compare equal. That is safe here in a way it would
// not be for a password: the input is 32 random bytes, so there is nothing to
// guess by brute force.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Link builds the URL a member clicks, on the tenant's own host.
func Link(protocol, domain, shortCode string) string {
	return fmt.Sprintf("%s://%s/api/auth/magic/%s", protocol, domain, url.PathEscape(shortCode))
}

// WithRedirect appends the post-login destination to a link.
//
// The caller is responsible for having validated the path: this value ends up
// in a URL the frontend navigates to right after it establishes a session, so
// anything a browser could resolve to another origin turns the magic link into
// an open redirect.
func WithRedirect(link, redirectPath string) string {
	if redirectPath == "" {
		return link
	}
	return withParam(link, "redirect", redirectPath)
}

// WithReset turns a link into a password-reset link.
//
// `next=reset` tells the frontend to forward to the reset form *without*
// stamping "usedAt". On that path the reset handler is what claims the row, and
// a row the magic route had already consumed is refused — the link would die
// before the member finished typing a password.
func WithReset(link string) string {
	return withParam(link, "next", "reset")
}

func withParam(link, key, value string) string {
	separator := "?"
	if strings.Contains(link, "?") {
		separator = "&"
	}
	return link + separator + key + "=" + url.QueryEscape(value)
}

// Protocol picks the scheme for a link on the given host. Only local
// development is served over http; the check anchors on the whole "localhost"
// label so a domain like "localhostfoo.com" does not slide in.
func Protocol(domain string) string {
	host := domain
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "http"
	}
	return "https"
}
