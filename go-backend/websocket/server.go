package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"

	"github.com/gin-gonic/gin"
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
		// Update DB Status
		go updateNodeStatus(nodeId, 1, client.Version)
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

func updateNodeStatus(nodeId int64, status int, version string) {
	var node model.Node
	if err := global.DB.First(&node, nodeId).Error; err == nil {
		node.Status = status
		if version != "" {
			node.Version = version
		}
		global.DB.Save(&node)
	}
}

func HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	id := c.Query("id")
	msgType := c.Query("type")
	secret := c.Query("secret")
	version := c.Query("version")

	// Validate Node Secret
	if msgType == "1" {
		nodeId, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			conn.Close()
			return
		}
		var node model.Node
		if err := global.DB.First(&node, nodeId).Error; err != nil {
			conn.Close() // Node not found
			return
		}
		if node.Secret != secret {
			conn.Close() // Invalid Secret
			return
		}
	} else {
		// Admin validation?
		// For now simple admin check or assume internal trust if no better auth mechanism provided in specs.
		// Java used session attributes via interceptor?
		// We can add simple token check here if passed in query.
	}

	client := &Client{
		ID:      id,
		Type:    msgType,
		Conn:    conn,
		Secret:  secret,
		Version: version,
		Valid:   true,
	}

	if secret != "" {
		client.AES = NewAESCrypto(secret)
	}

	Manager.Register(client)
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
func SendMsg(nodeId int64, msg interface{}, msgType string) *dto.GostDto {
	Manager.mu.Lock()
	client, ok := Manager.NodeSessions[nodeId]
	Manager.mu.Unlock()

	if !ok || client == nil || !client.Valid {
		return &dto.GostDto{Msg: "节点不在线"}
	}

	requestId := uuid.New().String()

	// Create Request Payload
	payload := dto.CommandMessage{
		Type:      msgType,
		Data:      msg,
		RequestId: requestId,
	}
	jsonPayload, _ := json.Marshal(payload)

	// Setup Response Channel
	respChan := make(chan dto.GostDto, 1) // Buffered to avoid block
	Manager.mu.Lock()
	Manager.PendingRequests[requestId] = respChan
	Manager.mu.Unlock()

	// Send
	if err := client.SendEncrypted(string(jsonPayload)); err != nil {
		Manager.mu.Lock()
		delete(Manager.PendingRequests, requestId)
		Manager.mu.Unlock()
		return &dto.GostDto{Msg: "发送失败: " + err.Error()}
	}

	// Wait Response
	select {
	case res := <-respChan:
		return &res
	case <-time.After(10 * time.Second):
		Manager.mu.Lock()
		delete(Manager.PendingRequests, requestId)
		Manager.mu.Unlock()
		return &dto.GostDto{Msg: "等待响应超时"}
	}
}
