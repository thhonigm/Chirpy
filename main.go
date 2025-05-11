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

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {
	type chirp struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}
	ch := chirp{}
	err := json.NewDecoder(r.Body).Decode(&ch)
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, jsonError{
			Error: fmt.Sprintf("Error decoding JSON: %v", err),
		})
		return
	}
	if len(ch.Body) > 140 {
		sendJSONResponse(w, http.StatusBadRequest, jsonError{
			Error: "Chirp is too long",
		})
		return
	}
	user, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleanChirp(ch.Body),
		UserID: ch.UserID,
	})
	if err != nil {
		sendJSONResponse(w, http.StatusInternalServerError, jsonError{
			Error: fmt.Sprintf("Error creating chirp: %v", err),
		})
		return
	}
	type chirpCreated struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}
	sendJSONResponse(w, http.StatusCreated, chirpCreated{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Body:      user.Body,
		UserID:    user.UserID,
	})
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
	mux.HandleFunc("POST /api/chirps", apiCfg.chirpsHandler)
	var srv http.Server
	srv.Handler = mux
	srv.Addr = ":8080"
	srv.ListenAndServe()
}
