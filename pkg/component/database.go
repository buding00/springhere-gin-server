package component

import (
	"context"
	"fmt"
	"time"

	"github.com/buding00/springhere-gin-server/pkg/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenPostgres(cfg config.DatabaseConfig) (*gorm.DB, func()) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		panic(fmt.Errorf("initialize PostgreSQL: open database: %w", err))
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(fmt.Errorf("initialize PostgreSQL: get database handle: %w", err))
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		panic(fmt.Errorf("initialize PostgreSQL: ping database: %w", err))
	}
	return db, func() { _ = sqlDB.Close() }
}

func PingPostgres(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get PostgreSQL handle: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}
