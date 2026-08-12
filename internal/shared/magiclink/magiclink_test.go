package magiclink

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewShortCode(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		code, err := NewShortCode()
		require.NoError(t, err)
		require.Len(t, code, shortCodeLen)

		// The confusable characters are excluded on purpose: a member may be
		// reading the code off a printed email.
		for _, c := range code {
			require.Contains(t, shortCodeAlphabet, string(c), "unexpected character %q in %q", c, code)
		}

		assert.False(t, seen[code], "short code repeated within 1000 draws: %s", code)
		seen[code] = true
	}
}

// NewShortCode maps a random byte onto the alphabet with a modulo, which is
// only unbiased while the alphabet's length divides 256. Shortening or
// extending the alphabet without noticing would skew the codes toward its first
// characters and quietly cost entropy on a unique-indexed column.
func TestShortCodeAlphabetKeepsTheModuloUnbiased(t *testing.T) {
	require.Equal(t, 32, len(shortCodeAlphabet))
	assert.Zero(t, 256%len(shortCodeAlphabet))
}

// The short code lands in a path segment, so it must not need escaping — an
// escaped code would not match the row the frontend looks up.
func TestNewShortCodeIsPathSafe(t *testing.T) {
	code, err := NewShortCode()
	require.NoError(t, err)
	assert.Equal(t, code, url.PathEscape(code))
}

func TestNewRawToken(t *testing.T) {
	a, err := NewRawToken()
	require.NoError(t, err)
	b, err := NewRawToken()
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
	// 32 bytes in unpadded base64url.
	assert.Len(t, a, 43)
	assert.NotContains(t, a, "=")
}

// The digest has to match what the frontend computes for the same token, so
// this pins the algorithm rather than just asserting "not the plaintext".
func TestHashTokenIsSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("token-abc"))
	assert.Equal(t, hex.EncodeToString(sum[:]), HashToken("token-abc"))

	got := HashToken("token-abc")
	assert.Len(t, got, 64)
	assert.NotContains(t, got, "token-abc")
	assert.NotEqual(t, got, HashToken("token-abd"))
}

func TestLink(t *testing.T) {
	assert.Equal(t,
		"https://escola.com.br/api/auth/magic/ABCD2345EFGH",
		Link("https", "escola.com.br", "ABCD2345EFGH"),
	)
}

// The whole point of the short-code format: nothing identifying travels in the
// URL.
func TestLinkCarriesNeitherTokenNorEmail(t *testing.T) {
	link := Link("https", "escola.com.br", "ABCD2345EFGH")
	assert.NotContains(t, link, "@")
	assert.NotContains(t, link, "token")
	assert.NotContains(t, link, "?")
}

func TestWithRedirect(t *testing.T) {
	t.Run("adds the first query parameter", func(t *testing.T) {
		got := WithRedirect("https://escola.com.br/api/auth/magic/ABC", "/curso/1")
		assert.Equal(t, "https://escola.com.br/api/auth/magic/ABC?redirect=%2Fcurso%2F1", got)
	})

	// Links minted before the short-code format already carry a query string;
	// they can still be sitting in the cache when this deploys.
	t.Run("appends to an existing query string", func(t *testing.T) {
		got := WithRedirect("https://escola.com.br/login?token=abc", "/curso/1")
		assert.Equal(t, "https://escola.com.br/login?token=abc&redirect=%2Fcurso%2F1", got)
	})

	t.Run("no destination leaves the link untouched", func(t *testing.T) {
		link := "https://escola.com.br/api/auth/magic/ABC"
		assert.Equal(t, link, WithRedirect(link, ""))
	})

	t.Run("escapes the destination", func(t *testing.T) {
		got := WithRedirect("https://escola.com.br/api/auth/magic/ABC", "/busca?q=vendas&page=2")
		parsed, err := url.Parse(got)
		require.NoError(t, err)
		assert.Equal(t, "/busca?q=vendas&page=2", parsed.Query().Get("redirect"))
	})
}

func TestWithReset(t *testing.T) {
	assert.Equal(t,
		"https://escola.com.br/api/auth/magic/ABC?next=reset",
		WithReset("https://escola.com.br/api/auth/magic/ABC"),
	)

	// Order does not matter to the frontend, but both parameters have to
	// survive: one picks the reset form, the other says where to land after.
	both := WithRedirect(WithReset("https://escola.com.br/api/auth/magic/ABC"), "/conta")
	parsed, err := url.Parse(both)
	require.NoError(t, err)
	assert.Equal(t, "reset", parsed.Query().Get("next"))
	assert.Equal(t, "/conta", parsed.Query().Get("redirect"))
}

func TestProtocol(t *testing.T) {
	cases := map[string]string{
		"localhost":                "http",
		"localhost:3000":           "http",
		"dev.localhost:8181":       "http",
		"acme.memberclass.com.br":  "https",
		"escola.com.br":            "https",
		"localhostfoo.com":         "https",
		"notlocalhost.example.com": "https",
	}
	for domain, want := range cases {
		t.Run(domain, func(t *testing.T) {
			assert.Equal(t, want, Protocol(domain))
		})
	}
}
