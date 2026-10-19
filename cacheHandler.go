package loch_queue

import "time"

// getSessionFromCache retrieves and type-asserts a session from the cache.
// This is a read-only operation on the cache itself.
func (qm *QueueManager) getSessionFromCache(clientID string) (Session, bool) {
	obj, found := qm.cache.Get(clientID) // This cache.Get is thread-safe
	if !found {
		return Session{}, false
	}
	sessionData, ok := obj.(Session)
	if !ok {
		qm.logf("warn", "Failed to assert session data type from cache for client %s. Cache data: %+v. Treating as not found.", clientID, obj)
		// Optionally, delete the problematic cache entry: qm.cache.Delete(clientID)
		return Session{}, false
	}
	return sessionData, true
}

// updateClientActivityInCache updates the LastSeenAt timestamp for a client's session in the cache.
func (qm *QueueManager) updateClientActivityInCache(clientID string, sessionData Session) {
	sessionData.LastSeenAt = time.Now()
	// Important: HardExpiryAt is NOT updated by activity. It's a fixed point from CreatedAt.
	qm.cache.Set(clientID, sessionData, qm.inactivityTimeoutDur) // Reset inactivity timer by re-setting in cache
	qm.logf("debug", "Updated LastSeenAt for client %s in cache.", clientID)
}

// enqueueOrUpdateClient places a client in the queue or updates their LastSeenAt if already queued.
// Returns the client's 0-based position in the queue.
// This function acquires the main lock (qm.mu).
func (qm *QueueManager) enqueueOrUpdateClient(clientID string) int {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// Check if client is already in the queue
	for i := range qm.queue {
		if qm.queue[i].ID == clientID {
			qm.logf("debug", "Client %s already in queue at position %d. Updating activity.", clientID, i)
			// Update LastSeenAt for the existing queue entry (via cache)
			if sessionData, found := qm.getSessionFromCache(clientID); found {
				qm.updateClientActivityInCache(clientID, sessionData) // Updates cache
				qm.queue[i].LastSeenAt = time.Now()                   // Also update in-memory queue representation
			} else {
				// Session not in cache, but in queue? This is inconsistent. Re-add to cache.
				qm.logf("warn", "Client %s in queue but not in cache. Re-caching.", clientID)
				qm.queue[i].LastSeenAt = time.Now()
				qm.cache.Set(clientID, qm.queue[i], qm.inactivityTimeoutDur)
			}
			return i // Return current position
		}
	}

	// Client is not in queue, add them
	now := time.Now()
	newQueueSession := Session{
		ID:         clientID,
		CreatedAt:  now, // Time they entered the queue system
		LastSeenAt: now,
		Position:   len(qm.queue), // 0-based position for the new entry
		// HardExpiryAt is typically not set for queued items, only for active ones.
	}
	qm.queue = append(qm.queue, newQueueSession)
	// Add to cache with inactivity timeout; queued users also need to show activity.
	qm.cache.Set(clientID, newQueueSession, qm.inactivityTimeoutDur)

	qm.logf("info", "Client %s added to queue at position %d. Current queue size: %d", clientID, newQueueSession.Position, len(qm.queue))
	return newQueueSession.Position
}
