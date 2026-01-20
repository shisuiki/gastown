package web

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/models"
)

// handleAPIModelsList returns all available models from CLI discovery.
func (h *GUIHandler) handleAPIModelsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Use stale-while-revalidate with short TTL (models can change)
	cached := h.cache.GetStaleOrRefresh("models_list", ModelsCacheTTL, func() interface{} {
		return h.fetchModelsList()
	})

	if cached != nil {
		json.NewEncoder(w).Encode(cached)
		return
	}

	// No cache - fetch synchronously
	result := h.fetchModelsList()
	h.cache.Set("models_list", result, ModelsCacheTTL)
	json.NewEncoder(w).Encode(result)
}

// fetchModelsList discovers available models via CLI adapters.
// Returns a map of agent type to list of normalized model IDs.
func (h *GUIHandler) fetchModelsList() map[string][]string {
	agents := config.ListAgentPresets()
	result := make(map[string][]string, len(agents))

	for _, agent := range agents {
		models := models.Discover(agent)
		if len(models) > 0 {
			result[agent] = models
		} else {
			result[agent] = []string{}
			log.Printf("No models discovered for agent %q", agent)
		}
	}
	return result
}
