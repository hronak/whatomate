package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zerodha/logf"
	"gorm.io/gorm"

	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/database"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
)

// migrateSchema brings the database up to date: AutoMigrate every model, create
// the indexes, ensure the default admin exists, then convert any legacy
// chatbot flow to a v2 graph.
//
// Shared by `server -migrate` and `install` so the two can never drift. Every
// step is idempotent, so re-running is a no-op.
func migrateSchema(db *gorm.DB, cfg *config.Config, lo logf.Logger, out io.Writer) error {
	if err := database.RunMigrationWithProgress(db, &cfg.DefaultAdmin, lo, out); err != nil {
		return err
	}
	// Backfill v2 graph for any legacy chatbot flow still on Steps[].
	// Idempotent — re-running is a no-op once every row is converted.
	if err := handlers.BackfillChatbotFlowGraph(db, lo); err != nil {
		return fmt.Errorf("chatbot flow graph backfill: %w", err)
	}
	return nil
}

// runInstall implements the `install` command: one shot to take an empty
// database to a running app, so a fresh clone doesn't need to know that the way
// to initialize is a flag on the server command.
func runInstall(args []string) {
	installFlags := flag.NewFlagSet("install", flag.ExitOnError)
	// Same default as `server` and `worker`. Development goes through the
	// Makefile, which passes -config dev/config.toml explicitly; a released
	// binary has no dev/ directory at all.
	configPath := installFlags.String("config", "config.toml", "Path to config file")
	idempotent := installFlags.Bool("idempotent", false, "Exit successfully if the database is already installed")
	yes := installFlags.Bool("yes", false, "Don't prompt before migrating a database that already has tables")
	seed := installFlags.Bool("seed", false, "Also insert demo contacts, tags and a starter chatbot flow")
	_ = installFlags.Parse(args)

	lo := logf.New(logf.Opts{
		EnableColor:     true,
		Level:           logf.InfoLevel,
		TimestampFormat: "2006-01-02 15:04:05",
		DefaultFields:   []any{"app", "whatomate"},
	})

	cfg, err := config.Load(*configPath)
	if err != nil {
		lo.Fatal("Failed to load config", "error", err, "path", *configPath)
	}

	db, err := database.NewPostgres(&cfg.Database, false)
	if err != nil {
		lo.Fatal("Failed to connect to database", "error", err,
			"host", cfg.Database.Host, "port", cfg.Database.Port, "name", cfg.Database.Name)
	}

	installed := db.Migrator().HasTable(&models.User{})

	switch {
	case installed && *idempotent:
		// The whole point of -idempotent: a provisioning script can call install
		// unconditionally without having to know whether this is a first run.
		fmt.Printf("Database %q is already installed. Nothing to do.\n", cfg.Database.Name)
		if *seed {
			// Seeding stays available on an existing install — it has its own
			// guard and is the only part someone might want to add later.
			break
		}
		return
	case installed && !*yes:
		fmt.Printf("Database %q already has Whatomate tables.\n", cfg.Database.Name)
		fmt.Println("Migrations are additive and idempotent, but this will write to an existing database.")
		if !confirm("Continue?") {
			fmt.Println("Aborted.")
			os.Exit(1)
		}
	}

	if err := migrateSchema(db, cfg, lo, os.Stdout); err != nil {
		lo.Fatal("Install failed", "error", err)
	}

	if *seed {
		summary, err := database.SeedDemoData(db)
		if err != nil {
			lo.Fatal("Seeding failed", "error", err)
		}
		fmt.Printf("  %s\n", summary)
	}

	fmt.Printf(`
  Whatomate is installed.

    Admin login   %s / %s
    Start it      make dev      (then open http://localhost:%d)

`, cfg.DefaultAdmin.Email, cfg.DefaultAdmin.Password, frontendPortHint())
}

// confirm asks a yes/no question on stdin. Anything that isn't an explicit yes
// is a no, and a closed stdin (CI, a pipe) reads as no rather than hanging.
func confirm(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// frontendPortHint reports the port `make dev` will serve the app on, so the
// closing message stays right when FRONTEND_PORT is overridden.
func frontendPortHint() int {
	if v := os.Getenv("FRONTEND_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil && p > 0 {
			return p
		}
	}
	return 3000
}
