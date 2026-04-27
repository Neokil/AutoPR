package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/Neokil/AutoPR/internal/server"
)

func main() {
	portFlag := flag.Int("port", 0, "HTTP port override (default uses config server_port)")
	flag.Parse()

	err := server.Run(*portFlag)
	if err != nil {
		slog.Error("auto-prd", "err", err)
		os.Exit(1)
	}
}
