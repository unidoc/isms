package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"isms.sh/internal/isms/api"
	"isms.sh/internal/isms/db"
)

func serveCmd() *cobra.Command {
	var (
		addr   string
		webDir string
		dev    bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the ISMS web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := os.Getenv("DATABASE_URL")
			if dbURL == "" {
				return fmt.Errorf("DATABASE_URL is required. Set it in your environment.")
			}

			// Signal-cancelled context: SIGINT/SIGTERM trigger graceful shutdown.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			database, err := db.New(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("database: %w", err)
			}
			defer database.Close()

			// Auto-run migrations using embedded SQL files.
			log.Println("Running database migrations...")
			if err := database.MigrateFS(ctx, embeddedMigrations, "migrations"); err != nil {
				return fmt.Errorf("auto-migration failed: %w", err)
			}

			// Web dir: flag > env > auto-detect
			wd := webDir
			if wd == "" {
				wd = os.Getenv("ISMS_WEB_DIR")
			}

			// Set dev mode env for auth middleware
			if dev {
				os.Setenv("ISMS_DEV_MODE", "1")
				return serveDev(ctx, addr, wd, database)
			}

			// Use embedded web if no disk webDir
			var embFS fs.FS
			if wd == "" {
				embFS = embeddedWebFS()
			}
			srv := api.NewWithFS(addr, wd, database, embFS)
			return srv.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address")
	cmd.Flags().StringVar(&webDir, "web-dir", "", "Path to Vue dist/ directory (env: ISMS_WEB_DIR)")
	cmd.Flags().BoolVar(&dev, "dev", false, "Dev mode: start Go API + Vite dev server together")
	return cmd
}

// serveDev starts Go API on the given addr and Vite dev server as a subprocess.
// Vite proxies /api/* to Go. User opens Vite's URL for hot reload.
func serveDev(ctx context.Context, addr, webDir string, database *db.DB) error {
	// Find web/ directory: use --web-dir (strip /dist suffix), or auto-detect.
	webPath := ""
	if webDir != "" {
		// --web-dir might point to dist/, go up to web/
		candidate := webDir
		if filepath.Base(candidate) == "dist" {
			candidate = filepath.Dir(candidate)
		}
		pkg := filepath.Join(candidate, "package.json")
		if _, err := os.Stat(pkg); err == nil {
			webPath, _ = filepath.Abs(candidate)
		}
	}
	if webPath == "" {
		for _, candidate := range []string{"web", "../web"} {
			pkg := filepath.Join(candidate, "package.json")
			if _, err := os.Stat(pkg); err == nil {
				webPath, _ = filepath.Abs(candidate)
				break
			}
		}
	}
	if webPath == "" {
		return fmt.Errorf("web/ directory not found. Use --web-dir to point to the web/ source directory")
	}

	// Start Vite dev server as subprocess
	vite := exec.Command("npx", "vite", "--host")
	vite.Dir = webPath
	vite.Stdout = os.Stdout
	vite.Stderr = os.Stderr
	vite.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := vite.Start(); err != nil {
		return fmt.Errorf("starting Vite: %w", err)
	}

	log.Printf("Vite dev server started (pid %d)", vite.Process.Pid)
	log.Printf("Open http://localhost:5173 for hot-reload dev UI")
	log.Printf("Go API on %s", addr)

	// Start Go API (this blocks until ctx is cancelled — SIGINT/SIGTERM)
	srv := api.New(addr, "", database)
	err := srv.Start(ctx)

	// Cleanup Vite on exit
	if vite.Process != nil {
		syscall.Kill(-vite.Process.Pid, syscall.SIGTERM)
	}

	return err
}
