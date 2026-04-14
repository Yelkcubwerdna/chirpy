package main

import (
	"chirpy/internal/auth"
	"chirpy/internal/database"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	dbQueries      database.Queries
	fileserverHits atomic.Int32
	platform       string
	secret         string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) hitsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Create string showing how many hits the server has counted
	hitsString := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())

	w.Write([]byte(hitsString))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, http.StatusForbidden, "403 Forbidden")
		return
	}

	// Reset hits file server hits
	cfg.fileserverHits.Swap(0)

	// Delete all users from database
	err := cfg.dbQueries.ResetUsers(r.Context())
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reseting users databse: %s", err))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits Reset to 0"))
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters: %s", err))
		return
	}

	hashed, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error hashing password: %s", err))
		return
	}

	dbUser, err := cfg.dbQueries.CreateUser(context.Background(),
		database.CreateUserParams{
			Email:          params.Email,
			HashedPassword: hashed,
		})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating user: %s", err))
		return
	}

	responseUser := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}

	respondWithJSON(w, http.StatusCreated, responseUser)
}

func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	// Get JWT Token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Invalid Authorization\nError: %v", err))
		return
	}

	// Validate the token
	userId, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Invalid Authorization\nError: %v", err))
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters: %s", err))
		return
	}

	if utf8.RuneCountInString(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	dbChirp, err := cfg.dbQueries.CreateChirp(r.Context(),
		database.CreateChirpParams{
			Body:   cleanChirp(params.Body),
			UserID: userId,
		})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating chirp: %s", err))
		return
	}

	responseChirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	respondWithJSON(w, http.StatusCreated, responseChirp)
}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	// Get chirps
	dbChirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error getting chirps: %s", err))
		return
	}

	// Map chirps to chirp struct
	responseChirps := []Chirp{}

	for _, ch := range dbChirps {
		responseChirps = append(responseChirps, Chirp{
			ID:        ch.ID,
			CreatedAt: ch.CreatedAt,
			UpdatedAt: ch.UpdatedAt,
			Body:      ch.Body,
			UserID:    ch.UserID,
		})
	}

	// Return chirps
	respondWithJSON(w, 200, responseChirps)
}

func (cfg *apiConfig) getChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpString := r.PathValue("chirpID")

	chirpUUID, err := uuid.Parse(chirpString)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Invalid Chirp ID: %s", err))
		return
	}

	dbChirp, err := cfg.dbQueries.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Chirp does not exist: %s", err))
		return
	}

	respChirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	respondWithJSON(w, 200, respChirp)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password         string `json:"password"`
		Email            string `json:"email"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error decoding parameters: %s", err))
		return
	}

	// Verify if an expiration was given or valid
	if params.ExpiresInSeconds < 1 || params.ExpiresInSeconds > 3600 {
		// Set to default of 1 hour
		params.ExpiresInSeconds = 3600
	}

	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "incorrect email or password")
		return
	}

	valid, err := auth.CheckPasswordHash(params.Password, dbUser.HashedPassword)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error checking password: %s", err))
		return
	}

	if !valid {
		respondWithError(w, http.StatusUnauthorized, "incorrect email or password")
	} else {
		duration, err := time.ParseDuration(fmt.Sprintf("%ds", params.ExpiresInSeconds))
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating time.Duration object: %v", err))
		}

		// Get a token to give to user
		token, err := auth.MakeJWT(dbUser.ID, cfg.secret, duration)
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating token: %v", err))
			return
		}

		type respUser struct {
			Id         uuid.UUID `json:"id"`
			Created_at time.Time `json:"created_at"`
			Updated_at time.Time `json:"updated_at"`
			Email      string    `json:"email"`
			Token      string    `json:"token"`
		}

		resp := respUser{
			Id:         dbUser.ID,
			Created_at: dbUser.CreatedAt,
			Updated_at: dbUser.UpdatedAt,
			Email:      dbUser.Email,
			Token:      token,
		}

		respondWithJSON(w, http.StatusOK, resp)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errRsp struct {
		Err string `json:"error"`
	}

	errorBody := errRsp{
		Err: msg,
	}

	dat, err := json.Marshal(errorBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSOM: %s", err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content/Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func cleanChirp(s string) string {
	words := strings.Split(s, " ")
	bad_words := []string{"kerfuffle", "sharbert", "fornax"}

	for i, w := range words {
		for _, bw := range bad_words {
			if strings.ToLower(w) == bw {
				words[i] = "****"
			}
		}
	}

	return strings.Join(words, " ")
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error loading database: %v\n", err)
		os.Exit(1)
	}

	dbQueries := database.New(db)

	mux := http.NewServeMux()

	platform := os.Getenv("PLATFORM")

	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		dbQueries:      *dbQueries,
		platform:       platform,
		secret:         os.Getenv("SECRET"),
	}

	server := http.Server{}
	server.Handler = mux
	server.Addr = ":8080"

	fileserver := http.FileServer(http.Dir("."))

	healthzHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fileserver)))
	mux.HandleFunc("GET /api/healthz", healthzHandler)
	mux.HandleFunc("GET /admin/metrics", apiCfg.hitsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirpHandler)
	mux.HandleFunc("POST /api/users", apiCfg.createUserHandler)
	mux.HandleFunc("GET /api/chirps", apiCfg.getChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirpHandler)
	mux.HandleFunc("POST /api/login", apiCfg.loginHandler)

	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("Error starting server: %v", "err")
		os.Exit(1)
	}
}
