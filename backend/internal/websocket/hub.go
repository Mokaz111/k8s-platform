package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// Client 表示一个 WebSocket 在线客户端连接
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	userID   uint64
	permTree *auth.PermissionTree
	send     chan []byte
	channels map[string]bool // 订阅的频道
	mu       sync.RWMutex
}

// newClient 构造一个 Client 实例
func newClient(hub *Hub, conn *websocket.Conn, userID uint64, pt *auth.PermissionTree) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		userID:   userID,
		permTree: pt,
		send:     make(chan []byte, 256),
		channels: make(map[string]bool),
	}
}

// subscribed 判断客户端是否订阅了指定频道（支持前缀匹配，便于 pod_logs:{cluster}:{ns}:{pod} 等动态频道）
func (c *Client) subscribed(channel string) bool {
	if channel == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.channels[channel] {
		return true
	}
	// 前缀匹配：例如订阅 "pod_logs:cluster1:default:nginx"，命中 "pod_logs:cluster1:default:nginx:*" 等
	for ch := range c.channels {
		// 简单前缀匹配（订阅时若以 "*" 结尾视为通配前缀）
		if len(ch) > 0 && ch[len(ch)-1] == '*' {
			if len(channel) >= len(ch)-1 && channel[:len(ch)-1] == ch[:len(ch)-1] {
				return true
			}
		}
	}
	return false
}

// subscribe 订阅频道
func (c *Client) subscribe(channel string) {
	if channel == "" {
		return
	}
	c.mu.Lock()
	c.channels[channel] = true
	c.mu.Unlock()
}

// unsubscribe 取消订阅
func (c *Client) unsubscribe(channel string) {
	c.mu.Lock()
	delete(c.channels, channel)
	c.mu.Unlock()
}

// 消息类型常量
const (
	TypePodLogs      = "pod_logs"
	TypeTaskProgress = "task_progress"
	TypeClusterEvent = "cluster_event"
	TypePong         = "pong"
	TypeError        = "error"
)

// Redis PubSub channel 名称（与 BackupWorker 等发布端约定）
const (
	RedisChannelTaskProgress = "notifier:task_progress"
	RedisChannelClusterEvent = "notifier:cluster_event"
	RedisChannelPodLogs      = "notifier:pod_logs"
)

// Message WebSocket 推送给客户端的消息统一格式
// channel 标识订阅频道（如 "task_progress" 或 "pod_logs:cluster1:default:nginx"）
// data 为原始负载（JSON 反序列化为 map 后透传）
type Message struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Hub 管理所有在线 WebSocket 客户端连接，并负责把 Redis PubSub 收到的
// 三通道消息（pod_logs / task_progress / cluster_event）按订阅关系转发给客户端
type Hub struct {
	log *logger.Logger
	rdb *redis.Client

	mu      sync.RWMutex
	clients map[*Client]bool

	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte

	stopCh chan struct{}
	once   sync.Once
}

func NewHub(rdb *redis.Client, log *logger.Logger) *Hub {
	return &Hub{
		log:        log,
		rdb:        rdb,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan []byte, 256),
		stopCh:     make(chan struct{}),
	}
}

// Start 启动 Hub：1) select register/unregister/broadcast 主循环
// 2) 订阅 Redis PubSub 3 个 channel，收到后按 type 广播给所有订阅了对应频道的 Client
func (h *Hub) Start() {
	go h.runLoop()
	go h.subscribeRedis()
	h.log.Info("🌐 WebSocket Hub started (3 Redis PubSub channels subscribed)")
}

// Stop 关闭 Hub（优雅退出）
func (h *Hub) Stop() {
	h.once.Do(func() { close(h.stopCh) })
}

// Register 注册一个新客户端（线程安全，可外部调用）
func (h *Hub) Register(c *Client) {
	select {
	case h.register <- c:
	default:
		// register 队列满了直接丢，主循环会兜底
	}
}

// Unregister 注销客户端
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	default:
	}
}

// Broadcast 广播一条原始 JSON 消息给所有客户端（无论订阅）
func (h *Hub) Broadcast(payload []byte) {
	select {
	case h.broadcast <- payload:
	default:
	}
}

