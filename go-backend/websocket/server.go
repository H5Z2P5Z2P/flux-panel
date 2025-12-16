package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-backend/config"
	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

type Client struct {
	ID        string // Node ID (string)
	Type      string // "1" for Node, others for Admin
	Conn      *websocket.Conn
	Secret    string
	Version   string
	AES       *AESCrypto
	Valid     bool
	WriteLock sync.Mutex
}

type WSManager struct {
	NodeSessions    map[int64]*Client
	AdminSessions   map[*Client]bool
	PendingRequests map[string]chan dto.GostDto
	mu              sync.RWMutex
}

var Manager = &WSManager{
	NodeSessions:    make(map[int64]*Client),
	AdminSessions:   make(map[*Client]bool),
	PendingRequests: make(map[string]chan dto.GostDto),
}

func (m *WSManager) Register(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client.Type == "1" {
		nodeId, _ := strconv.ParseInt(client.ID, 10, 64)
		// Kick existing
		if old, ok := m.NodeSessions[nodeId]; ok {
			old.Valid = false
			old.Conn.Close()
		}
		m.NodeSessions[nodeId] = client
		// Broadcast Status Online
		m.broadcastStatus(client.ID, 1)
		// Update DB Status - Handled by HandleWebSocket detailed update
	} else {
		m.AdminSessions[client] = true
	}
}

func (m *WSManager) Unregister(client *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client.Type == "1" {
		nodeId, _ := strconv.ParseInt(client.ID, 10, 64)
		if current, ok := m.NodeSessions[nodeId]; ok && current == client {
			delete(m.NodeSessions, nodeId)
			// Broadcast Status Offline
			m.broadcastStatus(client.ID, 0)
			// Update DB Status
			go updateNodeStatus(nodeId, 0, "")
		}
	} else {
		delete(m.AdminSessions, client)
	}
	client.Valid = false
	client.Conn.Close()
}

func (m *WSManager) broadcastStatus(id string, status int) {
	// Construct message
	msg := map[string]interface{}{
		"id":   id,
		"type": "status",
		"data": status,
	}
	jsonMsg, _ := json.Marshal(msg)

	// Send to all admins
	for client := range m.AdminSessions {
		go client.SendText(string(jsonMsg))
	}
}

func updateNodeStatusDetail(nodeId int64, status int, version, httpStr, tlsStr, socksStr string) {
	var node model.Node
	if err := global.DB.First(&node, nodeId).Error; err == nil {
		node.Status = status
		if version != "" {
			node.Version = &version
		}
		if httpStr != "" {
			if p, err := strconv.Atoi(httpStr); err == nil {
				node.Http = p
			}
		}
		if tlsStr != "" {
			if p, err := strconv.Atoi(tlsStr); err == nil {
				node.Tls = p
			}
		}
		if socksStr != "" {
			if p, err := strconv.Atoi(socksStr); err == nil {
				node.Socks = p
			}
		}
		global.DB.Save(&node)
	}
}

func updateNodeStatus(nodeId int64, status int, version string) {
	updateNodeStatusDetail(nodeId, status, version, "", "", "")
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// Query params from handshake
	idParam := c.Query("id") // Likely empty for Node
	msgType := c.Query("type")
	secret := c.Query("secret")
	version := c.Query("version")

	clientId := idParam

	// Validate Node
	if msgType == "1" {
		// Java logic: Node lookup by secret
		var node model.Node
		if err := global.DB.Where("secret = ?", secret).First(&node).Error; err != nil {
			conn.Close() // Not found or invalid secret
			return
		}
		// Secret is valid if found
		clientId = strconv.FormatInt(node.ID, 10)
	} else {
		// Admin validation (Type != 1)
		// Java: WebSocketInterceptor validates token
		if secret == "" {
			conn.Close()
			return
		}
		// Inline JWT Validation to avoid import cycle with utils -> service -> websocket
		token, err := jwt.Parse(secret, func(token *jwt.Token) (interface{}, error) {
			return []byte(config.AppConfig.JwtSecret), nil
		})

		if err != nil || !token.Valid {
			conn.Close()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			conn.Close()
			return
		}

		// Extract Subject (UserId)
		sub, _ := claims.GetSubject()
		if sub == "" {
			// Fallback: Check "id" claim if any? Utils uses StandardClaims Subject.
			conn.Close()
			return
		}
		clientId = sub
	}

	client := &Client{
		ID:      clientId,
		Type:    msgType,
		Conn:    conn,
		Secret:  secret,
		Version: version,
		Valid:   true,
	}

	if secret != "" {
		client.AES = NewAESCrypto(secret)
	}

	// Register Client
	Manager.Register(client)

	// Update Node Status with Ports
	if msgType == "1" {
		httpPortStr := c.Query("http")
		tlsPortStr := c.Query("tls")
		socksPortStr := c.Query("socks")

		nodeId, _ := strconv.ParseInt(clientId, 10, 64)
		go updateNodeStatusDetail(nodeId, 1, version, httpPortStr, tlsPortStr, socksPortStr)
	}

	go client.ReadPump()
}

