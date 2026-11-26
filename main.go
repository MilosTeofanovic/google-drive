package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"drive-backend/config"
	"drive-backend/handlers"
	"drive-backend/middleware"
	"drive-backend/services"

	"github.com/gocql/gocql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	// 1. Povezivanje sa MongoDB bazom
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(cfg.MongoURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Greška pri povezivanju sa MongoDB: %v", err)
	}

	// Provera konekcije
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("MongoDB nije dostupan: %v", err)
	}
	fmt.Println("Uspešno povezani sa MongoDB bazom!")

	// 2. Povezivanje sa Apache Cassandra bazom za notifikacije
	cluster := gocql.NewCluster(cfg.CassandraHosts...)
	cluster.Keyspace = cfg.CassandraKeyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 10 * time.Second

	cassandraSession, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("Greška pri povezivanju sa Cassandrom: %v", err)
	}
	defer cassandraSession.Close()
	fmt.Println("Uspešno povezani sa Apache Cassandra bazom!")

	// Inicijalizacija kolekcija, sesija i konfiguracije handlera
	db := client.Database(cfg.MongoDBName)
	handlers.InitEmailSender(services.NewEmailSender())
	handlers.InitFolderCollection(db.Collection(cfg.FoldersColName))
	handlers.InitFileCollection(db.Collection(cfg.FilesColName))
	handlers.InitUserCollection(db.Collection(cfg.UsersColName))
	handlers.InitNotificationSession(cassandraSession)
	handlers.InitUploadConfig(cfg.UploadDir, cfg.MaxUploadBytes)
	handlers.InitDefaultQuota(cfg.DefaultUserQuota)

	// Inicijalizacija sinhronih klijenata između servisa
	usersClient := services.NewUsersClient(cfg.UsersServiceURL)
	filesClient := services.NewFilesClient(cfg.FilesServiceURL)
	handlers.InitSyncClients(usersClient, filesClient)

	// 3. Registracija ruta
	http.HandleFunc("/api/login/verify-otps", handlers.VerifyOTPSHandler)
	http.HandleFunc("/api/password/change", middleware.AuthMiddleware(handlers.ChangePasswordHandler))
	http.HandleFunc("/api/password/forgot", handlers.ForgotPasswordHandler)
	http.HandleFunc("/api/password/reset", handlers.ResetPasswordHandler)
	http.HandleFunc("/api/folders", middleware.AuthMiddleware(handlers.CreateFolderHandler))
	http.HandleFunc("/api/files/upload", middleware.AuthMiddleware(handlers.UploadFileHandler))
	http.HandleFunc("/api/files/version", middleware.AuthMiddleware(handlers.UploadNewFileVersionHandler))
	http.HandleFunc("/api/files/open", handlers.OpenFileHandler)
	http.HandleFunc("/api/folders/content", middleware.AuthMiddleware(handlers.GetFolderContentHandler))
	http.HandleFunc("/api/items/rename", middleware.AuthMiddleware(handlers.RenameItemHandler))
	http.HandleFunc("/api/notifications", middleware.AuthMiddleware(handlers.GetNotificationsHandler))
	http.HandleFunc("/api/quota", middleware.AuthMiddleware(handlers.GetQuotaInfoHandler))
	http.HandleFunc("/api/register", handlers.RegisterHandler)
	http.HandleFunc("/api/login", handlers.LoginHandler)

	// Interni endpointi za sinhronu komunikaciju između servisa
	http.HandleFunc("/api/internal/users/", handlers.InternalUserRouteHandler)
	http.HandleFunc("/api/internal/files/used", handlers.InternalUsedStorageHandler)

	// Serviranje statičkih fajlova (CSS)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Ruta koja učitava HTML šablon
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	fmt.Printf("Server je pokrenut na portu %s...\n", cfg.ServerPort)
	err = http.ListenAndServe(cfg.ServerPort, nil)
	if err != nil {
		fmt.Printf("Greška pri pokretanju servera: %v\n", err)
	}
}
