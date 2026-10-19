package loch_queue

import (
	"net/http"
	"strings"
)

func (qm *QueueManager) getTranslatedMessages(req *http.Request) TranslatedMessages {
	qm.logf("debug", "Getting translated messages for user language")

	acceptLang := req.Header.Get("Accept-Language")
	if acceptLang != "" {
		// Exemple : "fr-FR,fr;q=0.9,en;q=0.8"
		parts := strings.Split(acceptLang, ",")
		if len(parts) > 0 {
			langCode := strings.ToLower(strings.TrimSpace(parts[0]))
			if len(langCode) >= 2 {
				langCode = langCode[:2] // On garde juste "fr", "en", etc.
				qm.logf("debug", "Extracted language : %s, %v", langCode, qm.translatedMessages[langCode])

				return qm.translatedMessages[langCode]
			}
		}
	}
	// Fallback
	qm.logf("debug", "No language-specific template found, using default: %s, %v", qm.config.QueuePageFile, qm.translatedMessages)
	return qm.translatedMessages["en"]
}
