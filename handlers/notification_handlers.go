package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"drive-backend/models"

	"github.com/gocql/gocql"
)

var cassandraSession *gocql.Session

func InitNotificationSession(session *gocql.Session) {
	cassandraSession = session
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

	notifications := []models.Notification{}

	if cassandraSession != nil {
		query := cassandraSession.Query(
			`SELECT id, user_id, message, read, created_at FROM notifications WHERE user_id = ?`,
			userID,
		).Consistency(gocql.One)

		iter := query.Iter()
		var n models.Notification
		var createdAt time.Time

		for iter.Scan(&n.ID, &n.UserID, &n.Message, &n.Read, &createdAt) {
			n.CreatedAt = createdAt.UTC()
			notifications = append(notifications, n)
			n = models.Notification{}
		}

		if err := iter.Close(); err != nil {
			log.Printf("Greška pri čitanju notifikacija iz Cassandre: %v", err)
			http.Error(w, "Greška pri čitanju notifikacija", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": notifications,
	})
}
