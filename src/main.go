package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/mode"
	"github.com/apimgr/pastebin/src/paths"
	"github.com/apimgr/pastebin/src/server"
)

const appName = "pastebin"

func main() {
	serviceMode := flag.Bool("service", false, "Run in service mode (production)")
	maintenanceMode := flag.Bool("maintenance", false, "Run in maintenance mode")
	showVersion := flag.Bool("version", false, "Show version information")
	cleanExpired := flag.Bool("clean-expired", false, "Clean up expired pastes")
	portFlag := flag.String("port", "", "Server port (overrides config)")
	addressFlag := flag.String("address", "", "Listen address (overrides config)")
	modeFlag := flag.String("mode", "", "Application mode (dev/development, prod/production)")
	updateCmd := flag.String("update", "", "Update command (stable, beta, nightly)")
	configPath := flag.String("config", "", "Path to configuration file")
	dataPath := flag.String("data", "", "Path to data directory")
	logsPath := flag.String("logs", "", "Path to logs directory")
	showStatus := flag.Bool("status", false, "Show service status")
	showHelp := flag.Bool("help", false, "Show help information")
	flag.Parse()

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	if *showStatus {
		printStatus()
		os.Exit(0)
	}

	// Handle update command
	if *updateCmd != "" {
		handleUpdateCommand(*updateCmd)
		os.Exit(0)
	}

	// Initialize mode
	if err := mode.Initialize(*modeFlag); err != nil {
		log.Printf("Warning: invalid mode: %v", err)
	}

	if *showVersion {
		fmt.Println("pastebin v1.0.0")
		fmt.Println("A pastebin service with multi-database support")
		os.Exit(0)
	}

	// Get config directory
	configDir := paths.GetConfigDir(appName)
	if err := paths.EnsureDir(configDir); err != nil {
		log.Printf("Warning: could not create config directory: %v", err)
	}

	// Get data directory
	dataDir := paths.GetDataDir(appName)
	if *dataPath != "" {
		dataDir = *dataPath
	}
	if err := paths.EnsureDir(dataDir); err != nil {
		log.Printf("Warning: could not create data directory: %v", err)
	}

	// Get logs directory
	logsDir := paths.GetLogsDir(appName)
	if *logsPath != "" {
		logsDir = *logsPath
	}
	if err := paths.EnsureDir(logsDir); err != nil {
		log.Printf("Warning: could not create logs directory: %v", err)
	}
	_ = logsDir // Use logs directory for log files

	// Load configuration
	cfgPath := filepath.Join(configDir, "server.yml")
	if *configPath != "" {
		cfgPath = *configPath
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
	}

	// Set default database path if using SQLite and path is relative
	if cfg.Database.Type == "sqlite" && !filepath.IsAbs(cfg.Database.Path) {
		cfg.Database.Path = filepath.Join(dataDir, "pastebin.db")
	}

	// Connect to database
	log.Printf("Connecting to %s database...", cfg.Database.Type)
	db, err := database.NewDatabase(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Printf("Connected to %s database", db.Type())

	// Handle maintenance mode
	if *maintenanceMode || *cleanExpired {
		count, err := db.DeleteExpiredPastes()
		if err != nil {
			log.Fatalf("Failed to clean expired pastes: %v", err)
		}
		log.Printf("Deleted %d expired pastes", count)
		os.Exit(0)
	}

	// Build info
	version := "1.0.0"

	// Create and start server
	srv := server.New(db, cfg, version)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		cancel()
	}()

	// Override config with CLI flags if provided
	if *portFlag != "" {
		cfg.Server.Port = *portFlag
	}
	if *addressFlag != "" {
		cfg.Server.Address = *addressFlag
	}

	addr := cfg.Server.Address + ":" + cfg.Server.Port
	if *serviceMode {
		log.Printf("Starting pastebin server in service mode on %s", addr)
	} else {
		log.Printf("Starting pastebin server on %s", addr)
		log.Printf("Database: %s", db.Type())
		log.Printf("")
		log.Printf("URLs:")
		log.Printf("  Web interface: http://%s/", addr)
		log.Printf("  API docs: http://%s/api", addr)
		log.Printf("  Health check: http://%s/healthz", addr)
		log.Printf("")
		log.Printf("Usage examples:")
		log.Printf("  Upload text: curl -X POST --data-binary @file.txt http://%s/create", addr)
		log.Printf("  Upload file: curl -X POST -F \"files=@file.txt\" http://%s/create", addr)
		log.Printf("  Upload JSON: curl -H \"Content-Type: application/json\" -d '{\"content\":\"hello\"}' http://%s/api/v1/create", addr)
	}

	if err := srv.Run(ctx, addr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func printHelp() {
	fmt.Printf(`%s - A pastebin service with multi-database support

USAGE:
    %s [OPTIONS]

OPTIONS:
    --service            Run in service mode (production)
    --maintenance        Run in maintenance mode (cleanup)
    --clean-expired      Clean up expired pastes
    --mode <MODE>        Application mode (dev, prod)
    --update <BRANCH>    Update from branch (stable, beta, nightly)
    --config <PATH>      Path to configuration file
    --data <PATH>        Path to data directory
    --logs <PATH>        Path to logs directory
    --port <PORT>        Server port (overrides config)
    --address <ADDR>     Listen address (overrides config)
    --status             Show service status
    --version            Show version information
    --help               Show this help message

EXAMPLES:
    %s --service                    Run in production mode
    %s --mode dev --port 8080       Run in development mode on port 8080
    %s --maintenance                Clean expired pastes
    %s --update stable              Update to latest stable version

`, appName, appName, appName, appName, appName, appName)
}

func printStatus() {
	fmt.Printf("%s Status\n", appName)
	fmt.Println("================")
	fmt.Printf("Mode:     %s\n", mode.Get())
	fmt.Printf("Config:   %s\n", filepath.Join(paths.GetConfigDir(appName), "server.yml"))
	fmt.Printf("Data:     %s\n", paths.GetDataDir(appName))
	fmt.Printf("Logs:     %s\n", paths.GetLogsDir(appName))
}

func handleUpdateCommand(branch string) {
	validBranches := map[string]bool{
		"stable":  true,
		"beta":    true,
		"nightly": true,
	}

	if !validBranches[branch] {
		fmt.Printf("Error: invalid update branch %q (valid: stable, beta, nightly)\n", branch)
		os.Exit(1)
	}

	fmt.Printf("Updating %s from %s branch...\n", appName, branch)

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Println("Error: git is not installed")
		os.Exit(1)
	}

	// Perform update
	cmd := exec.Command("git", "pull", "origin", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Update failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Update complete. Please rebuild the application.")
}
