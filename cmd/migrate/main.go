package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed all:migrations
var migrations embed.FS

func main() {
	action := flag.String("action", "up", "up, down, or version")
	flag.Parse()

	cfg := config.New(config.DefaultPath)
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		fail(err.Error())
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, cfg.Database.DSN)
	if err != nil {
		fail(err.Error())
	}
	defer m.Close()

	switch *action {
	case "up":
		from := currentVersion(m)
		pending := pendingUp(src, from)
		if len(pending) > 0 {
			fmt.Printf("current version=%d, applying %d migration(s):\n", from, len(pending))
			for _, item := range pending {
				fmt.Printf("  -> %s\n", item)
			}
		}
		err = m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			fmt.Printf("already at version %d, no pending migrations\n", from)
			return
		}
		if err != nil {
			fail(err.Error())
		}
		fmt.Printf("migrated to version=%d\n", currentVersion(m))
		return
	case "down":
		from := currentVersion(m)
		fmt.Printf("current version=%d, rolling back 1 migration\n", from)
		err = m.Steps(-1)
	case "version":
		version, dirty, versionErr := m.Version()
		if versionErr != nil {
			fail(versionErr.Error())
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return
	default:
		fail("unknown action")
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fail(err.Error())
	}
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return
	}
	fmt.Printf("now at version=%d\n", currentVersion(m))
}

func currentVersion(m *migrate.Migrate) uint {
	version, _, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0
	}
	if err != nil {
		fail(err.Error())
	}
	return version
}

func pendingUp(src source.Driver, from uint) []string {
	var (
		version uint
		err     error
	)
	if from == 0 {
		version, err = src.First()
	} else {
		version, err = src.Next(from)
	}
	if err != nil {
		return nil
	}
	var pending []string
	for {
		pending = append(pending, formatMigration(src, version))
		next, nextErr := src.Next(version)
		if nextErr != nil {
			break
		}
		version = next
	}
	return pending
}

func formatMigration(src source.Driver, version uint) string {
	readCloser, identifier, err := src.ReadUp(version)
	if readCloser != nil {
		_, _ = io.Copy(io.Discard, readCloser)
		_ = readCloser.Close()
	}
	if err != nil || identifier == "" {
		return fmt.Sprintf("%d", version)
	}
	return fmt.Sprintf("%d %s", version, identifier)
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
