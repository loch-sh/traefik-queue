package loch_queue

import (
	"fmt"
	"math"
)

// prepareQueuePageData calculates and prepares the data for rendering the queue page.
func (qm *QueueManager) prepareQueuePageData(positionInQueue int) QueuePageData {
	qm.mu.RLock() // Read lock for accessing queue size and active count
	queueSize := len(qm.queue)
	activeCount := len(qm.activeSessionIDs)
	qm.mu.RUnlock()

	// Estimated wait time logic (simplified: 0.3 to 0.7 minutes per person ahead)
	// This can be made more sophisticated if average service time is known.
	// Using a slightly variable factor to make it seem less static.
	waitFactor := 0.3 + (float64(positionInQueue%5) * 0.08)   // Varies between 0.3 and 0.62
	rawEstimatedTime := float64(positionInQueue) * waitFactor // positionInQueue is 0-based here

	estimatedWaitTime := int(math.Max(float64(qm.config.MinWaitTimeMinutes), math.Ceil(rawEstimatedTime)))
	// If they are at position 0 and served queue page, it means capacity was full at that instant.
	// So, even at pos 0, show MinWaitTimeMinutes unless it's genuinely 0.
	if positionInQueue == 0 && estimatedWaitTime == 0 && qm.config.MinWaitTimeMinutes > 0 {
		estimatedWaitTime = qm.config.MinWaitTimeMinutes
	}

	// Calculate progress percentage
	progressPercentage := 0
	if queueSize > 0 {
		if positionInQueue >= queueSize { // Should not happen if pos is 0-based from len(queue)
			progressPercentage = 1 // At least 1% if considered in queue but pos is off
		} else if queueSize == 1 && positionInQueue == 0 { // Special case: Only one in queue
			progressPercentage = 50 // Arbitrary: in queue, but you're next, show 50%
		} else {
			// Progress: (queueSize - (positionInQueue+1)) / queueSize * 100
			// Example: 5 in queue, you are pos 0 (1st). Progress: (5-1)/5 = 80%
			// Example: 5 in queue, you are pos 4 (5th). Progress: (5-5)/5 = 0% -> adjust to show some progress
			progress := float64(queueSize-(positionInQueue+1)) / float64(queueSize)
			progressPercentage = int(math.Round(progress * 100))
			if progressPercentage <= 0 && positionInQueue < queueSize { // If calculated to 0 or less but still in queue
				progressPercentage = 1 // Ensure at least 1%
			}
		}
	} else if positionInQueue == 0 { // No one in queue, and you are pos 0 (means you are about to get in or just missed)
		progressPercentage = 99 // Almost there
	}
	// Clamp percentage
	if progressPercentage > 100 {
		progressPercentage = 100
	}
	if progressPercentage < 0 {
		progressPercentage = 0
	}

	var debugInfo string
	if qm.config.Debug {
		debugInfo = fmt.Sprintf("Pos(0-based): %d, QSize: %d, Active: %d, Max: %d, RawWait: %.2f min, Factor: %.2f",
			positionInQueue, queueSize, activeCount, qm.config.MaxEntries, rawEstimatedTime, waitFactor)
	}

	return QueuePageData{
		Position:           positionInQueue + 1, // Display 1-based position
		QueueSize:          queueSize,
		EstimatedWaitTime:  estimatedWaitTime,
		RefreshInterval:    qm.config.RefreshIntervalSeconds,
		ProgressPercentage: progressPercentage,
		DebugInfo:          debugInfo,
	}
}
