package utils

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

func JSON(w http.ResponseWriter, status int, payload APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Success(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Message: message, Data: data})
}

func SuccessMeta(w http.ResponseWriter, message string, data interface{}, meta interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Message: message, Data: data, Meta: meta})
}

func Created(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusCreated, APIResponse{Success: true, Message: message, Data: data})
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, APIResponse{Success: false, Message: message})
}
