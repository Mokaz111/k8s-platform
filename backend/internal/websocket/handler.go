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

const wsTokenProtocolPrefix = "access_token."

const (
	wsReadDeadline  = 30 * time.Second
	wsWriteWait     = 10 * time.Second
	wsPingPeriod    = 20 * time.Second
	wsSendQueueSize = 256
)

type clientAction struct {
	Action  string `json:"action"`
	Channel string `json:"channel,omitempty"`
}

type Handler struct {
	hub          *Hub
	authSvc      *auth.Service
	allowOrigins []string
}

func NewHandler(hub *Hub, authSvc *auth.Service, allowOrigins []string) *Handler {
	return &Handler{hub: hub, authSvc: authSvc, allowOrigins: allowOrigins}
}

func (h *Handler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, o := range h.allowOrigins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func extractWSToken(r *http.Request) (token string, respHeader http.Header) {
	respHeader = http.Header{}
	if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
		return t, respHeader
	}
	if hdr, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if t := strings.TrimSpace(hdr); t != "" {
			return t, respHeader
		}
	}
	for _, p := range websocket.Subprotocols(r) {
		if token, ok := strings.CutPrefix(p, wsTokenProtocolPrefix); ok && token != "" {
			respHeader.Set("Sec-WebSocket-Protocol", p)
			return token, respHeader
		}
	}
	return "", respHeader
}

func (h *Handler) WsHandler(c *gin.Context) {
	tokenStr, upgradeHeader := extractWSToken(c.Request)
	if tokenStr == "" {
		response.FailWithStatus(c, http.StatusUnauthorized,
			errcode.New(errcode.Unauthenticated, "缺少 token（Sec-WebSocket-Protocol / Authorization / query）"))
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
	if h.authSvc.IsRevoked(c.Request.Context(), claims) {
		response.FailWithStatus(c, http.StatusUnauthorized,
			errcode.New(errcode.Unauthenticated, "Token 已失效"))
		return
	}

	pt, err := h.authSvc.GetUserPermissionTree(c.Request.Context(), claims.UserID)
	if err != nil {
		response.FailWithStatus(c, http.StatusForbidden, errcode.Wrap(errcode.PermissionDenied, err, "加载权限失败"))
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return h.originAllowed(r)
		},
	}

	conn, upErr := upgrader.Upgrade(c.Writer, c.Request, upgradeHeader)
	if upErr != nil {
		return
	}

	client := newClient(h.hub, conn, claims.UserID, pt)
	h.hub.Register(client)

	go h.writePump(client)
	h.readPump(client)
}

func (h *Handler) readPump(c *Client) {
	defer func() {
		c.conn.Close()
		h.hub.Unregister(c)
	}()

	c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(wsReadDeadline))

		var act clientAction
		if err := json.Unmarshal(message, &act); err != nil {
			if strings.EqualFold(strings.TrimSpace(string(message)), "ping") {
				h.sendPong(c, "")
				continue
			}
			continue
		}

		switch strings.ToLower(act.Action) {
		case "subscribe":
			if authorizeChannel(c.permTree, act.Channel) {
				c.subscribe(act.Channel)
			} else {
				h.sendError(c, act.Channel, "无权订阅该频道")
			}
		case "unsubscribe":
			c.unsubscribe(act.Channel)
		case "ping":
			h.sendPong(c, act.Channel)
		}
	}
}

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

func (h *Handler) sendError(c *Client, channel, msg string) {
	payload, err := json.Marshal(map[string]string{"message": msg})
	if err != nil {
		return
	}
	out, err := json.Marshal(Message{
		Type:    TypeError,
		Channel: channel,
		Data:    payload,
	})
	if err != nil {
		return
	}
	h.hub.SendToClient(c, out)
}

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
