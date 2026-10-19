package loch_queue

import (
	"encoding/json"
	"net/http"
	"time"
)

func (qm *QueueManager) rateLimitCheck(rw http.ResponseWriter, req *http.Request) bool {
	cookie, err := req.Cookie(qm.config.CookieName)
	if err != nil {
		qm.logf("debug", "Cannot get cookie from %s", qm.config.CookieName)
	}
	if cookie != nil {
		return false
	}

	now := time.Now()
	window := time.Minute
	maxReq := qm.config.RateLimitTimes

	ip := getClientIP(req)
	// Purge old timestamps
	var recent []time.Time
	for _, t := range qm.rateLimit[ip] {
		if now.Sub(t) < window {
			recent = append(recent, t)
		}
	}
	qm.rateLimit[ip] = append(recent, now)

	// Check limits and return response if needed
	qm.logf("debug", "Rate limit check length: %v", len(qm.rateLimit[ip]))
	if len(qm.rateLimit[ip]) > maxReq {
		qm.logf("warn", "Rate-limit reached for IP %s (no cookie)", ip)

		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("X-Loch-sh", "Loch Queue")
		rw.WriteHeader(qm.config.HTTPResponseCode)

		response := ErrorResponse{
			Error:   "capacity overload",
			Handler: "Loch.sh Queue Manager",
		}

		err := json.NewEncoder(rw).Encode(response)
		if err != nil {
			qm.logf("error", "Failed to encode response body: %s", err)
		}

		return true
	}

	return false
}
