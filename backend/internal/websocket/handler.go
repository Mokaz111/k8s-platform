package websocket

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

// wsUpgrader WebSocket 升级器，允许任意 Origin（鉴权由 ?token=xxx 完成）
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	wsReadDeadline  = 30 * time.Second // 心跳/读循环等待时长上限
	wsWriteWait     = 10 * time.Second
	wsPingPeriod    = 20 * time.Second // 服务端 ping 间隔（< wsReadDeadline）
	wsSendQueueSize = 256
)

// clientAction 客户端发来的订阅/取消订阅指令
type clientAction struct {
	Action  string `json:"action"`            // subscribe | unsubscribe | ping
	Channel string `json:"channel,omitempty"` // 频道名
}

// Handler WebSocket HTTP 处理器
type Handler struct {
	hub     *Hub
	authSvc *auth.Service
}

func NewHandler(hub *Hub, authSvc *auth.Service) *Handler {
	return &Handler{hub: hub, authSvc: authSvc}
}

// WsHandler GET /api/v1/ws?token=xxx
// HTTP 升级为 WebSocket，从 query param ?token=xxx 或 Authorization Header 提取 JWT，
// 校验通过后创建 Client 注册到 Hub
func (h *Handler) WsHandler(c *gin.Context) {
	// 1. 提取 JWT（优先 query param，兼容 Authorization Header，便于浏览器 WebSocket 不能设 header）
	tokenStr := strings.TrimSpace(c.Query("token"))
	if tokenStr == "" {
		if hdr, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer "); ok {
			tokenStr = strings.TrimSpace(hdr)
		}
	}
	if tokenStr == "" {
		response.FailWithStatus(c, http.StatusUnauthorized,
			errcode.New(errcode.Unauthenticated, "缺少 token query 参数或 Authorization Header"))
		return
	}

	claims, parseErr := h.authSvc.JWT.ParseToken(tokenStr)
	if parseErr != nil {
		if strings.Contains(parseErr.Error(), jwt.ErrTokenExpired.Error()) {
			response.FailWithStatus(c, http.StatusUnauthorized, errcode.New(errcode.TokenExpired))
			return
		}
		response.FailWithStatus(c, http.StatusUnauthorized,
			errcode.New(errcode.Unauthenticated, "Token 解析失败或无效"))
		return
	}
	if claims.Type != auth.AccessToken {
		response.FailWithStatus(c, http.StatusUnauthorized,
			errcode.New(errcode.Unauthenticated, "Token 类型错误，请使用 Access Token"))
		return
	}

	// 2. HTTP 升级为 WebSocket
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade 内部已写入响应，这里仅记录
		return
	}

	// 3. 创建 Client 注册到 Hub
	client := newClient(h.hub, conn, claims.UserID)
	h.hub.Register(client)

	// 4. 启动写 pump（把 send 通道消息推给客户端）
	go h.writePump(client)
	// 5. 启动读 pump（处理客户端发来的 subscribe/unsubscribe/ping）—— 阻塞直到连接关闭
	h.readPump(client)
}

// readPump 读取循环：处理客户端发来的 subscribe/unsubscribe 消息
// 同时实现心跳：客户端发 "ping" → 回 "pong"；30s 无活动断开
func (h *Handler) readPump(c *Client) {
	defer func() {
		c.conn.Close()
		h.hub.Unregister(c)
	}()

	c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
	// 客户端可能发 ping frame，回 pong frame 自动重置 deadline
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// 客户端关闭/超时/异常
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))

		// 尝试解析为 clientAction
		var act clientAction
		if err := json.Unmarshal(message, &act); err != nil {
			// 容错：纯文本 "ping" 也接受
			if strings.EqualFold(strings.TrimSpace(string(message)), "ping") {
				h.sendPong(c, "")
				continue
			}
			continue
		}

		switch strings.ToLower(act.Action) {
		case "subscribe":
			c.subscribe(act.Channel)
		case "unsubscribe":
			c.unsubscribe(act.Channel)
		case "ping":
			h.sendPong(c, act.Channel)
		default:
			// 未知 action，忽略
		}
	}
}

// sendPong 回复客户端一条 pong 消息（以 Message JSON 形式）
func (h *Handler) sendPong(c *Client, channel string) {
	out, err := json.Marshal(Message{
		Type:    TypePong,
		Channel: channel,
	})
	if err != nil {
		return
	}
	h.hub.SendToClient(c, out)
}

// writePump 把 send 通道里的消息推给客户端；同时周期发 ping frame 维持心跳
func (h *Handler) writePump(c *Client) {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			if !ok {
				// send 已关闭（Hub 注销）
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
