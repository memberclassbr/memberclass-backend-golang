package http

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type SuccessResponse struct {
	Data interface{} `json:"data,omitempty"`
}

func WriteError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{
		Message: message,
	})
}

func WriteErrorWithData(w http.ResponseWriter, message string, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{
		Message: message,
		Data:    data,
	})
}

func WriteSuccess(w http.ResponseWriter, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := SuccessResponse{
		Data: data,
	}

	json.NewEncoder(w).Encode(response)
}

func WriteSuccessWithMessage(w http.ResponseWriter, message string, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := SuccessResponse{
		Data: data,
	}

	json.NewEncoder(w).Encode(response)
}
