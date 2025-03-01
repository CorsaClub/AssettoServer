package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"metrics/config"
	"metrics/types"
	"metrics/utils"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking when HTTPS is configured
		return true
	},
}

type WebSocketServer struct {
	sync.RWMutex
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	authConfig *config.AuthConfig
}

func NewWebSocketServer(authConfig *config.AuthConfig) *WebSocketServer {
	return &WebSocketServer{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		authConfig: authConfig,
	}
}

// AuthMessage représente le message d'authentification envoyé par le client
type AuthMessage struct {
	SteamID string `json:"steamId"`
	UserID  string `json:"userId"`
}

func (s *WebSocketServer) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-s.register:
			s.Lock()
			s.clients[client] = true
			s.Unlock()
		case client := <-s.unregister:
			s.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				client.Close()
			}
			s.Unlock()
		case message := <-s.broadcast:
			s.RLock()
			for client := range s.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					utils.LogError("Error sending message to client: %v", err)
					client.Close()
					delete(s.clients, client)
				}
			}
			s.RUnlock()
		}
	}
}

func (s *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Vérifier que l'authentification est configurée
	if !s.authConfig.IsValid() {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		utils.LogError("WebSocket upgrade error: %v", err)
		return
	}

	// Attendre le message d'authentification
	var authMsg AuthMessage
	err = conn.ReadJSON(&authMsg)
	if err != nil {
		utils.LogError("Authentication message read error: %v", err)
		conn.Close()
		return
	}

	// Vérifier l'authentification
	if !s.validateAuth(authMsg) {
		utils.LogWarning("Authentication failed for SteamID: %s", authMsg.SteamID)
		conn.WriteJSON(map[string]string{"error": "Authentication failed"})
		conn.Close()
		return
	}

	utils.LogInfo("Client authenticated successfully: SteamID=%s", authMsg.SteamID)

	// Envoyer confirmation d'authentification
	conn.WriteJSON(map[string]string{"status": "authenticated"})

	s.register <- conn

	// Ping/Pong pour garder la connexion active
	go s.keepAlive(conn)
}

func (s *WebSocketServer) validateAuth(auth AuthMessage) bool {
	return auth.SteamID == s.authConfig.SteamID &&
		auth.UserID == s.authConfig.UserID
}

func (s *WebSocketServer) keepAlive(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			return
		}
	}
}

func (s *WebSocketServer) BroadcastLog(log types.LogEntry) {
	data, err := json.Marshal(log)
	if err != nil {
		utils.LogError("Error marshaling log: %v", err)
		return
	}
	s.broadcast <- data
}
