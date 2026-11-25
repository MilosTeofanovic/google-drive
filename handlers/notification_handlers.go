package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"drive-backend/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var notificationCollection *mongo.Collection

func InitNotificationCollection(collection *mongo.Collection) {
	notificationCollection = collection
}

func GetNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Dozvoljen je samo GET metod", http.StatusMethodNotAllowed)
		return
	}

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Neautorizovan pristup", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var notifications []models.Notification
	if notificationCollection != nil {
		opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
		cursor, err := notificationCollection.Find(ctx, bson.M{"user_id": userID}, opts)
		if err != nil {
			http.Error(w, "Greška pri čitanju notifikacija", http.StatusInternalServerError)
			return
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &notifications); err != nil {
			http.Error(w, "Greška pri obradi notifikacija", http.StatusInternalServerError)
			return
		}
	}

	if notifications == nil {
		notifications = []models.Notification{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": notifications,
	})
}
