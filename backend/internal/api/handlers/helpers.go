package handlers

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// credentialsFromSettings достаёт логин и пароль камеры из JSONB-поля settings.
func credentialsFromSettings(settings map[string]any) (string, string) {
	if settings == nil {
		return "", ""
	}
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	return username, password
}
