package loch_queue

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Config holds the plugin configuration.
type Config struct {
	Enabled                  bool   `json:"enabled"`                  // Enable/disable the queue manager
	QueuePageFile            string `json:"queuePageFile"`            // Path to queue page HTML template
	QueueTranslationsFile    string `json:"queueTranslationsFile"`    // Path to queue translations json template
	InactivityTimeoutSeconds int    `json:"inactivityTimeoutSeconds"` // How long an inactive session is valid for (in seconds)
	HardSessionLimitSeconds  int    `json:"hardSessionLimitSeconds"`  // Optional: Absolute max time for an active session (seconds), 0 to disable
	CleanupIntervalSeconds   int    `json:"cleanupIntervalSeconds"`   // How often to run cleanup logic (in seconds)
	MaxEntries               int    `json:"maxEntries"`               // Maximum concurrent users
	HTTPResponseCode         int    `json:"httpResponseCode"`         // HTTP response code for queue page
	HTTPContentType          string `json:"httpContentType"`          // Content type of queue page
	UseCookies               bool   `json:"useCookies"`               // Use cookies or IP+UserAgent hash for client identification
	CookieName               string `json:"cookieName"`               // Name of the cookie
	CookieMaxAgeSeconds      int    `json:"cookieMaxAgeSeconds"`      // Max age of the cookie in seconds
	QueueStrategy            string `json:"queueStrategy"`            // Queue strategy: "fifo" (currently only supported)
	RefreshIntervalSeconds   int    `json:"refreshIntervalSeconds"`   // Refresh interval for queue page (in seconds)
	Debug                    bool   `json:"debug"`                    // Enable verbose debug logging (overrides LogLevel to debug)
	MinWaitTimeMinutes       int    `json:"minWaitTimeMinutes"`       // Minimum wait time to show users (in minutes)
	LogFile                  string `json:"logFile"`                  // Optional: Path to a log file. Default is stderr.
	LogLevel                 string `json:"logLevel"`                 // Logging level: "debug", "info", "warn", "error"
	RateLimitTimes           int    `json:"rateLimitTimes"`           // Limit of per-IP call in window if no cookie is set between request in check interval
}

// Session represents a visitor session in the queue or active service.
type Session struct {
	ID           string    `json:"id"`           // Unique client identifier
	CreatedAt    time.Time `json:"createdAt"`    // Timestamp when the session was first created (for hard limit)
	LastSeenAt   time.Time `json:"lastSeenAt"`   // Timestamp when the client was last seen (for inactivity)
	HardExpiryAt time.Time `json:"hardExpiryAt"` // Absolute expiry time, if hard session limit is enabled
	Position     int       `json:"position"`     // 0-based position in the queue (-1 if not in queue/active)
}

// QueuePageData contains data to be passed to the HTML template.
type QueuePageData struct {
	Position           int    `json:"position"`           // 1-based position in queue for display
	QueueSize          int    `json:"queueSize"`          // Total current queue size
	EstimatedWaitTime  int    `json:"estimatedWaitTime"`  // Estimated wait time in minutes
	RefreshInterval    int    `json:"refreshInterval"`    // Refresh interval in seconds for the page
	ProgressPercentage int    `json:"progressPercentage"` // Visual progress percentage
	DebugInfo          string `json:"debugInfo"`          // Debug information (only shown if debug mode enabled)
}

type TranslatedMessages struct {
	PageTitle    string `json:"pageTitle"`
	Title        string `json:"title"`
	Introduction string `json:"introduction"`
	YourPosition string `json:"yourPosition"`
	ETA          string `json:"eta"`
	RefreshTime  string `json:"refreshTime"`
	Mins         string `json:"mins"`
}

type QueueTemplateData struct {
	QueueData    QueuePageData
	Translations TranslatedMessages
}

// QueueManager is the main middleware handler struct.
type QueueManager struct {
	next               http.Handler
	name               string
	config             *Config
	logger             *log.Logger
	cache              *SimpleCache // In-memory cache for session data
	tpl                *template.Template
	translatedMessages map[string]TranslatedMessages
	tplLock            sync.RWMutex // For thread-safe template parsing/reloading

	// Queue and active session management
	queue            []Session       // FIFO queue of waiting sessions
	activeSessionIDs map[string]bool // Set of currently active session IDs (value is always true)
	mu               sync.RWMutex    // Mutex for thread-safe access to queue and activeSessionIDs

	// Durations derived from config for convenience
	inactivityTimeoutDur time.Duration
	hardSessionLimitDur  time.Duration // Will be 0 if not configured
	cleanupIntervalDur   time.Duration

	// Cleanup routine
	cleanupTicker *time.Ticker
	stopCleanup   chan bool // Channel to signal the cleanup goroutine to stop

	// Log file handle if a log file is used
	logFileHandle *os.File

	// IP Table for no-cookie users
	rateLimit map[string][]time.Time
}
