package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// #region agent log
func agentDebugLog(location, message, hypothesisID string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	payload := map[string]any{
		"sessionId":    "f19e18",
		"runId":        "pre-fix",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if f, err := os.OpenFile("/home/dianrp/coding/d5net-billing/.cursor/debug-f19e18.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
	go func() {
		req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:7813/ingest/d5ceb638-f02e-4b79-b49c-b843ba23dc69", bytes.NewReader(b))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Debug-Session-Id", "f19e18")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}()
}

func webhookBodyKeys(body map[string]any) []string {
	if body == nil {
		return nil
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	return keys
}

// #endregion
