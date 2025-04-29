package ratelimit

import (
	"encoding/json"
	"go-cloud-camp-2025-test-assignment/config"
	"go-cloud-camp-2025-test-assignment/internal/storage"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"
)

type ClientManager struct {
	storage       storage.Storage
	rateLimiter   *TokenBucketRateLimiter
	defaultConfig config.TokenBucketConfig
}

type ClientConfigRequest struct {
	ClientID   string `json:"client_id"`
	Capacity   int    `json:"capacity"`
	RefillRate int    `json:"refill_rate"`
}

type ClientConfigResponse struct {
	ClientID   string `json:"client_id"`
	Capacity   int    `json:"capacity"`
	RefillRate int    `json:"refill_rate"`
}

type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewClientManager(store storage.Storage, limiter *TokenBucketRateLimiter, cfg *config.RateLimitConfig) *ClientManager {
	return &ClientManager{
		storage:       store,
		rateLimiter:   limiter,
		defaultConfig: cfg.Default,
	}
}

func (cm *ClientManager) HandleAddClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req ClientConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("Failed to decode client config request")
		sendErrorResponse(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.ClientID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	if req.Capacity <= 0 {
		req.Capacity = cm.defaultConfig.Capacity
	}
	if req.RefillRate <= 0 {
		req.RefillRate = cm.defaultConfig.RefillRate
	}

	if err := cm.rateLimiter.UpdateClientConfig(r.Context(), req.ClientID, req.Capacity, req.RefillRate); err != nil {
		log.Error().Err(err).Str("client_id", req.ClientID).Msg("Failed to update client config")
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to update client configuration")
		return
	}

	resp := ClientConfigResponse{
		ClientID:   req.ClientID,
		Capacity:   req.Capacity,
		RefillRate: req.RefillRate,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("Failed to encode response")
	}

	log.Info().
		Str("client_id", req.ClientID).
		Int("capacity", req.Capacity).
		Int("refill_rate", req.RefillRate).
		Msg("Client configuration updated")
}

func (cm *ClientManager) HandleGetClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	capacity, refillRate, err := cm.storage.GetClientConfig(r.Context(), clientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to get client config")
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to get client configuration")
		return
	}

	if capacity == 0 || refillRate == 0 {
		capacity = cm.defaultConfig.Capacity
		refillRate = cm.defaultConfig.RefillRate
	}

	resp := ClientConfigResponse{
		ClientID:   clientID,
		Capacity:   capacity,
		RefillRate: refillRate,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("Failed to encode response")
	}
}

func (cm *ClientManager) HandleDeleteClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Client ID is required")
		return
	}

	if err := cm.storage.SetClientConfig(r.Context(), clientID, 0, 0); err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to delete client config")
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to delete client configuration")
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.Info().Str("client_id", clientID).Msg("Client configuration deleted")
}

func (cm *ClientManager) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/clients", cm.HandleCRUD)
}

func (cm *ClientManager) HandleCRUD(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		cm.HandleAddClient(w, r)
	case http.MethodGet:
		cm.HandleGetClient(w, r)
	case http.MethodDelete:
		cm.HandleDeleteClient(w, r)
	default:
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (cm *ClientManager) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {

		clientID = getClientIP(r)
	}

	capacity, refillRate, err := cm.storage.GetClientConfig(r.Context(), clientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to get client config")
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to get client status")
		return
	}

	if capacity == 0 || refillRate == 0 {
		capacity = cm.defaultConfig.Capacity
		refillRate = cm.defaultConfig.RefillRate
	}

	_, remaining, err := cm.rateLimiter.Allow(r.Context(), clientID, 0)
	if err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to get tokens remaining")
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to get tokens remaining")
		return
	}

	type ClientStatus struct {
		ClientID         string `json:"client_id"`
		Capacity         int    `json:"capacity"`
		RefillRate       int    `json:"refill_rate"`
		TokensRemaining  int    `json:"tokens_remaining"`
		TokensPercentage int    `json:"tokens_percentage"`
	}

	status := ClientStatus{
		ClientID:         clientID,
		Capacity:         capacity,
		RefillRate:       refillRate,
		TokensRemaining:  remaining,
		TokensPercentage: int(float64(remaining) / float64(capacity) * 100),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Error().Err(err).Msg("Failed to encode response")
	}
}

func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{
		Code:    statusCode,
		Message: message,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error().Err(err).Msg("Failed to encode error response")
	}
}

func getClientIP(r *http.Request) string {

	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		return forwardedFor
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return remoteIP
}
