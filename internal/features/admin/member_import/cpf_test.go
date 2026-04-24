package member_import

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocumentVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"non-cpf raw = single", "ABC-999", []string{"ABC-999", "999"}},
		{"11 digits → 3 variants", "03092846001", []string{"03092846001", "030.928.460-01"}},
		{"formatted CPF → 3 variants", "030.928.460-01", []string{"030.928.460-01", "03092846001"}},
		{"CPF with extra spaces", " 030.928.460-01 ", []string{" 030.928.460-01 ", "03092846001", "030.928.460-01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := documentVariants(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStripNonDigits(t *testing.T) {
	assert.Equal(t, "03092846001", stripNonDigits("030.928.460-01"))
	assert.Equal(t, "", stripNonDigits("abc"))
	assert.Equal(t, "12345", stripNonDigits("1a2b3c4d5"))
}
