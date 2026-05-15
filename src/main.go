package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/mode"
	"github.com/apimgr/pastebin/src/paths"
	"github.com/apimgr/pastebin/src/scheduler"
	"github.com/apimgr/pastebin/src/server"
)

// Version, Commit, and BuildDate are injected at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

const appName = "pastebin"

func main() {
	var (
		portFlag     = flag.String("port", "", "server port (overrides config)")
		addressFlag  = flag.String("address", "", "listen address (overrides config)")
		modeFlag     = flag.String("mode", "", "application mode: dev|production")
		configPath   = flag.String("config", "", "path to config file (server.yml)")
		dataPath     = flag.String("data", "", "path to data directory")
		logsPath     = flag.String("logs", "", "path to logs directory")
		showVersion  = flag.Bool("version", false, "print version and exit")
		showStatus   = flag.Bool("status", false, "show runtime paths and exit")
		showHelp     = flag.Bool("help", false, "show help and exit")
		cleanExpired = flag.Bool("clean-expired", false, "delete expired pastes and exit")
		debugFlag    = flag.Bool("debug", false, "enable debug output")
	)
	flag.Parse()

	binaryName := filepath.Base(os.Args[0])

	if *showHelp {
		printHelp(binaryName)
		return
	}

	if *showVersion {
		fmt.Printf("%s %s\n", binaryName, Version)
		return
	}

	if err := mode.Initialize(*modeFlag); err != nil {
		log.Printf("warning: %v", err)
	}

	if *debugFlag {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("debug mode enabled")
	}

	// Resolve directories.
	configDir := paths.GetConfigDir(appName)
	dataDir := paths.GetDataDir(appName)
	if *dataPath != "" {
		dataDir = *dataPath
	}
	logsDir := paths.GetLogsDir(appName)
	if *logsPath != "" {
		logsDir = *logsPath
	}

	for _, dir := range []string{configDir, dataDir, logsDir} {
		if err := paths.EnsureDir(dir); err != nil {
			log.Printf("warning: could not create directory %s: %v", dir, err)
		}
	}

	if *showStatus {
		fmt.Printf("%s %s\n", binaryName, Version)
		fmt.Printf("Mode:   %s\n", mode.Get())
		fmt.Printf("Config: %s\n", filepath.Join(configDir, "server.yml"))
		fmt.Printf("Data:   %s\n", dataDir)
		fmt.Printf("Logs:   %s\n", logsDir)
		return
	}

	// Load config.
	cfgFile := filepath.Join(configDir, "server.yml")
	if *configPath != "" {
		cfgFile = *configPath
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Printf("warning: config load: %v", err)
	}

	// Resolve database path.
	if cfg.Database.Path == "" {
		cfg.Database.Path = filepath.Join(dataDir, "db", "server.db")
	}
	if err := paths.EnsureDir(filepath.Dir(cfg.Database.Path)); err != nil {
		log.Printf("warning: db dir: %v", err)
	}

	// Open database.
	db, err := database.NewDatabase(cfg.Database.Type, cfg.Database.Path)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	log.Printf("%s %s — database: %s (%s)", appName, Version, db.Type(), cfg.Database.Path)

	// One-shot: clean expired pastes then exit.
	if *cleanExpired {
		n, err := db.DeleteExpiredPastes()
		if err != nil {
			log.Fatalf("clean expired: %v", err)
		}
		b, _ := db.DeleteBurnedPastes()
		log.Printf("deleted %d expired + %d burned pastes", n, b)
		return
	}

	// CLI flag overrides.
	if *portFlag != "" {
		cfg.Server.Port = *portFlag
	}
	if *addressFlag != "" {
		cfg.Server.Address = *addressFlag
	}

	// Start background scheduler.
	sched := scheduler.New()
	sched.AddTask("expire-pastes", 10*time.Minute, func() error {
		n, err := db.DeleteExpiredPastes()
		if err != nil {
			return err
		}
		b, _ := db.DeleteBurnedPastes()
		if n+b > 0 {
			log.Printf("scheduler: removed %d expired + %d burned pastes", n, b)
		}
		return nil
	})
	sched.Start()
	defer sched.Stop()

	// Build and start HTTP server.
	srv := server.New(db, cfg, Version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("shutting down…")
		cancel()
	}()

	addr := cfg.Server.Address + ":" + cfg.Server.Port
	log.Printf("listening on %s", addr)

	if err := srv.Run(ctx, addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func printHelp(name string) {
	fmt.Printf(`%s — a fast, public pastebin service

USAGE
    %s [OPTIONS]

OPTIONS
    --port <PORT>        server port (default from config)
    --address <ADDR>     listen address (default from config)
    --mode <MODE>        dev | production
    --config <PATH>      path to server.yml
    --data <PATH>        path to data directory
    --logs <PATH>        path to logs directory
    --clean-expired      delete expired/burned pastes and exit
    --status             show runtime paths and exit
    --version            print version and exit
    --debug              enable debug logging
    --help               show this help

EXAMPLES
    %s                              run with defaults
    %s --port 8080 --mode dev       development mode on port 8080
    %s --clean-expired              one-shot cleanup
    %s --status                     inspect runtime paths

`, name, name, name, name, name, name)
}
