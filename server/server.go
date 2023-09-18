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
	ip        string
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
	client := s.Clients[ws]
	log.Printf("Client connected: %s. Total connected: %d", client.ip, len(s.Clients))
	s.Pinger(client)
}

func (s *Server) OnClose(socket *gws.Conn, err error) {
	c := s.Clients[socket]
	c.pinger.Stop()
	s.RemoveClient(socket)
	log.Printf("Client disconnected: %s. Total connected: %d\n", c.ip, len(s.Clients))
}

func (s *Server) OnPing(socket *gws.Conn, payload []byte) {

}

func (s *Server) OnPong(socket *gws.Conn, payload []byte) {
	socket.WritePong(payload)
}

func (s *Server) OnMessage(socket *gws.Conn, message *gws.Message) {
	bytes := message.Bytes()
	switch MessageType(bytes[0]) {
	case PONG:
		atomic.AddInt64(s.Clients[socket].pongCount, 1)
	case MSG:
		b := gws.NewBroadcaster(gws.OpcodeBinary, bytes)
		defer b.Release()
		for c := range s.Clients {
			b.Broadcast(c)
		}
	case ASK_SYNC:
		b := gws.NewBroadcaster(gws.OpcodeBinary, bytes)
		defer b.Release()
		i := 2 // ask 2 of others for a sync
		for c := range s.Clients {
			if c != socket {
				b.Broadcast(c)
				i--
			}
			if i == 0 {
				break
			}
		}
		if i == 2 {
			//no others
			sendMsg(socket, []byte{byte(NO_SYNC_ANSWER)})
		}
	case SYNC_ANSWER:
		b := gws.NewBroadcaster(gws.OpcodeBinary, bytes)
		defer b.Release()
		for c := range s.Clients {
			if c != socket { // only notify others
				b.Broadcast(c)
			}
		}
	}
}
func (s *Server) Auth(r *http.Request) bool {
	return s.wspath != "" && strings.HasSuffix(r.URL.Path, s.wspath)
}

func (s *Server) AddClient(ws *gws.Conn, r *http.Request) *Client {
	client := &Client{
		ws: ws,
		ip: FindActualRemoteIp(r),
	}
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
				log.Printf("Client %s did not respond to ping in time", client.ip)
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
	ASK_SYNC
	SYNC_ANSWER
	NO_SYNC_ANSWER
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

func FindActualRemoteIp(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}
	ip = r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return ip
	}
	return r.RemoteAddr
}
