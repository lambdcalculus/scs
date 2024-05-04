package main

import (
    "os"

    "github.com/lambdcalculus/scs/internal/server"
    "github.com/lambdcalculus/scs/pkg/logger"
)

func main() {
    log := logger.NewLoggerOutputs(logger.LevelTrace, nil, "stdout", "log/server.log")
    serv, err := server.MakeServer(log)
    if err != nil {
        log.Fatalf("Couldn't make server (%v).", err)
        os.Exit(1)
    }
    log.Fatalf("Server stopped running: %s", serv.Run())
}
