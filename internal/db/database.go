package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDB() *pgxpool.Pool {
	return InitFromEnv("DATABASE_URL")
}

func InitFromEnv(envVar string) *pgxpool.Pool {
	dsn := os.Getenv(envVar)
	if dsn == "" {
		log.Fatalf("%s environment variable is required", envVar)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Unable to parse %s: %v", envVar, err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Unable to connect to database (%s): %v", envVar, err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("Database ping failed (%s): %v", envVar, err)
	}

	fmt.Printf("Connected to Database (%s)\n", envVar)
	return pool
}
