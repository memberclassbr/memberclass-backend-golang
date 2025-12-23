package utils

import (
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	counter      int64
	counterMutex sync.Mutex
	fingerprint  string
)

func init() {
	// Generate fingerprint from hostname and process ID
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	fingerprint = fmt.Sprintf("%s%d", hostname, pid)
	if len(fingerprint) > 2 {
		fingerprint = fingerprint[:2]
	}
}

// GenerateCUID generates a CUID (Collision-resistant Unique Identifier)
// Format: c + timestamp + counter + fingerprint + random
// Example: clwhvhcaz0001j1j81rcqvzki (25 chars)
func GenerateCUID() string {
	counterMutex.Lock()
	counter++
	currentCounter := counter
	counterMutex.Unlock()

	// Get timestamp in milliseconds
	timestamp := time.Now().UnixMilli()

	// Convert timestamp to base36 (should be ~10 chars)
	timestampBase36 := base36Encode(uint64(timestamp))

	// Convert counter to base36 (4 chars)
	counterBase36 := base36Encode(uint64(currentCounter))
	if len(counterBase36) < 4 {
		counterBase36 = fmt.Sprintf("%04s", counterBase36)
	}
	if len(counterBase36) > 4 {
		counterBase36 = counterBase36[len(counterBase36)-4:]
	}

	// Ensure fingerprint is 2 chars
	fp := fingerprint
	if len(fp) < 2 {
		fp = fp + "0"
	}
	if len(fp) > 2 {
		fp = fp[:2]
	}

	// Calculate remaining length for random part
	// Total: 25 chars = 1 (c) + timestamp + 4 (counter) + 2 (fingerprint) + random
	usedLength := 1 + len(timestampBase36) + 4 + 2
	randomLength := 25 - usedLength
	if randomLength < 0 {
		randomLength = 0
	}

	// Generate random part
	randomPart := generateRandomString(randomLength)

	// Combine: c + timestamp + counter + fingerprint + random
	cuid := fmt.Sprintf("c%s%s%s%s", timestampBase36, counterBase36, fp, randomPart)

	// Ensure it's exactly 25 characters
	if len(cuid) > 25 {
		cuid = cuid[:25]
	} else if len(cuid) < 25 {
		// Pad with random if needed
		pad := generateRandomString(25 - len(cuid))
		cuid = cuid + pad
	}

	return cuid
}

// base36Encode converts a number to base36 (0-9, a-z)
func base36Encode(n uint64) string {
	if n == 0 {
		return "0"
	}

	var result string
	base := uint64(36)
	chars := "0123456789abcdefghijklmnopqrstuvwxyz"

	for n > 0 {
		result = string(chars[n%base]) + result
		n /= base
	}

	return result
}

// generateRandomString generates a random alphanumeric string
func generateRandomString(length int) string {
	chars := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := make([]byte, length)
	rand.Read(bytes)
	for i := range bytes {
		bytes[i] = chars[bytes[i]%byte(len(chars))]
	}
	return string(bytes)
}

