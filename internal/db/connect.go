package db

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"os"
)

// Connect подключается к БД по DATABASE_URL (или env-переменным)
func Connect(defaultDB string) (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "5432"),
			getEnv("DB_USER", defaultDB),
			getEnv("DB_PASSWORD", defaultDB),
			getEnv("DB_NAME", defaultDB),
		)
	}
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err = database.Ping(); err != nil {
		return nil, err
	}
	return database, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
