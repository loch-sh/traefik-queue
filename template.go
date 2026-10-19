package loch_queue

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
)

// loadTemplate reads and parses the HTML template file.
func (qm *QueueManager) loadTemplate() error {
	qm.tplLock.Lock()
	defer qm.tplLock.Unlock()

	if !fileExists(qm.config.QueuePageFile) {
		qm.tpl = nil // Ensure template is nil if file doesn't exist
		return fmt.Errorf("template file not found: %s", qm.config.QueuePageFile)
	}

	content, err := os.ReadFile(qm.config.QueuePageFile)
	if err != nil {
		qm.tpl = nil
		return fmt.Errorf("error reading template file '%s': %w", qm.config.QueuePageFile, err)
	}

	newTpl, parseErr := template.New("QueuePage").Delims("[[", "]]").Funcs(template.FuncMap{
		"safeHtml": func(s string) template.HTML {
			return template.HTML(s)
		},
	}).Parse(string(content))
	if parseErr != nil {
		qm.tpl = nil
		return fmt.Errorf("error parsing template '%s': %w", qm.config.QueuePageFile, parseErr)
	}

	qm.tpl = newTpl
	qm.logf("info", "Successfully loaded/reloaded queue page template: %s", qm.config.QueuePageFile)
	return nil
}

// getTemplate safely retrieves the current parsed template.
func (qm *QueueManager) getTemplate() *template.Template {
	qm.tplLock.RLock()
	defer qm.tplLock.RUnlock()
	return qm.tpl
}

// serveFallbackTemplate provides a basic, hardcoded HTML queue page.
func (qm *QueueManager) serveFallbackTemplate(rw http.ResponseWriter, data QueueTemplateData) {
	// Minified and slightly improved fallback HTML
	fallbackHTML := `<!DOCTYPE html><html><head><title>Service Queue</title><meta http-equiv="refresh" content="[[.QueueData.RefreshInterval]]"><meta name="viewport" content="width=device-width, initial-scale=1.0"><style>body{font-family:Arial,sans-serif;text-align:center;margin:20px;padding:0;background-color:#f4f4f4;color:#333;} .container{max-width:600px;margin:40px auto;padding:20px;background-color:white;border-radius:8px;box-shadow:0 2px 10px rgba(0,0,0,0.1);} h1{color:#2c3e50;margin-bottom:15px;} p{line-height:1.6;} .progress-container{width:100%;background-color:#e9ecef;border-radius:5px;margin:25px 0;overflow:hidden;} .progress-bar{height:24px;width:[[.QueueData.ProgressPercentage]]%;background-color:#3498db;text-align:center;line-height:24px;color:white;font-weight:bold;transition:width .3s ease;} .info-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:15px;margin:20px 0;} .info-box{background-color:#f8f9fa;padding:15px;border-radius:5px;border-left:4px solid #3498db;} .info-box strong{display:block;margin-bottom:5px;color:#2c3e50;} .debug{font-size:0.85em;color:#7f8c8d;margin-top:20px;padding:10px;background-color:#ecf0f1;border-radius:4px;text-align:left;display:[[if .QueueData.DebugInfo]]block[[else]]none[[end]];}</style></head><body><div class="container"><h1>You're in the Queue</h1><p>Our service is currently experiencing high demand. Please wait, and this page will refresh automatically.</p><div class="progress-container"><div class="progress-bar">[[.QueueData.ProgressPercentage]]%</div></div><div class="info-grid"><div class="info-box"><strong>Your Position</strong>[[.QueueData.Position]] / [[.QueueData.QueueSize]]</div><div class="info-box"><strong>Est. Wait Time</strong>~[[.QueueData.EstimatedWaitTime]] min(s)</div></div><p>This page will refresh in <span id="countdown">[[.QueueData.RefreshInterval]]</span> seconds.</p><div class="debug"><strong>Debug Info:</strong> <pre>[[.QueueData.DebugInfo]]</pre></div></div><script>let s=[[.QueueData.RefreshInterval]];const e=document.getElementById("countdown");function n(){s--,e.textContent=s,s<=0&&window.location.reload(!0)}e&&setInterval(n,1e3);</script></body></html>`

	tmpl, err := template.New("FallbackQueuePage").Delims("[[", "]]").Parse(fallbackHTML)
	if err != nil {
		qm.logf("error", "FATAL: Could not parse internal fallback template: %v", err)
		http.Error(rw, "Service temporarily unavailable. Error Code: FBTPLP.", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", qm.config.HTTPContentType)
	rw.WriteHeader(qm.config.HTTPResponseCode)

	if execErr := tmpl.Execute(rw, data); execErr != nil {
		qm.logf("error", "Error executing fallback queue page template: %v", execErr)
		// Ultimate fallback to plain text if template execution fails
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8") // Ensure plain text
		_, printErr := fmt.Fprintf(rw, "You are %d of %d in queue. Estimated wait: ~%d min(s). Page will refresh in %d seconds.",
			data.QueueData.Position, data.QueueData.QueueSize, data.QueueData.EstimatedWaitTime, data.QueueData.RefreshInterval)
		if printErr != nil {
			return
		}
	}
}

// serveQueuePage renders and serves the queue page HTML to the client.
func (qm *QueueManager) serveQueuePage(rw http.ResponseWriter, req *http.Request, positionInQueue int) {
	pageData := qm.prepareQueuePageData(positionInQueue)
	translations := qm.getTranslatedMessages(req)
	qm.logf("denug", "Got translations: %v", translations)

	data := QueueTemplateData{
		QueueData:    pageData,
		Translations: translations,
	}

	// Set headers to prevent caching of the queue page
	rw.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, post-check=0, pre-check=0")
	rw.Header().Set("Pragma", "no-cache")
	rw.Header().Set("Expires", "Thu, 01 Jan 1970 00:00:00 GMT") // Classic "don't cache"

	currentTpl := qm.getTemplate()
	if currentTpl == nil {
		// Attempt to reload template if it was nil (e.g., initial load failed or file changed)
		qm.logf("debug", "Queue page template is nil, attempting to reload from: %s", qm.config.QueuePageFile)
		if err := qm.loadTemplate(); err != nil {
			qm.logf("warn", "Failed to load template '%s' on demand: %v. Serving fallback.", qm.config.QueuePageFile, err)
		}
		currentTpl = qm.getTemplate() // Get again after attempting load
	}

	if currentTpl != nil {
		rw.Header().Set("Content-Type", qm.config.HTTPContentType)
		rw.WriteHeader(qm.config.HTTPResponseCode)
		if err := currentTpl.Execute(rw, data); err != nil {
			qm.logf("error", "Error executing custom queue page template: %v. Serving fallback.", err)
			// Fall through to serve fallback template if execution fails
		} else {
			qm.logf("debug", "Served custom queue page to client. Position (1-based): %d", pageData.Position)
			return // Successfully served custom template
		}
	}

	// Fallback to default internal template if custom template is not available or failed
	qm.logf("debug", "Serving fallback queue page. Position (1-based): %d", pageData.Position)
	qm.serveFallbackTemplate(rw, data)
}
