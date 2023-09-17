package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/lxzan/gws"
	msgforward "github.com/yotsubar/msgforward/server"
)

var config *msgforward.Config

func main() {
	logPath := "./logs"
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		err = os.Mkdir(logPath, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
	file, err := os.OpenFile("logs/forward.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	log.SetOutput(file)

	config = loadConfig()

	s := &msgforward.Server{
		Config:    config,
		Keepalive: 60,
		Clients:   make(map[*gws.Conn]*msgforward.Client, 0),
	}
	http.HandleFunc("/login", s.Login)
	upgrader := gws.NewUpgrader(s, &gws.ServerOption{
		ReadAsyncEnabled: true,
		CompressEnabled:  true,
	})
	http.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {

		if !s.Auth(r) {
			log.Printf("Unauthorized client: %s.", r.RemoteAddr)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		socket, err := upgrader.Upgrade(w, r)
		if err != nil {
			return
		}
		go func() {
			socket.ReadLoop()
		}()
	})

	http.ListenAndServe(config.Addr, new(CorsMiddleware))
}

type CorsMiddleware struct {
	Next http.Handler
}

func (am *CorsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	if am.Next == nil {
		am.Next = http.DefaultServeMux
	}
	am.Next.ServeHTTP(w, r)
}
func loadConfig() *msgforward.Config {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var config = msgforward.Config{}
	err = dec.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}
	return &config
}
