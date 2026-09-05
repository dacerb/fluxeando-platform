package main

import (
	"context"
	"flag"
	"github.com/cashflow/desktop/api/internal/application"
	"github.com/cashflow/desktop/api/internal/httpapi"
	"github.com/cashflow/desktop/api/internal/infrastructure/sqlite"
	stdlog "log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	dbPath := flag.String("db", "cashflow.db", "SQLite database path")
	mysqlHost := flag.String("mysql-host", "", "MySQL host")
	mysqlPort := flag.String("mysql-port", "3306", "MySQL port")
	mysqlDatabase := flag.String("mysql-database", "", "MySQL database")
	mysqlUsername := flag.String("mysql-username", "", "MySQL username")
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	validate := flag.Bool("validate", false, "validate a SQLite database without changing it")
	flag.Parse()
	if *validate {
		if e := sqlite.Validate(*dbPath); e != nil {
			stdlog.Printf("database validation failed: %v", e)
			os.Exit(1)
		}
		return
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var repo *sqlite.Repository
	var e error
	if *mysqlHost != "" || *mysqlDatabase != "" || *mysqlUsername != "" {
		if *mysqlHost == "" || *mysqlDatabase == "" || *mysqlUsername == "" {
			stdlog.Print("MySQL configuration is incomplete")
			os.Exit(1)
		}
		password := os.Getenv("CASHFLOW_MYSQL_PASSWORD")
		if passwordFile := os.Getenv("CASHFLOW_MYSQL_PASSWORD_FILE"); passwordFile != "" {
			contents, readErr := os.ReadFile(passwordFile)
			if readErr != nil {
				stdlog.Printf("MySQL password file could not be read: %v", readErr)
				os.Exit(1)
			}
			password = strings.TrimSpace(string(contents))
		}
		if password == "" {
			stdlog.Print("MySQL password is missing")
			os.Exit(1)
		}
		repo, e = sqlite.OpenMySQL(*mysqlHost, *mysqlPort, *mysqlDatabase, *mysqlUsername, password)
	} else {
		repo, e = sqlite.Open(*dbPath)
	}
	if e != nil {
		log.Error("database startup failed", "level", "critical", "component", "api", "layer", "infrastructure", "error", strings.ReplaceAll(e.Error(), os.Getenv("CASHFLOW_MYSQL_PASSWORD"), "[redacted]"))
		os.Exit(1)
	}
	defer repo.Close()
	backups := application.NewBackupManager(repo, os.Getenv("CASHFLOW_BACKUP_ROOT"), os.Getenv("CASHFLOW_GOOGLE_DRIVE_ACCESS_TOKEN"), os.Getenv("CASHFLOW_GOOGLE_OAUTH_CLIENT_ID"), os.Getenv("CASHFLOW_GOOGLE_OAUTH_CLIENT_SECRET"), os.Getenv("CASHFLOW_GOOGLE_OAUTH_REDIRECT_URL"), os.Getenv("CASHFLOW_BACKUP_ENCRYPTION_KEY"))
	app := application.New(repo, backups)
	server := &http.Server{Addr: *addr, Handler: httpapi.New(app, log).Handler()}
	log.Info("cashflow api started", "component", "api", "layer", "bootstrap", "operation", "start", "address", *addr)
	go func() {
		if err := backups.RunOnStartup(context.Background()); err != nil {
			log.Error("startup backup failed", "component", "backup", "operation", "startup", "error", err.Error())
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := backups.RunOnShutdown(ctx); err != nil {
			log.Error("shutdown backup failed", "component", "backup", "operation", "shutdown", "error", err.Error())
		}
		if err := server.Shutdown(ctx); err != nil {
			log.Error("api shutdown failed", "component", "api", "layer", "bootstrap", "error", err.Error())
		}
	}()
	if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Error("api stopped", "level", "critical", "component", "api", "layer", "bootstrap", "error", e.Error())
	}
}
