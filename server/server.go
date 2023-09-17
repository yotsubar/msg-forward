package msgforward

import (
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lxzan/gws"
)

var mutex = &sync.Mutex{}

type Server struct {
	Config    *Config
	wspath    string
	Clients   map[*gws.Conn]*Client
	Keepalive int
}
type Client struct {
	sync.Mutex
	pinger    *time.Ticker
	pongCount *int64
	ws        *gws.Conn
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		username := r.FormValue("username")
		pwd := r.FormValue("password")
		if username == s.Config.Username && pwd == s.Config.Password {
			mutex.Lock()
			if s.wspath == "" {
				s.wspath = randomString(32)
			}
			mutex.Unlock()
			w.Write([]byte(s.wspath))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) OnOpen(ws *gws.Conn) {
	client := s.AddClient(ws)
	log.Printf("Client connected: %s. Total connected: %d", ws.RemoteAddr(), len(s.Clients))
	s.Pinger(client)
}

func (s *Server) OnClose(socket *gws.Conn, err error) {
	s.Clients[socket].pinger.Stop()
	s.RemoveClient(socket)
	log.Printf("Client disconnected: %s. Total connected: %d\n", socket.RemoteAddr(), len(s.Clients))
}

func (s *Server) OnPing(socket *gws.Conn, payload []byte) {

}

func (s *Server) OnPong(socket *gws.Conn, payload []byte) {
	socket.WritePong(payload)
}

func (s *Server) OnMessage(socket *gws.Conn, message *gws.Message) {
	bytes := message.Bytes()
	if bytes[0] == byte(PONG) {
		atomic.AddInt64(s.Clients[socket].pongCount, 1)
	} else if bytes[0] == byte(MSG) {
		for c := range s.Clients {
			c.WriteAsync(gws.OpcodeBinary, message.Bytes())
		}
	}
}
func (s *Server) Auth(r *http.Request) bool {
	return s.wspath != "" && strings.HasSuffix(r.URL.Path, s.wspath)
}

func (s *Server) AddClient(ws *gws.Conn) *Client {
	client := &Client{ws: ws}
	s.Clients[ws] = client
	return client
}

func (s *Server) RemoveClient(ws *gws.Conn) {
	delete(s.Clients, ws)
}

// Pinger sends a ping message to the client in the interval specified in Keepalive in the ServerConfig
// If no pongs were received during the elapsed time, the server will close the client connection.
func (s *Server) Pinger(client *Client) {
	client.pinger = time.NewTicker(time.Duration(s.Keepalive) * time.Second)
	pongCount := int64(1)
	client.pongCount = &pongCount
	go func() {
		for range client.pinger.C {
			if atomic.LoadInt64(&pongCount) == 0 {
				log.Printf("Client %s did not respond to ping in time", client.ws.RemoteAddr())
				client.ws.WriteClose(1000, nil)
				return
			}

			sendMsg(client.ws, []byte{byte(PING)})
			atomic.StoreInt64(&pongCount, 0)
		}
	}()
}

type MessageType byte

const (
	Error MessageType = iota
	OK
	PING
	PONG
	MSG
)

type Config struct {
	Addr     string
	Username string
	Password string
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

func randomString(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[rand.Intn(len(charset))])
	}
	return sb.String()
}

func sendMsg(ws *gws.Conn, data []byte) error {
	err := ws.WriteAsync(gws.OpcodeBinary, data)
	return err
}
