package main

import (
	"flag"
	"log"
	"net/http"
)

var configPath = flag.String("config", defaultConfigPath, "Services health config file path")
var listenAddr = flag.String("addr", defaultListenAddr, "HTTP listen address")

func main() {
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	ui, err := newUIServer(cfg)
	if err != nil {
		log.Fatal(err)
	}

	mux := newMux(ui)
	log.Printf("services health listening on %s configPath=%q", *listenAddr, *configPath)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
