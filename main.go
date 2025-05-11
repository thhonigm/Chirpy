package main

import _ "github.com/lib/pq"

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/thhonigm/Chirpy/internal/database"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	cfg.fileserverHits.Store(0)
	if cfg.platform == "dev" {
		err := cfg.db.DeleteUsers(r.Context())
		if err != nil {
			sendJSONResponse(w, http.StatusInternalServerError, jsonError{
				Error: fmt.Sprintf("Error deleting users: %v", err),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}
}

func sendJSONResponse(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"error":"Error marshalling JSON: %s}"`, err))
		code = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

type jsonError struct {
	Error string `json:"error"`
}

func (e *jsonError) sendResponse(w http.ResponseWriter, code int) {
	sendJSONResponse(w, code, e)
}

type validateOK struct {
	Body string `json:"cleaned_body"`
}

func (o *validateOK) sendResponse(w http.ResponseWriter) {
	sendJSONResponse(w, http.StatusOK, o)
}

func cleanChirp(chirp string) string {
	s := strings.Split(chirp, " ")
	var c []string
	for i := 0; i < len(s); i++ {
		censor := false
		switch l := strings.ToLower(s[i]); l {
		case "kerfuffle":
			censor = true
		case "sharbert":
			censor = true
		case "fornax":
			censor = true
		}
		if censor {
			c = append(c, "****")
		} else {
			c = append(c, s[i])
		}
	}
	return strings.Join(c, " ")
}

func validateChirp(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}
	decoder := json.NewDecoder(r.Body)
	ch := chirp{}
	err := decoder.Decode(&ch)
	if err != nil {
		errmsg := jsonError{
			Error: fmt.Sprintf("Error decoding JSON: %v", err),
		}
		errmsg.sendResponse(w, http.StatusInternalServerError)
	} else {
		if len(ch.Body) > 140 {
			errmsg := jsonError{
				Error: "Chirp is too long",
			}
			errmsg.sendResponse(w, http.StatusBadRequest)
		} else {
			okmsg := validateOK{
				Body: cleanChirp(ch.Body),
			}
			okmsg.sendResponse(w)
		}
	}
}

func (cfg *apiConfig) usersHandler(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	p := params{}
	err := decoder.Decode(&p)
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, jsonError{
			Error: fmt.Sprintf("Error decoding JSON: %v", err),
		})
		return
	}
	user, err := cfg.db.CreateUser(r.Context(), p.Email)
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, jsonError{
			Error: fmt.Sprintf("Error creating user: %v", err),
		})
		return
	}
	type userCreated struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}
	sendJSONResponse(w, http.StatusCreated, userCreated{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	})
}

func main() {
	godotenv.Load()
	var apiCfg apiConfig
	db, err := sql.Open("postgres", os.Getenv("DB_URL"))
	if err != nil {
		fmt.Printf("sql.Open: %v\n", err)
		return
	}
	apiCfg.db = database.New(db)
	apiCfg.platform = os.Getenv("PLATFORM")
	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("GET /api/healthz", healthHandler)
	mux.HandleFunc("POST /api/users", apiCfg.usersHandler)
	mux.HandleFunc("POST /api/validate_chirp", validateChirp)
	var srv http.Server
	srv.Handler = mux
	srv.Addr = ":8080"
	srv.ListenAndServe()
}
