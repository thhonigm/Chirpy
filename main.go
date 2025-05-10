package main

import ( 
  "fmt"
  "encoding/json"
  "net/http"
  "sync/atomic"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
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

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
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

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
  cfg.fileserverHits.Store(0)
  w.Header().Set("Content-Type", "text/plain; charset=utf-8");
  w.WriteHeader(http.StatusOK);
}

type validateError struct {
  Error string `json:"error"`
}

func (e *validateError) sendResponse(w http.ResponseWriter, code int) {
  data, err := json.Marshal(e)
  if err != nil {
    data = []byte(fmt.Sprintf(`{"error":"Error marshalling JSON: %s}"`, err))
    code = http.StatusInternalServerError
  }
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(code)
  w.Write(data)
}

type validateOK struct {
  Valid bool `json:"valid"`
}

func (o *validateOK) sendResponse(w http.ResponseWriter) {
  code := http.StatusOK
  data, err := json.Marshal(o)
  if err != nil {
    data = []byte(fmt.Sprintf(`{"error":"Error marshalling JSON: %s}"`, err))
    code = http.StatusInternalServerError
  }
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(code)
  w.Write(data)
}

func validateChirp(w http.ResponseWriter, r *http.Request) {
  type chirp struct {
    Body string `json:"body"`
  }
  decoder := json.NewDecoder(r.Body)
  ch := chirp{}
  err := decoder.Decode(&ch)
  if err != nil {
    errmsg := validateError{
      Error: fmt.Sprintf("Error decoding JSON: %v", err),
    }
    errmsg.sendResponse(w, http.StatusInternalServerError)
  } else {
    if len(ch.Body) > 140 {
      errmsg := validateError{
        Error: "Chirp is too long",
      }
      errmsg.sendResponse(w, http.StatusBadRequest)
    } else {
      okmsg := validateOK{
        Valid: true,
      }
      okmsg.sendResponse(w)
    }
  }
}

func main() {
  var apiCfg apiConfig
  mux := http.NewServeMux()
  mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
  mux.HandleFunc("GET /api/healthz", healthHandler)
  mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
  mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
  mux.HandleFunc("POST /api/validate_chirp", validateChirp)
  var srv http.Server
  srv.Handler = mux
  srv.Addr = ":8080"
  srv.ListenAndServe()
}
