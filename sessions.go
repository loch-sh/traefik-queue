package loch_queue

import (
	"fmt"
	"log"
	"os"
	"time"
)

// CleanupExpiredSessions handles eviction of expired sessions (inactive or past hard limit)
// and promotes clients from the queue if capacity allows.
// This function acquires the main lock (qm.mu).
func (qm *QueueManager) CleanupExpiredSessions() {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	now := time.Now()
	removedActiveCount := 0
	removedQueueCount := 0
	promotedCount := 0

	// 1. Check active sessions for expiry (inactivity via cache or hard limit)
	// Iterate over a copy of keys to allow modification of qm.activeSessionIDs
	currentActiveClientIDs := make([]string, 0, len(qm.activeSessionIDs))
	for id := range qm.activeSessionIDs {
		currentActiveClientIDs = append(currentActiveClientIDs, id)
	}

	for _, clientID := range currentActiveClientIDs {
		sessionData, foundInCache := qm.getSessionFromCache(clientID)
		shouldRemoveActive := false
		reason := ""

		if !foundInCache {
			// Session not in cache means it expired due to inactivity (cache's default TTL)
			shouldRemoveActive = true
			reason = "inactivity (expired from cache)"
		} else {
			// Session is in cache, check hard limit
			if qm.hardSessionLimitDur > 0 && now.After(sessionData.HardExpiryAt) {
				shouldRemoveActive = true
				reason = fmt.Sprintf("hard session limit (expiry: %v)", sessionData.HardExpiryAt)
				qm.cache.Delete(clientID) // Explicitly remove from cache if hard limit exceeded
			}
		}

		if shouldRemoveActive {
			qm.logf("info", "Removing client %s from active sessions due to %s.", clientID, reason)
			delete(qm.activeSessionIDs, clientID)
			removedActiveCount++
		}
	}

	// 2. Filter queue for inactive sessions (expired from cache)
	newQueue := make([]Session, 0, len(qm.queue))
	for _, sessionInQueue := range qm.queue {
		// Check if the queued session is still valid in the cache (not timed out by inactivity)
		cachedSessionData, foundInCache := qm.getSessionFromCache(sessionInQueue.ID)
		if foundInCache {
			// Update LastSeenAt to now for active check during this cleanup, and re-set in cache to refresh its timer
			cachedSessionData.LastSeenAt = now
			cachedSessionData.Position = len(newQueue) // Update position before appending
			qm.cache.Set(sessionInQueue.ID, cachedSessionData, qm.inactivityTimeoutDur)
			newQueue = append(newQueue, cachedSessionData)
		} else {
			qm.logf("info", "Removing client %s from queue due to inactivity (expired from cache).", sessionInQueue.ID)
			removedQueueCount++
		}
	}
	qm.queue = newQueue // Replace old queue with the filtered one

	// 3. Promote clients from queue if capacity is available
	for len(qm.activeSessionIDs) < qm.config.MaxEntries && len(qm.queue) > 0 {
		// Get the client from the front of the queue (FIFO)
		promotedClientSession := qm.queue[0]
		qm.queue = qm.queue[1:] // Dequeue

		qm.logf("info", "Promoting client %s from queue to active. Queue size now %d.", promotedClientSession.ID, len(qm.queue))

		// Update session data for activation
		promotedClientSession.LastSeenAt = now
		promotedClientSession.Position = -1 // Mark as not in queue
		if qm.hardSessionLimitDur > 0 {
			// Set/reset HardExpiryAt based on promotion time for fairness
			promotedClientSession.HardExpiryAt = now.Add(qm.hardSessionLimitDur)
			promotedClientSession.CreatedAt = now // Consider promotion time as new "creation" for hard limit
		}

		qm.activeSessionIDs[promotedClientSession.ID] = true
		qm.cache.Set(promotedClientSession.ID, promotedClientSession, qm.inactivityTimeoutDur)
		promotedCount++
	}

	// 4. After promotions, re-index positions for the remaining items in the queue
	for i := range qm.queue {
		qm.queue[i].Position = i
		// Optionally update cache for these items if position is critical there,
		// but it's mainly for display from this function's perspective.
		if s, found := qm.getSessionFromCache(qm.queue[i].ID); found {
			s.Position = i
			qm.cache.Set(s.ID, s, qm.inactivityTimeoutDur) // Keep it fresh in cache
		}
	}

	if removedActiveCount > 0 || removedQueueCount > 0 || promotedCount > 0 {
		qm.logf("info", "Cleanup results: Removed Active: %d, Removed Queued: %d, Promoted: %d. Current State -> Active: %d, Queue: %d",
			removedActiveCount, removedQueueCount, promotedCount, len(qm.activeSessionIDs), len(qm.queue))
	} else {
		qm.logf("debug", "Cleanup run: No significant changes. Active: %d, Queue: %d", len(qm.activeSessionIDs), len(qm.queue))
	}
}

