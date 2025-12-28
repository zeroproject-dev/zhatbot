package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	appruntime "zhatBot/internal/app/runtime"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	run, err := appruntime.Start(ctx, appruntime.Options{})
	if err != nil {
		log.Fatalf("no se pudo iniciar el runtime: %v", err)
	}

	log.Println("Iniciando bot...")

	<-ctx.Done()

	if err := run.Stop(); err != nil {
		log.Printf("error al apagar runtime: %v", err)
	}
	log.Println("Bot apagado.")
}
