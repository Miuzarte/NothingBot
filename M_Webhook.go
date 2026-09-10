package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	env "NothingBot_v4/environment"

	"github.com/Miuzarte/EasyOnebot/message"
)

type Webhook struct {
	Path  string
	Token string // Bearer Token 用于认证，为空则不验证

	UseBody bool // 当为 true 且 body 不为空时，发送 body 而不是 reply
	Reply   string

	GroupId int
	UserId  int
}

const webhookMId ModuleId = "Webhook"

var (
	webhooks   []Webhook
	httpServer *http.Server
	serverMux  *http.ServeMux
	serverLock sync.Mutex
)

var moduleWebhook = Module{
	ModuleMeta: ModuleMeta{
		Name:       webhookMId,
		Desc:       "Webhook",
		Conditions: Conditions{REMAKR_ONLY_ADMIN},
	},
	Disable: env.Testing,
}

func init() {
	NoBuildPrintFile("M_Webhook.go")

	moduleWebhook.Init = initWebhook
	moduleWebhook.ReInit = reInitWebhook
	moduleWebhook.AtExit = stopWebhookServer
	modules.Add(&moduleWebhook)
}

func initWebhook() {
	var webhookConfigs []Webhook
	err := config.DecodeModule(webhookMId, &webhookConfigs)
	if err != nil {
		log.Error(err)
		return
	}

	webhooks = decodeWebhook(webhookConfigs)
	startWebhookServer()
}

func reInitWebhook() {
	var webhookConfigs []Webhook
	err := config.DecodeModule(webhookMId, &webhookConfigs)
	if err != nil {
		log.Error(err)
		return
	}

	webhooks = decodeWebhook(webhookConfigs)
	updateWebhookRoutes()
}

func decodeWebhook(input []Webhook) (output []Webhook) {
	output = make([]Webhook, 0, len(input))
	log.Debug("[Webhook] found configs: ", len(input))
	for i, webhook := range input {
		if webhook.Path == "" {
			log.Warnf("[Webhook] invalid webhook config [%d]: path is empty", i)
			continue
		}

		if webhook.GroupId == 0 && webhook.UserId == 0 {
			log.Warnf("[Webhook] invalid webhook config [%d]: both GroupId and UserId are empty", i)
			continue
		}

		// 如果不使用 body，则 reply 字段必须非空
		if !webhook.UseBody && webhook.Reply == "" {
			log.Warnf("[Webhook] invalid webhook config [%d]: reply is empty and UseBody is false", i)
			continue
		}

		output = append(output, webhook)
	}
	return output
}

func startWebhookServer() {
	serverLock.Lock()
	defer serverLock.Unlock()

	if httpServer != nil {
		log.Debug("[Webhook] server already running")
		return
	}

	serverMux = http.NewServeMux()
	updateWebhookRoutesUnsafe()

	httpServer = new(http.Server{
		Addr:    "127.0.0.1:7771",
		Handler: serverMux,
	})

	go func() {
		log.Infof("[Webhook] starting server on %s", httpServer.Addr)
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("[Webhook] server error: %v", err)
		}
	}()
}

func updateWebhookRoutes() {
	serverLock.Lock()
	defer serverLock.Unlock()
	updateWebhookRoutesUnsafe()
}

func updateWebhookRoutesUnsafe() {
	if serverMux == nil {
		return
	}

	// 创建新的 mux 来替换旧的路由
	newMux := http.NewServeMux()
	for _, webhook := range webhooks {
		// 为配置的 path 添加 /webhook/ 前缀
		path := strings.TrimPrefix(webhook.Path, "/")
		fullPath := "/webhook/" + path

		newMux.HandleFunc(fullPath, func(w http.ResponseWriter, r *http.Request) {
			handleWebhook(w, r, webhook)
		})
		log.Debugf("[Webhook] registered route: %s", fullPath)
	}

	serverMux = newMux
	if httpServer != nil {
		httpServer.Handler = newMux
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request, webhook Webhook) {
	// 获取真实源 IP（支持 Cloudflare + Caddy）
	clientIP := getClientIP(r)
	log.Infof("[Webhook] received request: %s from %s", r.URL.Path, clientIP)

	// Bearer Token 认证
	if webhook.Token != "" {
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			log.Warnf("[Webhook] authentication failed from %s: invalid or missing token: %v", clientIP, r.Header)
		}

		expectedAuth := "Bearer " + webhook.Token

		if authHeader != expectedAuth {
			log.Warnf("[Webhook] authentication failed from %s: invalid or missing token: %q", clientIP, authHeader)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "Unauthorized: invalid or missing token")
			return
		}
		log.Debugf("[Webhook] authentication successful")
	}

	// 读取并打印 body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("[Webhook] failed to read body: %v", err)
	} else {
		log.Debugf("[Webhook] request body: %s", string(body))
	}

	// 决定发送的内容：如果 UseBody 为 true 且 body 不为空，发送 body；否则发送 reply
	var messageText string
	if webhook.UseBody && len(body) > 0 {
		messageText = string(body)
		log.Debugf("[Webhook] using body as message content")
	} else {
		messageText = webhook.Reply
		log.Debugf("[Webhook] using reply field as message content")
	}

	// 发送消息
	if webhook.GroupId != 0 {
		_, err = onebot.Call().Std.SendGroupMsg(webhook.GroupId, message.Text(messageText))
	} else if webhook.UserId != 0 {
		_, err = onebot.Call().Std.SendPrivateMsg(webhook.UserId, message.Text(messageText))
	}

	if err != nil {
		log.Errorf("[Webhook] failed to send message: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to send message: %v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Message sent successfully")
}

// getClientIP 获取真实客户端 IP，支持 Cloudflare 和其他反向代理
func getClientIP(r *http.Request) string {
	// 优先使用 Cloudflare 的 CF-Connecting-IP 头
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}

	// 其次使用 X-Real-IP 头
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// 最后使用 X-Forwarded-For 头（取第一个 IP）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 如果都没有，返回 RemoteAddr
	return r.RemoteAddr
}

func stopWebhookServer() {
	serverLock.Lock()
	defer serverLock.Unlock()

	if httpServer != nil {
		log.Info("[Webhook] stopping server")

		// 创建一个带超时的 context，给服务器 5 秒时间来优雅关闭
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 使用 Shutdown 而不是 Close，这样可以等待现有连接完成
		err := httpServer.Shutdown(ctx)
		if err != nil {
			log.Errorf("[Webhook] failed to shutdown server gracefully: %v", err)
			// 如果优雅关闭失败，强制关闭
			err = httpServer.Close()
			if err != nil {
				log.Errorf("[Webhook] failed to close server: %v", err)
			}
		} else {
			log.Info("[Webhook] server stopped gracefully")
		}

		httpServer = nil
		serverMux = nil
	}
}
