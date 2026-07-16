package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"
)

type InfoResponse struct {
	PodIP    string `json:"pod_ip"`
	NodeName string `json:"node_name"`
	Arch     string `json:"architecture"`
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	resp := InfoResponse{
		PodIP:    os.Getenv("POD_IP"),
		NodeName: os.Getenv("NODE_NAME"),
		Arch:     runtime.GOARCH,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func main() {
	http.HandleFunc("/", infoHandler)
	http.HandleFunc("/healthz", healthzHandler)

	addr := ":8080"
	log.Printf("hello-ci listening on %s (arch=%s)", addr, runtime.GOARCH)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
