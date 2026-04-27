package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/Neokil/AutoPR/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	portFlag := flag.Int("port", 0, "HTTP port override (default uses config server_port)")
	flag.Parse()

	err := server.Run(*portFlag)
	if err != nil {
		slog.Error("auto-prd", "err", err)
		os.Exit(1)
	}
}
