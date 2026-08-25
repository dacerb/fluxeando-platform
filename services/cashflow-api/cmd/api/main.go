package main

import (
	"flag"
	"github.com/cashflow/desktop/api/internal/application"
	"github.com/cashflow/desktop/api/internal/httpapi"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	"log"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	dbPath := flag.String("db", "cashflow.db", "SQLite database path")
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	validate := flag.Bool("validate", false, "validate a SQLite database without changing it")
	flag.Parse()
	if *validate {
		if e := sqlite.Validate(*dbPath); e != nil {
			log.Printf("database validation failed: %v", e)
			os.Exit(1)
		}
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	repo, e := sqlite.Open(*dbPath)
	if e != nil {
		log.Error("database startup failed", "level", "critical", "component", "api", "layer", "infrastructure", "error", e.Error())
		os.Exit(1)
	}
	defer repo.Close()
	app := application.New(repo)
	server := httpapi.New(app, log)
	log.Info("cashflow api started", "component", "api", "layer", "bootstrap", "operation", "start", "address", *addr)
	if e := http.ListenAndServe(*addr, server.Handler()); e != nil {
		log.Error("api stopped", "level", "critical", "component", "api", "layer", "bootstrap", "error", e.Error())
	}
}
