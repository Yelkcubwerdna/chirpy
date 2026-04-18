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
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
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

	respondWithJSON(w, http.StatusCreated, makeUserResponseData(dbUser))
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
		duration, err := time.ParseDuration("3600s")
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating time.Duration object: %v", err))
			return
		}

		// Get a token to give to user
		token, err := auth.MakeJWT(dbUser.ID, cfg.secret, duration)
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating token: %v", err))
			return
		}

		// Get a refresh_token
		refresh_token := auth.MakeRefreshToken()

		// Get expiration time
		days60, err := time.ParseDuration("5184000s")
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating time.Duration: %v", err))
			return
		}
		expires := time.Now().UTC().Add(days60)

		// Store refresh_token in database
		dbRefreshToken, err := cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
			Token:     refresh_token,
			UserID:    dbUser.ID,
			ExpiresAt: expires,
		})
		if err != nil {
			respondWithError(w, 500, fmt.Sprintf("Error creating refresh token: %v", err))
			return
		}

		type respUser struct {
			User
			Token        string `json:"token"`
			RefreshToken string `json:"refresh_token"`
		}

		resp := respUser{
			User:         makeUserResponseData(dbUser),
			Token:        token,
			RefreshToken: dbRefreshToken.Token,
		}

		respondWithJSON(w, http.StatusOK, resp)
	}
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	// Get token from header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Get token data from database
	dbToken, err := cfg.dbQueries.GetRefrshToken(r.Context(), token)
	if err != nil {
		respondWithError(w, 401, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Makes sure token hasn't expired or been revoked
	if (dbToken.ExpiresAt.Compare(time.Now()) == -1) || dbToken.RevokedAt.Valid {
		respondWithError(w, 401, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	hour, err := time.ParseDuration("60m")
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error parsing duration: %v", err))
		return
	}
	token, err = auth.MakeJWT(dbToken.UserID, cfg.secret, hour)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating JWT: %v", err))
		return
	}

	// If this code is reached, the token is valid. Return that value
	type Resp struct {
		Token string `json:"token"`
	}

	response := Resp{
		Token: token,
	}

	respondWithJSON(w, 200, response)
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {
	// Get the token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Token doesn't exist: %v", err))
		return
	}

	// Revoke the token
	err = cfg.dbQueries.RevokeToken(r.Context(), token)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Couldn't revoken token: %v", err))
		return
	}

	// Respond with 204 status code
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) updateAccountHandler(w http.ResponseWriter, r *http.Request) {
	// Set up struct for body
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	// Get parameters out of body
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	decoder.Decode(&params)

	// Get JWT token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Valid token and get user id
	userId, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Hash the password
	hashed, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Update the user's pass and email
	dbUser, err := cfg.dbQueries.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          params.Email,
		HashedPassword: hashed,
		ID:             userId,
	})

	// Send response
	respondWithJSON(w, http.StatusOK, makeUserResponseData(dbUser))
}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {
	// Get user's token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Validate token and get userId
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 403, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Get the chirpID
	chirpIDString := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDString)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Chirp not found: %v", err))
		return
	}

	// Get chirp info
	dbChirp, err := cfg.dbQueries.GetChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Chirp not found: %v", err))
		return
	}

	// Verify that the user created the chirp
	if userID != dbChirp.UserID {
		respondWithError(w, 403, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	// Delete chirp
	err = cfg.dbQueries.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Chirp not found: %v", err))
		return
	}

	//Chirp was deleted, send a confirmation response
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) upgradeUserHandler(w http.ResponseWriter, r *http.Request) {
	// Specify request shape
	type data struct {
		UserID string `json:"user_id"`
	}

	type parameters struct {
		Event string `json:"event"`
		Data  data   `json:"data"`
	}

	// Get request data
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error parsing request data: %v", err))
	}

	// Make sure the event is "user.upgraded"
	if params.Event != "user.upgraded" {
		// Return 204 because we don't care about any other event
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Parse user_id into uuid
	userID, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("Unable to parse userId(%s): %v", params.Data.UserID, err))
		return
	}

	// Upgrade user to chirpy red
	_, err = cfg.dbQueries.UpgradeUser(r.Context(), userID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, fmt.Sprintf("Unable to find userId(%v): %v", userID, err))
		return
	}

	// Succesfully Upgraded
	w.WriteHeader(http.StatusNoContent)
}

func makeUserResponseData(dbUser database.User) User {

	return User{
		ID:          dbUser.ID,
		CreatedAt:   dbUser.CreatedAt,
		UpdatedAt:   dbUser.UpdatedAt,
		Email:       dbUser.Email,
		IsChirpyRed: dbUser.IsChirpyRed,
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
	mux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeHandler)
	mux.HandleFunc("PUT /api/users", apiCfg.updateAccountHandler)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirpHandler)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.upgradeUserHandler)

	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("Error starting server: %v", "err")
		os.Exit(1)
	}
}
