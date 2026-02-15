package db
import (
     "context"
     "fmt"
     "os"
     "github.com/jackc/pgx/v5/pgxpool"
)
func InitDB() *pgxpool.Pool {
     dsn := os.Getenv("DATABASE_URL")
     if dsn == "" { dsn = "postgres://admin:password123@localhost:5434/fantasy_db?sslmode=disable" }
     config, _ := pgxpool.ParseConfig(dsn)
     pool, _ := pgxpool.NewWithConfig(context.Background(), config)
     fmt.Println("Connected to Database")
     return pool
}