func (c *Client) ReadPump() {
	defer func() {
		Manager.Unregister(c)
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		// Decrypt if needed
		var payload []byte
		var encryptedMsg dto.EncryptedMessage
		if err := json.Unmarshal(message, &encryptedMsg); err == nil && encryptedMsg.Encrypted {
			if c.AES != nil {
				decrypted, err := c.AES.Decrypt(encryptedMsg.Data)
				if err != nil {
					log.Printf("Decrypt error: %v", err)
					continue
				}
				payload = decrypted
			} else {
				payload = message // Fallback
			}
		} else {
			payload = message
		}

		// Process Message
		c.handleMessage(payload)
	}
}

func (c *Client) handleMessage(payload []byte) {
	// 1. Check if it's Request ID response
	var response map[string]interface{}
	if err := json.Unmarshal(payload, &response); err == nil {
		if reqId, ok := response["requestId"].(string); ok && reqId != "" {
			Manager.mu.Lock()
			if ch, found := Manager.PendingRequests[reqId]; found {
				delete(Manager.PendingRequests, reqId)
				// Parse to GostDto
				gDto := dto.GostDto{
					Msg: "OK",
				}
				if msg, ok := response["message"].(string); ok {
					gDto.Msg = msg
				}
				gDto.Data = response["data"]

				select {
				case ch <- gDto:
				default:
				}
			}
			Manager.mu.Unlock()
			return
		}
	}

	// 2. Broadcast Info (if Node)
	if c.Type == "1" {
		// Heartbeat / Status Update Response
		if strPayload := string(payload); len(strPayload) > 0 {
			if strings.Contains(strPayload, "memory_usage") {
				c.SendEncrypted(`{"type":"call"}`)
			}
		}

		// Java: If type=1, broadcast {id, type:info, data: payload} to admins
		msg := map[string]interface{}{
			"id":   c.ID,
			"type": "info",
			"data": string(payload),
		}
		jsonMsg, _ := json.Marshal(msg)
		Manager.mu.Lock()
		for admin := range Manager.AdminSessions {
			go admin.SendText(string(jsonMsg))
		}
		Manager.mu.Unlock()
	}
}

func (c *Client) SendText(msg string) error {
	c.WriteLock.Lock()
	defer c.WriteLock.Unlock()
	if !c.Valid {
		return fmt.Errorf("connection closed")
	}
	return c.Conn.WriteMessage(websocket.TextMessage, []byte(msg))
}

func (c *Client) SendEncrypted(msg string) error {
	if c.AES == nil {
		return c.SendText(msg)
	}

	encryptedData, err := c.AES.Encrypt([]byte(msg))
	if err != nil {
		return err
	}

	wrapper := dto.EncryptedMessage{
		Encrypted: true,
		Data:      encryptedData,
		Timestamp: time.Now().UnixMilli(),
	}
	jsonWrapper, _ := json.Marshal(wrapper)
	return c.SendText(string(jsonWrapper))
}

// SendMsg to Node with Timeout
func SendMsg(nodeId int64, data interface{}, msgType string) *dto.GostDto {
	// Find Client
	Manager.mu.RLock()
	client, ok := Manager.NodeSessions[nodeId]
	Manager.mu.RUnlock()

	if !ok || client == nil || !client.Valid {
		return &dto.GostDto{Msg: "节点不在线"}
	}

	requestId := uuid.New().String()

	// Construct Message
	// Java puts requestId at root: {type:..., data:..., requestId:...}
	msg := map[string]interface{}{
		"type":      msgType,
		"data":      data,
		"requestId": requestId,
	}

	// Create Channel for Response
	ch := make(chan dto.GostDto)
	Manager.mu.Lock()
	Manager.PendingRequests[requestId] = ch
	Manager.mu.Unlock()

	// Serialize
	jsonMsg, _ := json.Marshal(msg)

	// Send Encrypted
	client.SendEncrypted(string(jsonMsg))

	// Wait for Response (Timeout 10s)
	select {
	case res := <-ch:
		return &res
	case <-time.After(10 * time.Second):
		// Clean up
		Manager.mu.Lock()
		delete(Manager.PendingRequests, requestId)
		Manager.mu.Unlock()
		return &dto.GostDto{Msg: "Timeout"}
	}
}
