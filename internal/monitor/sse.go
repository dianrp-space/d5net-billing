package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func SSEHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "sse unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		tenantID := xid.Nil()
		if q := r.URL.Query().Get("tenant_id"); q != "" {
			if id, err := xid.Parse(q); err == nil {
				tenantID = id
			}
		}

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				stats, _ := st.DashboardStats(r.Context(), tenantID)
				if stats == nil {
					stats = map[string]any{}
				}
				alerts, _ := st.ListRecentAlerts(r.Context(), tenantID, 10)
				if alerts == nil {
					alerts = []store.Alert{}
				}
				payload := map[string]any{
					"stats":  stats,
					"alerts": alerts,
				}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}
	}
}
