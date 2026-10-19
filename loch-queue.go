// Package loch_queue package loch_queue implements a Traefik middleware plugin that manages
// access to services by implementing a queue when capacity is reached.
// It functions similar to a virtual waiting room, allowing a controlled number
// of users to access the service while placing others in a structured queue.
package loch_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// New creates a new queue manager middleware instance.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid QueueManager configuration: %w", err)
	}

	// Setup logger
	logOutput, logFileHandle := setupLogOutput(config)
	logger := log.New(logOutput, "", log.LstdFlags|log.Lmicroseconds) // Added microseconds for finer log timing

	qm := &QueueManager{
		next:                 next,
		name:                 name,
		config:               config,
		logger:               logger,
		queue:                make([]Session, 0),
		activeSessionIDs:     make(map[string]bool),
		stopCleanup:          make(chan bool),
		inactivityTimeoutDur: time.Duration(config.InactivityTimeoutSeconds) * time.Second,
		cleanupIntervalDur:   time.Duration(config.CleanupIntervalSeconds) * time.Second,
		logFileHandle:        logFileHandle, // Store the file handle
		rateLimit:            make(map[string][]time.Time),
	}

	if config.HardSessionLimitSeconds > 0 {
		qm.hardSessionLimitDur = time.Duration(config.HardSessionLimitSeconds) * time.Second
	}

	// Initialize cache
	// The cache's internal cleanup can be more frequent than the plugin's main cleanup logic.
	cacheInternalCleanupInterval := qm.inactivityTimeoutDur / 2
	if cacheInternalCleanupInterval < 5*time.Second { // Minimum 5s for cache cleanup
		cacheInternalCleanupInterval = 5 * time.Second
	}
	qm.cache = NewSimpleCache(qm.inactivityTimeoutDur, cacheInternalCleanupInterval)

	// Attempt to load template during initialization
	if err := qm.loadTemplate(); err != nil {
		qm.logf("warn", "Could not load queue page template '%s' during init: %v. Will attempt on first request. Ensure file is accessible.", config.QueuePageFile, err)
	}

	// Load translations
	languageFile, readErr := os.ReadFile(qm.config.QueueTranslationsFile)
	if readErr != nil {
		qm.logf("warn", "Failed to read translated messages: %v", readErr)
	}

	var allTranslations map[string]TranslatedMessages
	if err := json.Unmarshal(languageFile, &allTranslations); err != nil {
		qm.logf("warn", "Failed to parse translated messages JSON: %v", err)
	}

	qm.translatedMessages = allTranslations

	// Start periodic cleanup goroutine
	qm.startCleanupRoutine()

	qm.logf("info", "QueueManager plugin '%s' initialized. MaxEntries: %d, InactivityTimeout: %v, HardSessionLimit: %v, CleanupInterval: %v",
		name, config.MaxEntries, qm.inactivityTimeoutDur, qm.hardSessionLimitDur, qm.cleanupIntervalDur)
	if config.Debug {
		qm.logf("debug", "Full configuration: %+v", config)
	}

	return qm, nil
}

// ServeHTTP is the main entry point for incoming HTTP requests.
func (qm *QueueManager) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !qm.config.Enabled {
		qm.next.ServeHTTP(rw, req)
		return
	}

	clientID, _ := qm.getClientID(rw, req)
	qm.logf("debug", "Request from ClientID: %s, Path: %s", clientID, req.URL.Path)

	// Check if not a replay without cookie
	if qm.rateLimitCheck(rw, req) {
		return
	}

	// Attempt to let the client proceed if conditions are met
	if qm.tryProceedClient(clientID, rw, req) {
		qm.logf("debug", "Client %s allowed to proceed to service.", clientID)
		qm.next.ServeHTTP(rw, req)
		return
	}

	// Client cannot proceed, must be queued or is already in queue
	positionInQueue := qm.enqueueOrUpdateClient(clientID)
	qm.logf("debug", "Client %s is being served queue page. Position (0-based): %d", clientID, positionInQueue)

	if req.Header.Get("accept") == "application/json" {
		qm.logf("debug", "Request Accept Header: %s", req.Header.Get("Accept"))
		qm.serveAPIResponse(rw, qm.prepareQueuePageData(positionInQueue))
		return
	}

	qm.serveQueuePage(rw, req, positionInQueue)
}

// tryProceedClient checks if the client can access the service.
// Handles active sessions, hard limits, and capacity checks.
// Returns true if client proceeds, false otherwise.
// This function acquires the main lock (qm.mu).
func (qm *QueueManager) tryProceedClient(clientID string, rw http.ResponseWriter, req *http.Request) bool {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// 1. Check if client is already active
	if qm.activeSessionIDs[clientID] {
		sessionData, foundInCache := qm.getSessionFromCache(clientID)
		if !foundInCache {
			// Was marked active, but its session expired from cache (due to inactivity).
			qm.logf("info", "Client %s was active, but session expired from cache (inactivity). Removing from active list.", clientID)
			delete(qm.activeSessionIDs, clientID)
			// Client will be re-evaluated below for capacity or requeue.
		} else {
			// Active and session found in cache. Check for hard limit.
			if qm.hardSessionLimitDur > 0 && time.Now().After(sessionData.HardExpiryAt) {
				qm.logf("info", "Client %s active session reached hard limit (Expiry: %v). Removing from active list.", clientID, sessionData.HardExpiryAt)
				delete(qm.activeSessionIDs, clientID)
				qm.cache.Delete(clientID) // Explicitly remove from cache as well
				// Client will be re-evaluated below for capacity or requeue.
			} else {
				// Active, in cache, and within hard limit. Update last seen and allow to proceed.
				qm.logf("debug", "Client %s is active and within limits. Updating activity and allowing proceed.", clientID)
				qm.updateClientActivityInCache(clientID, sessionData)
				return true
			}
		}
	}

	// 2. Client is not currently active (or was just removed). Check for capacity.
	if len(qm.activeSessionIDs) < qm.config.MaxEntries {
		qm.logf("info", "Capacity available (%d/%d). Promoting client %s to active.", len(qm.activeSessionIDs), qm.config.MaxEntries, clientID)

		// If client was in the queue, remove them first.
		qm.dequeueClientUnderLock(clientID) // This helper assumes qm.mu is already locked.

		now := time.Now()
		newSession := Session{
			ID:         clientID,
			CreatedAt:  now,
			LastSeenAt: now,
			Position:   -1, // Mark as not in queue
		}
		if qm.hardSessionLimitDur > 0 {
			newSession.HardExpiryAt = now.Add(qm.hardSessionLimitDur)
		}

		qm.activeSessionIDs[clientID] = true
		qm.cache.Set(clientID, newSession, qm.inactivityTimeoutDur) // Add/update cache with inactivity timeout

		return true
	}

	// 3. No capacity, and client is not currently active and allowed.
	qm.logf("debug", "Client %s cannot proceed. No capacity (%d/%d) or session expired.", clientID, len(qm.activeSessionIDs), qm.config.MaxEntries)
	return false
}
