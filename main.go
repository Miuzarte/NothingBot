package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"github.com/ysmood/gson"
	"golang.org/x/net/websocket"
)

const defaultConfig = `main: #冷更新
  websocket: "ws://127.0.0.1:9820" #go-cqhttp
  superUsers:  #int / []int
  #控制台日志等级，越大输出越多
  #Panic = 0
  #Fatal = 1
  #Error = 2
  #Warn  = 3
  #Info  = 4
  #Debug = 5
  #Trace = 6
  logLevel: 4
  #其他配置请参照: https://github.com/Miuzarte/NothingBot/blob/main/config.yaml
corpus: #热更新
# - #模板
#   regexp: "" #正则表达式
#   reply: "" #回复内容  string / []string  多于一条则发送合并转发消息，内容可以为空字符串，但是会被发送函数无视
#   scene: "" #触发场景 "a"/"all" / "g"/"group" / "p"/"private"
#   delay:  #延迟回复（秒）  支持小数
parse: #热更新
  settings:
    #同一会话重复解析同一链接的间隔（秒）
    sameParseInterval: 60
    #过长的视频/投票简介保留长度（中英字符）
    descTruncationLength: 32
push: #热更新
  settings:
    livePushMinimumInterval: 300 #同一直播间多次开播推送的最小间隔（秒）  用于解决某些主播因网络问题频繁重新推流导致多次推送
    dynamicUpdateInterval: 3 #拉取更新间隔（秒）
    resetCheckInterval: 15 #直播监听重连检测间隔（秒）
    roomChangeInfo: false #直播监控推送房间名更新（如果主播开播同时改房间名会导致推送两条）
    #通过拉取动态流进行推送，必须设置B站cookie，且需要关注想要推送的up
    cookie: ""
  list:
  # - #模板
  # uid: #up的uid  int ONLY
  # live: #up的直播间号，存在则监听并推送直播  int ONLY
  # user: #推送到的用户  int / []int
  # group: #推送到的群组  int / []int
  # at: #推送到群组时消息末尾at的人  int / []int
  # filter: #此键存在内容时仅推送包含在内的动态类型（白名单） []string
  #   - "DYNAMIC_TYPE_WORD" #文本动态（包括投票/预约）
  #   - "DYNAMIC_TYPE_DRAW" #图文动态（包括投票/预约）
  #   - "DYNAMIC_TYPE_AV" #视频投稿（包括动态视频）
  #   - "DYNAMIC_TYPE_ARTICLE" #文章投稿
`

