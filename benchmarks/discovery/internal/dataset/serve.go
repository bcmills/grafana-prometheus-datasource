package dataset

import (
	"net/http"
	"os"
	"sync"
)

func ServeMetricsFile(addr, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(body)
	})
	return http.ListenAndServe(addr, handler) // #nosec G114 -- local benchmark fixture
}
