package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Embede todo lo que esté dentro de desktop/appassets (recursivo)
//
//go:embed all:appassets
var embedded embed.FS

func init() {
	f, err := os.OpenFile("zhatbot.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
	log.Println("=== zhatBot starting ===")
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC: %v\n%s", r, debug.Stack())
			os.Exit(2)
		}
	}()

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("ZHATBOT_MODE")))
	devMode := mode == "development"
	if devMode {
		log.Println("Running desktop in development mode")
	} else {
		log.Println("Running desktop in production mode")
	}

	assetsFS, err := fs.Sub(embedded, "appassets")
	if err != nil {
		log.Fatalf("fs.Sub(appassets) failed: %v", err)
	}
	if _, errApp := fs.Stat(assetsFS, "_app"); errApp != nil {
		log.Printf("assets missing _app/: %v", errApp)
	} else {
		log.Printf("assets OK: _app/ present")
	}

	app := NewApp()

	debugOptions := options.Debug{}
	if devMode {
		debugOptions.OpenInspectorOnStartup = true
	}

	err = wails.Run(&options.App{
		Title:                    "zhatBot",
		Logger:                   logger.NewDefaultLogger(),
		Debug:                    debugOptions,
		EnableDefaultContextMenu: devMode,
		AssetServer: &assetserver.Options{
			Assets: assetsFS,
		},
		Bind:       []any{app},
		OnStartup:  app.OnStartup,
		OnShutdown: app.OnShutdown,
	})
	if err != nil {
		log.Printf("wails.Run error: %v\n%s", err, debug.Stack())
		os.Exit(1)
	}
}
