package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
)

var configPath = flag.String("config", "conf/admin_portal.json", "Admin portal config file path")
var listenAddr = flag.String("addr", ":8090", "HTTP listen address")
var showHelp = flag.Bool("help", false, "show usage")
var showVersion = flag.Bool("version", false, "show version")

func main() {
	flag.Parse()
	if *showHelp {
		flag.Usage()
		return
	}
	if *showVersion {
		log.Printf("admin-portal version %s", version)
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	uiDir, err := findUIDir()
	if err != nil {
		log.Fatal(err)
	}
	templates, err := loadPortalTemplates(uiDir)
	if err != nil {
		log.Fatal(err)
	}

	server := &portalServer{
		config:    normalizeConfig(cfg),
		templates: templates,
		staticDir: filepath.Join(uiDir, "static"),
		client: &http.Client{
			Timeout: proxyTimeout,
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/ui/static/", http.StripPrefix("/ui/static/", http.FileServer(http.Dir(server.staticDir))))
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc("/ui", server.handleOverview)
	mux.HandleFunc("/haproxy", server.handleHAProxy)
	mux.HandleFunc("/haproxy/", server.handleHAProxy)
	mux.HandleFunc("/sms/ui", server.handleSMSUI)
	mux.HandleFunc("/sms/ui/", server.handleSMSUI)
	mux.HandleFunc("/sms/send", server.handleSMSAPI)
	mux.HandleFunc("/sms/status", server.handleSMSStatus)
	mux.HandleFunc("/push/ui", server.handlePushUI)
	mux.HandleFunc("/push/ui/", server.handlePushUI)
	mux.HandleFunc("/push/send", server.handlePushAPI)
	mux.HandleFunc("/push/status", server.handlePushStatus)
	mux.HandleFunc("/command-center/ui", server.handleCommandCenterUI)
	mux.HandleFunc("/command-center/ui/", server.handleCommandCenterUI)

	log.Printf("listening on %s configPath=%q", *listenAddr, *configPath)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
