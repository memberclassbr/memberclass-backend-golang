package member_import

// documentVariants expands a user-provided document string into the
// variants we need to match against `UsersOnTenants.document`.
//
// Rules (mirrored from the Next.js action):
//   - Raw value (as typed) is always the first variant.
//   - Strip every non-digit to produce `digits`.
//   - If `digits` has exactly 11 characters, also produce a CPF-formatted
//     variant `XXX.XXX.XXX-XX`.
//
// Returned slice is de-duplicated and preserves order.
func documentVariants(raw string) []string {
	if raw == "" {
		return nil
	}
	digits := stripNonDigits(raw)

	out := []string{raw}
	appendUnique := func(v string) {
		if v == "" {
			return
		}
		for _, existing := range out {
			if existing == v {
				return
			}
		}
		out = append(out, v)
	}
	appendUnique(digits)

	if len(digits) == 11 {
		formatted := digits[0:3] + "." + digits[3:6] + "." + digits[6:9] + "-" + digits[9:11]
		appendUnique(formatted)
	}

	return out
}

func stripNonDigits(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			b = append(b, c)
		}
	}
	return string(b)
}
