package loch_queue

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error   string         `json:"error"`
	Handler string         `json:"handler"`
	Detail  *QueuePageData `json:"detail,omitempty"`
}

// serveAPIResponse return response for API based queries
func (qm *QueueManager) serveAPIResponse(rw http.ResponseWriter, data QueuePageData) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("X-Loch-sh", "Loch Queue")
	rw.WriteHeader(qm.config.HTTPResponseCode)

	response := ErrorResponse{
		Error:   "capacity overload",
		Handler: "Loch.sh Queue Manager",
		Detail:  &data,
	}

	err := json.NewEncoder(rw).Encode(response)
	if err != nil {
		qm.logf("error", "Failed to encode response body: %s", err)
		return
	}
}
