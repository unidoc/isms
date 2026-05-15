package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"isms.sh/internal/isms/db"
	"github.com/spf13/cobra"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

func migrateCmd() *cobra.Command {
	var migrationsDir string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDBURL()
			if dbURL == "" {
				return fmt.Errorf("DATABASE_URL is required. Set it in your environment or env file")
			}

			ctx := context.Background()
			d, err := db.New(ctx, dbURL)
			if err != nil {
				return err
			}
			defer d.Close()

			fmt.Println("Running migrations...")
			if migrationsDir != "" {
				// Use explicit directory from disk
				return d.Migrate(ctx, migrationsDir)
			}
			// Use embedded migrations (baked into binary)
			return d.MigrateFS(ctx, embeddedMigrations, "migrations")
		},
	}

	cmd.Flags().StringVar(&migrationsDir, "dir", "", "Migrations directory on disk (default: use embedded)")
	return cmd
}

// getDBURL returns the database URL from env.
func getDBURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return ""
}