// SendToClient 给指定客户端发送原始字节（非阻塞，send 满则丢弃）
func (h *Hub) SendToClient(c *Client, payload []byte) {
	select {
	case c.send <- payload:
	default:
		// send 满，说明客户端消费慢，丢弃此条
	}
}

// BroadcastToChannel 把消息推送给所有订阅了指定 channel 的客户端
// payload 必须是已经序列化好的 Message JSON（含 channel 字段）
func (h *Hub) BroadcastToChannel(channel string, payload []byte) {
	h.broadcastToChannel(channel, payload, nil)
}

// ChannelSubscriberCount 返回当前订阅了指定频道的在线客户端数
func (h *Hub) ChannelSubscriberCount(channel string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for c := range h.clients {
		if c.subscribed(channel) {
			n++
		}
	}
	return n
}

func (h *Hub) broadcastToChannel(channel string, payload []byte, progressRaw []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if !c.subscribed(channel) {
			continue
		}
		if channel == TypeTaskProgress && progressRaw != nil {
			if !authorizeTaskProgressPayload(c.permTree, progressRaw) {
				continue
			}
		}
		select {
		case c.send <- payload:
		default:
		}
	}
}

// publishMessage 把 Redis PubSub 收到的原始 payload 按指定 type/channel 推送
func (h *Hub) publishMessage(msgType string, channel string, payload []byte) {
	out, err := json.Marshal(Message{
		Type:    msgType,
		Channel: channel,
		Data:    json.RawMessage(payload),
	})
	if err != nil {
		return
	}
	if channel == TypeTaskProgress {
		h.broadcastToChannel(channel, out, payload)
		return
	}
	h.broadcastToChannel(channel, out, nil)
}

// runLoop 主循环：处理客户端上下线 + 全量广播
func (h *Hub) runLoop() {
	for {
		select {
		case <-h.stopCh:
			h.mu.Lock()
			for c := range h.clients {
				close(c.send)
				_ = c.conn.Close()
				delete(h.clients, c)
			}
			h.mu.Unlock()
			return
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()
			h.log.Debugf("WS client registered: user_id=%d total=%d", c.userID, len(h.clients))
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			h.log.Debugf("WS client unregistered: user_id=%d total=%d", c.userID, len(h.clients))
		case payload := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- payload:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

// subscribeRedis 订阅 3 个 Redis PubSub channel，把消息按 type 转发到 Hub
// 订阅映射：Redis channel → (消息 type, 客户端订阅 channel 前缀)
var redisChannelMap = map[string]struct {
	msgType string
	subChan string
}{
	RedisChannelTaskProgress: {TypeTaskProgress, TypeTaskProgress},
	RedisChannelClusterEvent: {TypeClusterEvent, TypeClusterEvent},
	RedisChannelPodLogs:      {TypePodLogs, TypePodLogs},
}

func (h *Hub) subscribeRedis() {
	// 用单独 goroutine 订阅每个 channel，避免互相阻塞
	for ch := range redisChannelMap {
		go h.subscribeOneRedisChannel(ch)
	}
}

func (h *Hub) subscribeOneRedisChannel(redisCh string) {
	mapping := redisChannelMap[redisCh]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		select {
		case <-h.stopCh:
			return
		default:
		}

		pubsub := h.rdb.Subscribe(ctx, redisCh)
		msgCh := pubsub.Channel()
		h.log.Infof("WS Hub subscribed Redis channel: %s", redisCh)

		func() {
			defer func() {
				_ = pubsub.Close()
			}()
			for {
				select {
				case <-h.stopCh:
					return
				case msg, ok := <-msgCh:
					if !ok {
						return
					}
					if msg == nil || len([]byte(msg.Payload)) == 0 {
						continue
					}
					h.publishMessage(mapping.msgType, mapping.subChan, []byte(msg.Payload))
				}
			}
		}()

		// 订阅断开，重试
		select {
		case <-h.stopCh:
			return
		case <-time.After(2 * time.Second):
		}
	}
}
