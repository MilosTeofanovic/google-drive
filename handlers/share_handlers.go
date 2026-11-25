package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ShareRequest struct {
	ItemID       string `json:"item_id"`
	ItemType     string `json:"item_type"`
	ItemName     string `json:"item_name"`
	TargetUserID string `json:"target_user_id"`
}

func ShareItemHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Dozvoljen je samo POST metod", http.StatusMethodNotAllowed)
		return
	}

	sharerID := r.Header.Get("X-User-ID")
	if sharerID == "" {
		sharerID = "1"
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Nevalidan JSON zahtev", http.StatusBadRequest)
		return
	}

	if req.TargetUserID == "" || req.ItemName == "" {
		http.Error(w, "Potrebni su target_user_id i item_name", http.StatusBadRequest)
		return
	}

	itemLabel := "stavku"
	if req.ItemType == "folder" {
		itemLabel = "folder"
	} else if req.ItemType == "file" {
		itemLabel = "datoteku"
	}

	msg := fmt.Sprintf("Korisnik %s je podelio sa vama %s: %s", sharerID, itemLabel, req.ItemName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	CreateNotification(ctx, req.TargetUserID, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message":      "Uspešno podeljeno i poslata je notifikacija",
		"target_user":  req.TargetUserID,
		"notification": msg,
	})
}
