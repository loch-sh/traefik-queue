package loch_queue

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"
)

// getClientID generates or retrieves a unique client identifier using cookies or IP/UserAgent hash.
// Returns (clientID, isTrulyNewCookie).
func (qm *QueueManager) getClientID(rw http.ResponseWriter, req *http.Request) (string, bool) {
	if qm.config.UseCookies {
		cookie, err := req.Cookie(qm.config.CookieName)
		if err == nil && cookie.Value != "" {
			// Cookie exists, return its value.
			// Optionally, refresh cookie's MaxAge if desired, but not strictly necessary for ID retrieval.
			// http.SetCookie(rw, &http.Cookie{... Same params but updated MaxAge ...})
			return cookie.Value, false // Not a new cookie
		}

		// No valid cookie found, create a new one.
		randomPart := generateSecureRandomID(secureIDLengthBytes)
		// Optionally, add a short, non-sensitive hash part for easier server-side log correlation if IPs change.
		// clientHashPart := generateClientHash(req)[:8]
		// newID := fmt.Sprintf("%s-%s", randomPart, clientHashPart)
		newID := randomPart

		http.SetCookie(rw, &http.Cookie{
			Name:     qm.config.CookieName,
			Value:    newID,
			Path:     "/", // Cookie accessible for all paths
			MaxAge:   qm.config.CookieMaxAgeSeconds,
			HttpOnly: true,           // Prevent client-side script access
			Secure:   req.TLS != nil, // Set Secure flag only if connection is HTTPS
			SameSite: http.SameSiteLaxMode,
		})
		qm.logf("debug", "New cookie issued for client. ID: %s (Secure: %t)", newID, req.TLS != nil)
		return newID, true // This is a new cookie
	}

	// Not using cookies, use IP + UserAgent hash.
	hashID := generateClientHash(req)
	qm.logf("debug", "Using IP+UserAgent hash for client. Hash: %s", hashID)
	// isTrulyNewCookie is false as hashes are deterministic for the same IP/UA.
	return hashID, false
}

// generateSecureRandomID creates a cryptographically secure random hex string.
func generateSecureRandomID(lengthBytes int) string {
	bytes := make([]byte, lengthBytes)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a less secure method if crypto/rand fails (should be extremely rare).
		log.Printf(logPrefix+"ERROR: crypto/rand.Read failed: %v. Using timestamp-based fallback for ID generation. THIS IS NOT SECURE.", err)
		// This fallback is NOT cryptographically secure. Production systems must have a working crypto/rand.
		timestamp := time.Now().UnixNano()
		fallbackStr := fmt.Sprintf("%x", timestamp) // Hex representation of timestamp
		// Ensure it has some length, pad if necessary
		for i := 0; i < lengthBytes && i < len(fallbackStr); i++ {
			bytes[i] = fallbackStr[i]
		}
		// If still too short, fill with pseudo-random based on timestamp
		for i := len(fallbackStr); i < lengthBytes; i++ {
			bytes[i] = byte((timestamp + int64(i)*101) % 256) // 101 is a prime
		}
	}
	return hex.EncodeToString(bytes)
}

// generateClientHash creates a SHA256 hash from client IP and User-Agent.
func generateClientHash(req *http.Request) string {
	clientIP := getClientIP(req)
	userAgent := req.UserAgent() // Can be empty

	hasher := sha256.New()
	hasher.Write([]byte(clientIP))
	hasher.Write([]byte("|")) // Separator
	hasher.Write([]byte(userAgent))
	return hex.EncodeToString(hasher.Sum(nil))
}
