package loch_queue

import (
	"fmt"
	"net/http"
)

// Constants for configuration defaults and internal settings.
const (
	defaultLogLevel              = "info" // Default logging level
	logPrefix                    = "[Loch-Queue] "
	defaultInactivityTimeoutSecs = 60  // Default seconds for a session to be considered inactive
	defaultCleanupIntervalSecs   = 30  // Default seconds for how often cleanup logic runs (reduced for responsiveness)
	defaultMaxEntries            = 100 // Default maximum concurrent users
	defaultHTTPResponseCode      = http.StatusTooManyRequests
	defaultHTTPContentType       = "text/html; charset=utf-8"
	defaultUseCookies            = true
	defaultCookieName            = "loch-queue-id"
	defaultCookieMaxAgeSecs      = 3600   // 1 hour
	defaultQueueStrategy         = "fifo" // Currently only "fifo" is implemented
	defaultRefreshIntervalSecs   = 20     // Default seconds for queue page refresh (reduced for better UX)
	defaultMinWaitTimeMinutes    = 1      // Default minimum wait time displayed to users
	defaultQueuePageFile         = "/var/public/queue-page.html"
	defaultQueueTranslationsFile = "/var/public/translations.json"
	defaultRateLimitTimes        = 5
	secureIDLengthBytes          = 16 // Number of random bytes for secure cookie ID generation
)

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Enabled:                  true,
		QueuePageFile:            defaultQueuePageFile,
		QueueTranslationsFile:    defaultQueueTranslationsFile,
		InactivityTimeoutSeconds: defaultInactivityTimeoutSecs,
		HardSessionLimitSeconds:  0, // Disabled by default
		CleanupIntervalSeconds:   defaultCleanupIntervalSecs,
		MaxEntries:               defaultMaxEntries,
		HTTPResponseCode:         defaultHTTPResponseCode,
		HTTPContentType:          defaultHTTPContentType,
		UseCookies:               defaultUseCookies,
		CookieName:               defaultCookieName,
		CookieMaxAgeSeconds:      defaultCookieMaxAgeSecs,
		QueueStrategy:            defaultQueueStrategy,
		RefreshIntervalSeconds:   defaultRefreshIntervalSecs,
		Debug:                    false,
		MinWaitTimeMinutes:       defaultMinWaitTimeMinutes,
		LogFile:                  "",
		LogLevel:                 defaultLogLevel,
		RateLimitTimes:           defaultRateLimitTimes,
	}
}

// validateConfig checks for essential configuration parameters.
func validateConfig(config *Config) error {
	if config.MaxEntries <= 0 {
		return fmt.Errorf("maxEntries must be greater than 0 (got %d)", config.MaxEntries)
	}
	if config.InactivityTimeoutSeconds <= 0 {
		return fmt.Errorf("inactivityTimeoutSeconds must be greater than 0 (got %d)", config.InactivityTimeoutSeconds)
	}
	if config.CleanupIntervalSeconds <= 0 {
		return fmt.Errorf("cleanupIntervalSeconds must be greater than 0 (got %d)", config.CleanupIntervalSeconds)
	}
	if config.HardSessionLimitSeconds < 0 { // 0 is valid (disabled)
		return fmt.Errorf("hardSessionLimitSeconds cannot be negative (got %d)", config.HardSessionLimitSeconds)
	}
	if config.RefreshIntervalSeconds <= 0 {
		return fmt.Errorf("refreshIntervalSeconds must be greater than 0 (got %d)", config.RefreshIntervalSeconds)
	}
	if config.QueuePageFile == "" {
		return fmt.Errorf("queuePageFile cannot be empty")
	}
	return nil
}
