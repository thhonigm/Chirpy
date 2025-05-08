package main

import ( 
  "fmt"
  "net/http"
  "sync/atomic"
)

func healthHandler(w http.ResponseWriter, h *http.Request) {
  w.Header().Set("Content-Type", "text/plain; charset=utf-8");
  w.WriteHeader(http.StatusOK);
  w.Write([]byte("OK"));
}

type apiConfig struct {
  fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    cfg.fileserverHits.Add(1);
    next.ServeHTTP(w, r);
  })
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, h *http.Request) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8");
  w.WriteHeader(http.StatusOK);
  w.Write([]byte(fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, h *http.Request) {
  cfg.fileserverHits.Store(0)
  w.Header().Set("Content-Type", "text/plain; charset=utf-8");
  w.WriteHeader(http.StatusOK);
}

func main() {
  var apiCfg apiConfig
  mux := http.NewServeMux()
  mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
  mux.HandleFunc("GET /api/healthz", healthHandler)
  mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
  mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
  var srv http.Server
  srv.Handler = mux
  srv.Addr = ":8080"
  srv.ListenAndServe()
}
