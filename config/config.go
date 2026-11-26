package config

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ServerPort        string
	MongoURI          string
	MongoDBName       string
	FoldersColName    string
	FilesColName      string
	UsersColName      string
	UploadDir         string
	MaxUploadBytes    int64
	DefaultUserQuota  int64
	UsersServiceURL   string
	FilesServiceURL   string
	CassandraHosts    []string
	CassandraKeyspace string
}

func Load() Config {
	loadDotEnv(".env")

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("Nedostaje MONGO_URI environment promenljiva (ili u .env fajlu)")
	}

	port := getEnv("SERVER_PORT", "8080")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	maxUploadMB := getEnvAsInt64("MAX_UPLOAD_MB", 50)
	defaultQuotaMB := getEnvAsInt64("DEFAULT_USER_QUOTA_MB", 100)

	cassandraHostsRaw := getEnv("CASSANDRA_HOSTS", "127.0.0.1")
	var cassandraHosts []string
	for _, h := range strings.Split(cassandraHostsRaw, ",") {
		trimmed := strings.TrimSpace(h)
		if trimmed != "" {
			cassandraHosts = append(cassandraHosts, trimmed)
		}
	}
	if len(cassandraHosts) == 0 {
		cassandraHosts = []string{"127.0.0.1"}
	}

	return Config{
		ServerPort:        port,
		MongoURI:          mongoURI,
		MongoDBName:       getEnv("MONGO_DB_NAME", "google_drive"),
		FoldersColName:    getEnv("MONGO_FOLDERS_COLLECTION", "folders"),
		FilesColName:      getEnv("MONGO_FILES_COLLECTION", "files"),
		UsersColName:      getEnv("MONGO_USERS_COLLECTION", "users"),
		UploadDir:         getEnv("UPLOAD_DIR", "./uploads"),
		MaxUploadBytes:    maxUploadMB << 20,
		DefaultUserQuota:  defaultQuotaMB * 1024 * 1024,
		UsersServiceURL:   getEnv("USERS_SERVICE_URL", "http://localhost"+port),
		FilesServiceURL:   getEnv("FILES_SERVICE_URL", "http://localhost"+port),
		CassandraHosts:    cassandraHosts,
		CassandraKeyspace: getEnv("CASSANDRA_KEYSPACE", "drive_notifications"),
	}
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt64(key string, fallback int64) int64 {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
