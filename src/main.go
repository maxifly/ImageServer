package main

import (
	"context"
	appimageserver "imgserver/internal/appimgserver"
	"imgserver/internal/pkg/dbase"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	db, err := dbase.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Println("Database initialized successfully")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8099"
	}

	app, err := appimageserver.NewImgSrv(port, db)
	if err != nil {
		log.Printf("Failed to initialize server: %v", err)
		panic(err)
	}

	defer app.Stop()

	go func() {
		if err := app.Start(); err != nil {
			log.Printf("Server failed: %v", err)
			panic(err)
		}
	}()

	// Ждать сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Application started, waiting for shutdown signal...")
	sig := <-quit
	log.Printf("Received signal: %v, shutting down...", sig)

	// Graceful shutdown с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("Application stopped gracefully")
}