// dequeueClientUnderLock removes a client from the queue by ID.
// IMPORTANT: This helper assumes qm.mu is already write-locked by the caller.
func (qm *QueueManager) dequeueClientUnderLock(clientID string) {
	foundIndex := -1
	for i, session := range qm.queue {
		if session.ID == clientID {
			foundIndex = i
			break
		}
	}

	if foundIndex != -1 {
		// Remove the element by slicing: qm.queue = append(qm.queue[:foundIndex], qm.queue[foundIndex+1:]...)
		// More explicit copy for clarity if needed, but above is idiomatic.
		copy(qm.queue[foundIndex:], qm.queue[foundIndex+1:]) // Shift elements left
		qm.queue = qm.queue[:len(qm.queue)-1]                // Truncate slice

		qm.logf("debug", "Client %s dequeued (under lock).", clientID)

		// Re-index positions for remaining items in the queue
		for i := range qm.queue {
			qm.queue[i].Position = i
			// Optionally update cache for these items if their position needs to be reflected there.
			// For now, position in cache is mostly for debug or if session is re-fetched.
			if s, found := qm.getSessionFromCache(qm.queue[i].ID); found {
				s.Position = i
				qm.cache.Set(s.ID, s, qm.inactivityTimeoutDur)
			}
		}
	}
}

// fileExists checks if a file exists at the given path.
func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && !info.IsDir() // Ensure it's a file, not a directory
}

// Stop signals the cleanup goroutine to terminate and cleans up resources.
// Note: Traefik plugins don't have a formal "Stop" or "Shutdown" lifecycle method
// that's guaranteed to be called. This is for potential use in other contexts or future Traefik enhancements.
func (qm *QueueManager) Stop() {
	qm.logf("info", "QueueManager plugin stopping...")
	if qm.stopCleanup != nil {
		// Non-blocking send in case channel is already closed or receiver is gone
		select {
		case qm.stopCleanup <- true:
			qm.logf("debug", "Stop signal sent to cleanup routine.")
		default:
			qm.logf("warn", "Could not send stop signal to cleanup routine (already stopped or not running?).")
		}
		close(qm.stopCleanup) // Close channel to prevent further sends
	}

	if qm.cache != nil {
		qm.cache.Stop() // Stop the cache's internal cleanup timer
		qm.logf("debug", "Cache cleanup routine signaled to stop.")
	}

	// Close log file if it was opened and stored
	if qm.logFileHandle != nil {
		qm.logf("info", "Closing log file: %s", qm.config.LogFile)
		err := qm.logFileHandle.Close()
		if err != nil {
			// Log to stderr if log file closing fails, as qm.logger might be using the file.
			log.Printf(logPrefix+"ERROR: Failed to close log file %s: %v", qm.config.LogFile, err)
		}
		qm.logFileHandle = nil // Avoid double closing
	}
	qm.logf("info", "QueueManager plugin has been signaled to stop.")
}

// startCleanupRoutine initializes and starts the periodic cleanup task.
func (qm *QueueManager) startCleanupRoutine() {
	qm.cleanupTicker = time.NewTicker(qm.cleanupIntervalDur)
	go func() {
		for {
			select {
			case <-qm.cleanupTicker.C:
				qm.logf("debug", "Cleanup routine triggered.")
				qm.CleanupExpiredSessions()
			case <-qm.stopCleanup:
				qm.cleanupTicker.Stop()
				qm.logf("info", "Cleanup routine has been stopped.")
				return
			}
		}
	}()
	qm.logf("info", "Cleanup routine started. Interval: %v", qm.cleanupIntervalDur)
}