const (
	PanicLevel = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

var (
	startTime          = time.Now().Unix()    //启动时间
	initCount          = 0                    //配置更新次数
	gocqUrl            = ""                   //websocketurl
	gocqConn           *websocket.Conn        //
	mainBlock          = make(chan os.Signal) //main阻塞
	tempBlock          = make(chan struct{})  //其他阻塞 热更新时重置
	logLever           = DebugLevel           //日志等级
	configPath         = ""                   //配置路径
	v                  = viper.New()          //配置体
	botName            = ""                   //bot称呼, 用于判断ctx.isToMe()
	connLost           = make(chan struct{})  //
	reconnectCount     = 0                    //
	heartbeatChecking  = false                //
	heartbeatOK        = false                //
	heartbeatCount     = 0                    //
	heartbeatLostCount = 0                    //
	heartbeatChan      = make(chan struct{})  //
	selfID             = 0                    //机器人QQ
	suID               = []int{}              //超级用户
	unescape           = strings.NewReplacer( //反转义还原CQ码
		"&amp;", "&", "&#44;", ",", "&#91;", "[", "&#93;", "]")
	msgTableGroup  = make(map[int]map[int]gocqMessage) //group_id:msg_id:msg
	msgTableFriend = make(map[int]map[int]gocqMessage) //user_id:msg_id:msg
)

var timeLayout = struct {
	L24  string
	L24C string
	M24  string
	M24C string
	S24  string
	S24C string
	T24  string
	T24C string
}{
	L24:  "2006/01/02 15:04:05",
	L24C: "2006年01月02日15时04分05秒",
	M24:  "01/02 15:04:05",
	M24C: "01月02日15时04分05秒",
	S24:  "02 15:04:05",
	S24C: "02日15时04分05秒",
	T24:  "15:04:05",
	T24C: "15时04分05秒",
}

var iheaders = map[string]string{
	"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
	"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	"Dnt":                "1",
	"Origin":             "https://t.bilibili.com",
	"Referer":            "https://t.bilibili.com/",
	"Sec-Ch-Ua":          "\"Not/A)Brand\";v=\"99\", \"Microsoft Edge\";v=\"115\", \"Chromium\";v=\"115\"",
	"Sec-Ch-Ua-Mobile":   "?0",
	"Sec-Ch-Ua-Platform": "\"Windows\"",
	"Sec-Fetch-Dest":     "document",
	"Sec-Fetch-Mode":     "navigate",
	"Sec-Fetch-Site":     "none",
	"Sec-Fetch-User":     "?1",
	"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.1901.203",
}

type gocqHeartbeat struct {
	self_id  int
	interval int
}

type gocqLifecycle struct {
	self_id      int
	_post_method int
}

type gocqPoke struct {
	group_id  int
	sender_id int
	target_id int
}

type gocqGroupRecall struct {
	time        int
	group_id    int
	user_id     int
	operator_id int //不同则为管理员撤回
	message_id  int
}

type gocqFriendRecall struct {
	time       int
	user_id    int
	message_id int
}

type gocqRequest struct {
	request_type string //请求类型: "friend"好友请求, "group"群请求
}

// 消息结构体
type gocqMessage struct {
	message_type    string //消息类型: "private"私聊消息, "group"群消息
	sub_type        string //消息子类型: "friend"好友, "normal"群聊, "anonymous"匿名, "group_self"群中自身发送, "group"群临时会话, "notice"系统提示, "connect"建立ws连接
	time            int    //时间戳
	user_id         int    //来源用户
	group_id        int    //来源群聊
	message_id      int    //消息ID
	message_seq     int    //消息序列
	raw_message     string //消息内容
	message         string //消息内容
	sender_nickname string //QQ昵称
	sender_card     string //群名片
	sender_rold     string //群身份: "owner", "admin", "member"
	extra           gocqMessageExtra
}

// 非标数据
type gocqMessageExtra struct {
	recalled         bool   //是否被撤回
	operator_id      int    //撤回者ID
	timeFormat       string //格式化的时间
	messageWithReply string //带回复内容的消息
	atWho            []int  //@的人
}

// 自定义消息转发节点
type gocqNodeData struct {
	name    string   //发送者名字
	uin     int      //发送者头像
	content []string //自定义消息
	seq     string   //具体消息
	time    int64    //时间戳
}

// 连接go-cqhttp
func connect(url string) {
	retryCount := 0
	for {
		c, err := websocket.Dial(url, "", "http://127.0.0.1")
		if err == nil {
			log.Info("[main] 与go-cqhttp建立ws连接成功")
			heartbeatOK = true
			gocqConn = c
			log2SU.Info(fmt.Sprint("[main] 已上线#", retryCount))
			break
		}
		retryCount++
		log.Error("[main] 与go-cqhttp建立ws连接失败, 5秒后重试")
		time.Sleep(time.Second * 5)
	}
	for {
		var rawPost string
		err := websocket.Message.Receive(gocqConn, &rawPost)
		if !heartbeatOK {
			log.Error("[main] websocket连接终止 !heartbeatOK")
			connLost <- struct{}{}
			break
		}
		if err == io.EOF {
			log.Error("[main] websocket连接终止 err == io.EOF")
			connLost <- struct{}{}
			heartbeatOK = false
			break
		}
		if err != nil {
			log.Error("[main] websocket连接出错 err != nil\n", err)
			continue
		}
		postHandler(rawPost)
	}
}

// gocq心跳监听
func heartbeatCheck(interval int) {
	log.Info("[main] 开始监听 go-cqhttp 心跳")
	retry := func() {
		reconnectCount++
		heartbeatOK = false
		time.Sleep(3)
		go connect(gocqUrl)
	}
	defer func() { heartbeatChecking = false }()
	for {
		select {
		case <-heartbeatChan:
			heartbeatCount++
		case <-time.After(time.Second * time.Duration(interval+2)):
			log.Error("[main] 心跳超时，开始重连")
			heartbeatLostCount++
			retry()
			break
		case <-connLost:
			log.Error("[main] 连接丢失，开始重连")
			retry()
			break
		}
	}
}

// 上报消息至go-cqhttp
func postMsg(msg gson.JSON) {
	if heartbeatOK {
		gocqConn.Write([]byte(msg.JSON("", "")))
	} else {
		log.Error("[main] 未连接到go-cqhttp")
	}
}

// 处理消息
func postHandler(rawPost string) {
	log.Trace("[gocq] 上报: ", rawPost)
	p := gson.NewFrom(rawPost)
	var request gocqRequest
	switch p.Get("post_type").Str() { //上报类型: "message"消息, "message_sent"消息发送, "request"请求, "notice"通知, "meta_event"
	case "message":
		msg := gocqMessage{ //消息内容
			message_type:    p.Get("message_type").Str(),
			sub_type:        p.Get("sub_type").Str(),
			time:            p.Get("time").Int(),
			user_id:         p.Get("user_id").Int(),
			group_id:        p.Get("group_id").Int(),
			message_id:      p.Get("message_id").Int(),
			message_seq:     p.Get("message_seq").Int(),
			raw_message:     p.Get("raw_message").Str(),
			message:         p.Get("message").Str(),
			sender_nickname: p.Get("sender.nickname").Str(),
			sender_card:     p.Get("sender.card").Str(),
			sender_rold:     p.Get("sender.role").Str(),
		}
		msg.extra = gocqMessageExtra{
			timeFormat:       time.Unix(int64(p.Get("time").Int()), 0).Format(timeLayout.T24),
			messageWithReply: msg.entityReply(),
			atWho:            msg.collectAt(),
		}
		switch msg.message_type {
		case "group":
			if msgTableGroup[msg.group_id] == nil {
				msgTableGroup[msg.group_id] = make(map[int]gocqMessage)
			}
			if msg.user_id != selfID {
				log.Info("[gocq] 在 ", msg.group_id, " 收到 ", msg.sender_card, "(", msg.sender_nickname, " ", msg.user_id, ") 的群聊消息: ", msg.message)
			}
			msgTableGroup[msg.group_id][msg.message_id] = msg //消息缓存
		case "private":
			if msgTableFriend[msg.user_id] == nil {
				msgTableFriend[msg.user_id] = make(map[int]gocqMessage)
			}
			if msg.user_id != selfID {
				log.Info("[gocq] 收到 ", msg.sender_nickname, "(", msg.user_id, ") 的消息: ", msg.message)
			}
			msgTableFriend[msg.user_id][msg.message_id] = msg //消息缓存
		}
		if msg.user_id == selfID {
			return
		}
		for i := 0; i < len(v.GetStringSlice("main.ban.group")); i++ { //群聊黑名单
			if v.GetInt(fmt.Sprint("main.ban.group.", i)) == 0 {
				break
			}
			if msg.group_id == v.GetInt(fmt.Sprint("main.ban.group.", i)) {
				log.Info("[gocq] 黑名单群组: ", msg.group_id)
				return
			}
		}
		for i := 0; i < len(v.GetStringSlice("main.ban.private")); i++ { //私聊黑名单
			if v.GetInt(fmt.Sprint("main.ban.private.", i)) == 0 {
				break
			}
			if msg.user_id == v.GetInt(fmt.Sprint("main.ban.private.", i)) {
				log.Info("[gocq] 黑名单用户: ", msg.sender_nickname, "(", msg.user_id, ")")
				return
			}
		}
		go func(msg gocqMessage) {
			go checkCorpus(msg)
			go checkParse(msg)
			go checkSearch(msg)
			go checkRecall(msg)
			go checkAt(msg)
			go checkInfo(msg)
			go checkBotInternal(msg)
			go checkSetu(msg)
		}(msg)
	case "message_sent":
		log.Info("[gocq] 发出了一条消息")
	case "request":
		request = gocqRequest{}
		_ = request
		log.Info("[gocq] request: ", rawPost)
	case "notice":
		switch p.Get("notice_type").Str() { //https://docs.go-cqhttp.org/reference/data_struct.html#post-notice-type
		case "group_recall": //群消息撤回
			recall := gocqGroupRecall{
				time:        p.Get("time").Int(),
				group_id:    p.Get("group_id").Int(),
				user_id:     p.Get("user_id").Int(),
				operator_id: p.Get("operator_id").Int(),
				message_id:  p.Get("message_id").Int(),
			}
			log.Info("[gocq] 在 ", recall.group_id, " 收到 ", recall.user_id, " 撤回群聊消息: ", msgTableGroup[recall.group_id][recall.message_id].message, " (", recall.message_id, ")")
			if msgTableGroup[recall.group_id] != nil { //防止开机刚好遇到撤回
				msg := msgTableGroup[recall.group_id][recall.message_id]
				msg.extra.recalled = true //标记撤回
				msg.extra.operator_id = recall.operator_id
				msgTableGroup[recall.group_id][recall.message_id] = msg
			}
		case "friend_recall": //好友消息撤回
			recall := gocqFriendRecall{
				time:       p.Get("time").Int(),
				user_id:    p.Get("user_id").Int(),
				message_id: p.Get("message_id").Int(),
			}
			log.Info("[gocq] 收到 ", recall.user_id, " 撤回私聊消息: ", msgTableFriend[recall.user_id][recall.message_id], " (", recall.message_id, ")")
			if msgTableFriend[recall.user_id] != nil { //防止开机刚好遇到撤回
				msg := msgTableFriend[recall.user_id][recall.message_id]
				msg.extra.recalled = true //标记撤回
				msgTableFriend[recall.user_id][recall.message_id] = msg
			}
		case "notify": //通知
			switch p.Get("sub_type").Str() {
			case "poke":
				poke := gocqPoke{
					group_id:  p.Get("group_id").Int(),
					sender_id: p.Get("sender_id").Int(),
					target_id: p.Get("target_id").Int(),
				}
				poke.pokeHandler()
			default:
				log.Info("[gocq] notice.notify: ", rawPost)
				log.Info("[gocq] notice.notify.sub_type: ", p.Get("sub_type").Str())
			}
		default:
			log.Info("[gocq] notice: ", rawPost)
		}
	case "meta_event": //元事件
		switch p.Get("meta_event_type").Str() { //"lifecycle"/"heartbeat"
		case "heartbeat":
			go func() { heartbeatChan <- struct{}{} }()
			heartbeatOK = true
			heartbeat := gocqHeartbeat{
				self_id:  p.Get("self_id").Int(),
				interval: p.Get("interval").Int(),
			}
			log.Debug("[gocq] heartbeat: ", heartbeat)
			if !heartbeatChecking {
				heartbeatChecking = true
				go heartbeatCheck(heartbeat.interval)
			}
		case "lifecycle":
			lifecycle := gocqLifecycle{
				self_id:      p.Get("self_id").Int(),
				_post_method: p.Get("_post_method").Int(),
			}
			selfID = lifecycle.self_id
			log.Info("[gocq] lifecycle: ", lifecycle)
		default:
			log.Info("[gocq] meta_event: ", p.JSON("", ""))
		}
	default:
		if !p.Get("data.message_id").Nil() && !p.Get("retcode").Nil() && !p.Get("status").Nil() {
			log.Info("[gocq] 消息发送成功    message_id: ", p.Get("data.message_id").Int(), "  retcode: ", p.Get("retcode").Int(), "  status: ", p.Get("status").Str())
		} else {
			log.Debug("[gocq] raw: ", rawPost)
		}
	}
}

// 反转义还原CQ码
func (g gocqMessage) unescape() gocqMessage {
	g.message = unescape.Replace(g.message)
	return g
}

// 具体化回复，go-cqhttp.extra-reply-data: true时不必要，但是开了那玩意又会导致回复带上原文又触发一遍机器人
func (ctx gocqMessage) entityReply() (messageWithReply string) {
	match := ctx.regexpMustCompile(`\[CQ:reply,id=(.*)]`)
	if len(match) > 0 {
		replyid_str := match[0][1]
		replyid_int, _ := strconv.Atoi(replyid_str)
		replyMsg := gocqMessage{}
		var reply string
		switch ctx.message_type {
		case "group":
			replyMsg = msgTableGroup[ctx.group_id][replyid_int]
		case "private":
			replyMsg = msgTableFriend[ctx.user_id][replyid_int]
		}
		reply = fmt.Sprint("[CQ:reply,qq=", replyMsg.user_id, ",time=", replyMsg.time, ",text=", replyMsg.message, "]")
		messageWithReply = strings.ReplaceAll(ctx.message, match[0][0], reply)
	} else {
		messageWithReply = ctx.message
	}
	return
}

// @的人列表
func (ctx gocqMessage) collectAt() (atWho []int) {
	match := ctx.regexpMustCompile(`\[CQ:reply,id=(.*)]`) //回复也算@
	if len(match) > 0 {
		replyid, _ := strconv.Atoi(match[0][1])
		switch ctx.message_type {
		case "group":
			atWho = append(atWho, msgTableGroup[ctx.group_id][replyid].user_id)
		case "private":
			atWho = append(atWho, msgTableFriend[ctx.user_id][replyid].user_id)
		}
	}
	match = ctx.regexpMustCompile(`\[CQ:at,qq=(.*)]`)
	if len(match) > 0 {
		for _, v := range match {
			atID, _ := strconv.Atoi(v[1])
			repeat := func() (repeat bool) { //检查重复收录
				for _, a := range atWho {
					if atID == a {
						repeat = true
					}
				}
				return
			}()
			if !repeat {
				atWho = append(atWho, atID)
			}
		}
	}
	return
}

// 戳一戳处理，先写死
func (poke gocqPoke) pokeHandler() {
	log.Info("[gocq] 收到 ", poke.sender_id, " 对 ", poke.target_id, " 的戳一戳")
	if poke.target_id == selfID && poke.sender_id != poke.target_id && poke.group_id != 0 {
		sendGroupMsg(poke.group_id, "[NothingBot] 在一条消息内只at我两次可以获取帮助信息～")
	}
}

// 发送群聊消息
func sendGroupMsg(group_id int, msg ...any) {
	if group_id == 0 || len(msg) == 0 {
		return
	}
	g := gson.New("")
	g.Set("action", "send_group_msg")
	g.Set("params", map[string]any{"group_id": group_id, "message": fmt.Sprint(msg...)})
	log.Info("[main] 发送消息到群聊 ", group_id, " ", g.Get("params.message").Str())
	postMsg(g)
	return
}

// 发送私聊消息
func sendPrivateMsg(user_id int, msg ...any) {
	if user_id == 0 || len(msg) == 0 {
		return
	}
	g := gson.NewFrom("")
	g.Set("action", "send_private_msg")
	g.Set("params", map[string]any{"user_id": user_id, "message": fmt.Sprint(msg...)})
	log.Info("[main] 发送消息到好友 ", user_id, " ", g.Get("params.message").Str())
	postMsg(g)
	return
}

// 发送群聊合并转发消息
func sendGroupForwardMsg(group_id int, forwardNode []map[string]any) {
	if group_id == 0 || len(forwardNode) == 0 {
		return
	}
	g := gson.New("")
	g.Set("action", "send_group_forward_msg")
	g.Set("params", map[string]any{"group_id": group_id, "messages": forwardNode})
	log.Info("[main] 发送合并转发到群聊 ", group_id, " ", gson.New(forwardNode).JSON("", ""))
	postMsg(g)
}

// 发送私聊合并转发消息
func sendPrivateForwardMsg(user_id int, forwardNode []map[string]any) {
	if user_id == 0 || len(forwardNode) == 0 {
		return
	}
	g := gson.New("")
	g.Set("action", "send_private_forward_msg")
	g.Set("params", map[string]any{"user_id": user_id, "messages": forwardNode})
	log.Info("[main] 发送合并转发到好友 ", user_id, " ", gson.New(forwardNode).JSON("", ""))
	postMsg(g)
}

type log2SuperUsers func(...any)

func (log2SU log2SuperUsers) Panic(msg ...any) {
	log2SU("[Panic] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Fatal(msg ...any) {
	log2SU("[NothingBot] [Fatal] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Error(msg ...any) {
	log2SU("[NothingBot] [Error] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Warn(msg ...any) {
	log2SU("[NothingBot] [Warn] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Info(msg ...any) {
	log2SU("[NothingBot] [Info] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Debug(msg ...any) {
	log2SU("[NothingBot] [Debug] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Trace(msg ...any) {
	log2SU("[NothingBot] [Trace] ", fmt.Sprint(msg...))
}

// 发送日志到超级用户
var log2SU log2SuperUsers = func(msg ...any) {
	sendMsg(suID, nil, "", msg...)
}

// 批量发送消息
func sendMsg(userID []int, groupID []int, at string, msg ...any) {
	if len(groupID) > 0 {
		for _, group := range groupID {
			sendGroupMsg(group, fmt.Sprint(msg...), at)
		}
	}
	if len(userID) > 0 {
		for _, user := range userID {
			sendPrivateMsg(user, msg...)
		}
	}
}

// 批量发送合并转发消息
func sendForwardMsg(userID []int, groupID []int, forwardNode []map[string]any) {
	if len(groupID) > 0 {
		for _, group := range groupID {
			sendGroupForwardMsg(group, forwardNode)
		}
	}
	if len(userID) > 0 {
		for _, user := range userID {
			sendPrivateForwardMsg(user, forwardNode)
		}
	}
}

// 根据上下文发送消息
func (ctx gocqMessage) sendMsg(msg ...any) {
	if ctx.message_type == "" || len(msg) == 0 {
		return
	}
	switch ctx.message_type {
	case "group":
		sendGroupMsg(ctx.group_id, msg...)
	case "private":
		sendPrivateMsg(ctx.user_id, msg...)
	}
}

// 根据上下文发送消息，带@
func (ctx gocqMessage) sendMsgAt(msg ...any) {
	if ctx.message_type == "" || len(msg) == 0 {
		return
	}
	switch ctx.message_type {
	case "group":
		sendGroupMsg(ctx.group_id, fmt.Sprint("[CQ:at,qq=", ctx.user_id, "]"), fmt.Sprint(msg...))
	case "private":
		sendPrivateMsg(ctx.user_id, msg...)
	}
}

// 根据上下文发送消息，带回复
func (ctx gocqMessage) sendMsgReply(msg ...any) {
	if ctx.message_type == "" || len(msg) == 0 {
		return
	}
	switch ctx.message_type {
	case "group":
		sendGroupMsg(ctx.group_id, fmt.Sprint("[CQ:reply,id=", ctx.message_id, "]"), fmt.Sprint(msg...))
	case "private":
		sendPrivateMsg(ctx.user_id, fmt.Sprint("[CQ:reply,id=", ctx.message_id, "]"), fmt.Sprint(msg...))
	}
}

// 根据上下文发送合并转发消息
func (ctx gocqMessage) sendForwardMsg(forwardNode []map[string]any) {
	if ctx.message_type == "" || len(forwardNode) == 0 {
		return
	}
	switch ctx.message_type {
	case "group":
		sendGroupForwardMsg(ctx.group_id, forwardNode)
	case "private":
		sendPrivateForwardMsg(ctx.user_id, forwardNode)
	}
}

// 正则完全匹配
func (ctx gocqMessage) regexpMustCompile(str string) (match [][]string) {
	return regexp.MustCompile(str).FindAllStringSubmatch(ctx.message, -1)
}

// 匹配超级用户
func (ctx gocqMessage) isSU() bool {
	for _, su := range suID {
		if ctx.user_id == su {
			return true
		}
	}
	return false
}

// 匹配消息来源
func (ctx gocqMessage) isGroup() bool {
	return ctx.message_type == "group"
}

// 匹配消息来源
func (ctx gocqMessage) isPrivate() bool {
	return ctx.message_type == "private"
}

// isPrivate() && isSU()
func (ctx gocqMessage) isPrivateSU() bool {
	return ctx.isPrivate() && ctx.isSU()
}

// 是否提及了Bot
func (ctx gocqMessage) isToMe() bool {
	atMe := func() bool {
		match := ctx.regexpMustCompile(fmt.Sprintf(`\[CQ:at,qq=%d]`, selfID))
		if len(match) > 0 {
			return true
		}
		return false
	}()
	callMe := func() bool {
		if botName == "" {
			return false
		}
		match := ctx.regexpMustCompile(botName)
		if len(match) > 0 {
			return true
		}
		return false
	}()
	return atMe || callMe || ctx.isPrivate() //私聊永远都是
}

// 群名片为空则返回昵称
func (ctx gocqMessage) getCardOrNickname() string {
	if ctx.sender_card != "" {
		return ctx.sender_card
	}
	return ctx.sender_nickname
}

// 快捷添加合并转发消息
func appendForwardNode(forwardNode []map[string]any, nodeData gocqNodeData) []map[string]any {
	timeS := nodeData.time
	name := nodeData.name
	uin := nodeData.uin
	if timeS == 0 {
		timeS = time.Now().Unix()
	}
	if name == "" {
		name = "NothingBot"
	}
	if uin == 0 {
		uin = selfID
	}
	for _, content_ := range nodeData.content {
		if content_ == "" {
			break
		}
		if nodeData.seq == "" {
			forwardNode = append(forwardNode, map[string]any{"type": "node", "data": map[string]any{
				"time": timeS, "name": name, "uin": uin, "content": content_}})
		} else {
			forwardNode = append(forwardNode, map[string]any{"type": "node", "data": map[string]any{"seq": nodeData.seq,
				"time": timeS, "name": name, "uin": uin, "content": content_}})
		}
	}
	return forwardNode
}

// 格式化时间戳至 x天x小时x分钟x秒
func timeFormat(timeS int64) string {
	time := int(timeS)
	days := time / (24 * 60 * 60)
	hours := (time / (60 * 60)) % 24
	minutes := (time / 60) % 60
	seconds := time % 60
	switch {
	case days > 0:
		return strconv.Itoa(days) + "天" + strconv.Itoa(hours) + "小时" + strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	case hours > 0:
		return strconv.Itoa(hours) + "小时" + strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	case minutes > 0:
		return strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	default:
		return strconv.Itoa(time) + "秒"
	}
}

// 初始化启动参数
func initFlag() {
	c := flag.String("c", "", "配置文件路径, 默认./config.yaml")
	flag.Parse()
	if *c != "" {
		configPath = *c
	}
}

// 初始化配置
func initConfig() {
	updateConfig := func() {
		suID = []int{}
		log.SetLevel(log.Level(v.GetInt("main.logLevel")))
		gocqUrl = v.GetString("main.websocket")
		suList := v.GetStringSlice("main.superUsers")
		if len(suList) > 0 {
			for _, each := range suList { //[]string to []int
				superUser, err := strconv.Atoi(each)
				if err != nil {
					log.Fatal("[init] superUsers内容格式有误 ", err)
				}
				suID = append(suID, superUser)
			}
		} else {
			log.Fatal("[init] 请指定至少一个超级用户")
		}
		log.Info("[init] superUsers: ", suID)
		go initCorpus()
		go initPush()
	}
	if configPath == "" {
		log.Info("[init] 读取默认配置文件: ./config.yaml")
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	} else {
		log.Info("[init] 读取自定义配置文件: ", configPath)
		v.SetConfigFile(configPath)
	}
	err := v.ReadInConfig()
	if err != nil {
		os.WriteFile("./config.yaml", []byte(defaultConfig), 0644)
		log.Fatal("[init] 缺失配置文件, 已生成默认配置, 请修改保存后重启程序, 参考: github.com/Miuzarte/NothingBot/blob/main/config.yaml")
	}
	v.WatchConfig()
	updateConfig()
	v.OnConfigChange(func(in fsnotify.Event) {
		log.Info("[main] 更新了配置文件")
		initCount++
		updateConfig()
		tempBlock <- struct{}{} //解除临时阻塞
	})
}

func main() {
	fmt.Print("        \n  Powered      \n         by    \n           GO  \n        \n")
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: timeLayout.M24,
		LogFormat:       "[%time%] [%lvl%] %msg%\n",
	})
	initFlag()
	initConfig()
	func() {
		go connect(gocqUrl)
		for {
			time.Sleep(time.Second)
			if heartbeatOK {
				return
			}
		}
	}()
	exitJobs()
}

// 结束运行前报告
func exitJobs() {
	signal.Notify(mainBlock, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGTERM)
	select {
	case <-mainBlock:
		runTime := timeFormat(time.Now().Unix() - startTime)
		log2SU.Info("[exit] 已下线", "\n此次运行时长：", runTime, "\n心跳包接收计数：", heartbeatCount, "\n心跳包丢失计数：", heartbeatLostCount)
		log.Info("[exit] 本次运行时长: ", runTime)
		log.Info("[exit] 心跳包接收计数: ", heartbeatCount)
		log.Info("[exit] 心跳包丢失计数: ", heartbeatLostCount)
	}
}
