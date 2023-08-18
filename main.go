package main

import (
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

const defaultConfig string = `main: #冷更新
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
  #语料库、推送配置请参照: https://github.com/Miuzarte/NothingBot/blob/main/config.yaml
corpus: #热更新
# - #模板
#   regexp: "" #正则表达式
#   reply: "" #回复内容
#   scene: "" #触发场景 "a"/"all" / "g"/"group" / "p"/"private"
#   delay:  #延迟回复（秒）  支持小数
push: #热更新，但最起码不要在5s内保存多次，发起直播监听连接需要时间，直播间越多越久
  settings:
    dynamicUpdateInterval: 3 #拉取更新间隔
    resetCheckInterval: 15 #直播监听重连检测间隔（秒）
    #通过拉取动态流进行推送，必须设置B站cookie，且需要关注想要推送的up
    cookie: ""
  list:
  # - #模板
  # uid: #up的uid  int ONLY
  # live: #up的直播间号，存在则监听并推送直播  int ONLY
  # user: #推送到的用户  int / []int
  # group: #推送到的群组  int / []int
  # at: #推送到群组时消息末尾at的人  int / []int    --------（有点鸡肋，之后想想怎么改又能不用群号当键）
  # filter: #此键存在内容时仅推送包含在内的动态类型（白名单） []string
  #     - "DYNAMIC_TYPE_WORD" #文本动态（包括投票/预约）
  #     - "DYNAMIC_TYPE_DRAW" #图文动态（包括投票/预约）
  #     - "DYNAMIC_TYPE_AV" #视频投稿（包括动态视频）
  #     - "DYNAMIC_TYPE_ARTICLE" #文章投稿
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
	startTime          = time.Now().Unix()                 //启动时间
	gocqUrl            = ""                                //websocketurl
	gocqConn           *websocket.Conn                     //
	mainBlock          = make(chan os.Signal)              //main阻塞
	tempBlock          = make(chan struct{})               //其他阻塞 热更新时重置
	logLever           = DebugLevel                        //日志等级
	configPath         = "./config.yaml"                   //配置路径
	v                  = viper.New()                       //配置体
	connLost           = make(chan struct{})               //
	reconnectCount     = 0                                 //
	heartbeatChecking  = false                             //
	heartbeatOK        = false                             //
	heartbeatCount     = 0                                 //
	heartbeatLostCount = 0                                 //
	heartbeatChan      = make(chan struct{})               //
	selfID             = 0                                 //机器人QQ
	suID               = []int{}                           //超管QQ
	msgTableGroup      = make(map[int]map[int]gocqMessage) //group_id:msg_id:msg
	msgTableFriend     = make(map[int]map[int]gocqMessage) //user_id:msg_id:msg
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
	L24C: "2006年01月02日  15时04分05秒",
	M24:  "01/02 15:04:05",
	M24C: "01月02日  15时04分05秒",
	S24:  "02 15:04:05",
	S24C: "02日  15时04分05秒",
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

type gocqMessage struct {
	message_type    string //消息类型: "private"私聊消息, "group"群消息
	sub_type        string //消息子类型: "friend"好友, "normal"群聊, "anonymous"匿名, "group_self"群中自身发送, "group"群临时会话, "notice"系统提示, "connect"建立ws连接
	time            int    //时间戳
	timeF           string //格式化的时间
	user_id         int    //来源用户
	group_id        int    //来源群聊
	message_id      int    //消息ID
	message_seq     int    //消息序列
	raw_message     string //消息内容
	message         string //消息内容
	messageF        string //具体化回复后的消息内容
	sender_nickname string //QQ昵称
	sender_card     string //群名片
	sender_rold     string //群身份: "owner", "admin", "member"
	recalled        bool   //是否被撤回
	operator_id     int    //撤回者ID
	atWho           []int  //@的人
}

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

func heartbeatCheck(interval int) {
	log.Info("[main] 开始监听心跳")
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

func msgEntity(p gson.JSON) string { //具体化回复  go-cqhttp没设置extra-reply-data: true时需要使用
	msg := p.Get("message").Str()
	reg := regexp.MustCompile(`\[CQ:reply\,id=(.*?)\]`).FindAllStringSubmatch(msg, -1)
	if len(reg) > 0 {
		replyID_str := reg[0][1]
		replyID_int, _ := strconv.Atoi(replyID_str)
		replyMsg := gocqMessage{}
		reply := ""
		switch p.Get("message_type").Str() {
		case "group":
			replyMsg = msgTableGroup[p.Get("group_id").Int()][replyID_int]
		case "private":
			replyMsg = msgTableFriend[p.Get("user_id").Int()][replyID_int]
		}
		reply = fmt.Sprint("[CQ:reply,qq=", replyMsg.user_id, ",time=", replyMsg.time, ",text=", replyMsg.message, "]")
		msg = strings.ReplaceAll(msg, fmt.Sprint("[CQ:reply,id=", reg[0][1], "]"), reply)
	}
	return msg
}

func postHandler(rawPost string) {
	log.Trace("[gocq] 上报: ", rawPost)
	p := gson.NewFrom(rawPost)
	var request gocqRequest
	switch p.Get("post_type").Str() { //上报类型: "message"消息, "message_sent"消息发送, "request"请求, "notice"通知, "meta_event"
	case "message":
		msg := gocqMessage{ //消息内容
			message_type: p.Get("message_type").Str(),
			sub_type:     p.Get("sub_type").Str(),
			time:         p.Get("time").Int(),
			timeF:        time.Unix(int64(p.Get("time").Int()), 0).Format(timeLayout.T24),
			user_id:      p.Get("user_id").Int(),
			group_id:     p.Get("group_id").Int(),
			message_id:   p.Get("message_id").Int(),
			message_seq:  p.Get("message_seq").Int(),
			raw_message:  p.Get("raw_message").Str(),
			message:      p.Get("message").Str(),
			//messageF: msgEntity(p),        //go-cqhttp中extra-reply-data: false时需要使用
			sender_nickname: p.Get("sender.nickname").Str(),
			sender_card:     p.Get("sender.card").Str(),
			sender_rold:     p.Get("sender.role").Str(),
			atWho: func(msg string) []int { //@的人
				reg := regexp.MustCompile(`\[CQ:at,qq=(.*?)\]`).FindAllStringSubmatch(msg, -1)
				atWho := []int{}
				if len(reg) != 0 {
					for _, v := range reg {
						atID, err := strconv.Atoi(v[1])
						if err == nil {
							atWho = append(atWho, atID)
						}
					}
				}
				return atWho
			}(p.Get("message").Str()),
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
		}(msg)
	case "message_sent":
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
				msg.recalled = true //标记撤回
				msg.operator_id = recall.operator_id
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
				msg.recalled = true //标记撤回
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
				log.Info("[gocq] 收到 ", poke.sender_id, " 对 ", poke.target_id, " 的戳一戳")
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

type log2SuperUsers func(...any)

func (log2SU log2SuperUsers) Panic(msg ...any) {
	log2SU("[Panic] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Fatal(msg ...any) {
	log2SU("[Fatal] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Error(msg ...any) {
	log2SU("[Error] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Warn(msg ...any) {
	log2SU("[Warn] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Info(msg ...any) {
	log2SU("[Info] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Debug(msg ...any) {
	log2SU("[Debug] ", fmt.Sprint(msg...))
}

func (log2SU log2SuperUsers) Trace(msg ...any) {
	log2SU("[Trace] ", fmt.Sprint(msg...))
}

var log2SU log2SuperUsers = func(msg ...any) {
	sendMsg(suID, []int{}, "", "[NothingBot] ", fmt.Sprint(msg...))
}

func sendMsg(userID []int, groupID []int, at string, msg ...any) {
	if len(msg) == 0 {
		return
	}
	if len(groupID) != 0 {
		for _, group := range groupID {
			sendGroupMsg(group, msg, at)
		}
	}
	if len(userID) != 0 {
		for _, user := range userID {
			sendPrivateMsg(user, msg...)
		}
	}
	return
}

func sendMsgCTX(ctx gocqMessage, msg ...any) { //根据上下文发送消息
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

func sendMsgAtCTX(ctx gocqMessage, msg ...any) { //根据上下文发送消息，带@
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

func sendMsgReplyCTX(ctx gocqMessage, msg ...any) { //根据上下文发送消息，带回复
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

func sendGroupMsg(group_id int, msg ...any) {
	if group_id == 0 || len(msg) == 0 {
		return
	}
	g := gson.NewFrom("")
	g.Set("action", "send_group_msg")
	g.Set("params", map[string]any{"group_id": group_id, "message": fmt.Sprint(msg...)})
	postMsg(g)
	return
}

func sendPrivateMsg(user_id int, msg ...any) {
	if user_id == 0 || len(msg) == 0 {
		return
	}
	g := gson.NewFrom("")
	g.Set("action", "send_private_msg")
	g.Set("params", map[string]any{"user_id": user_id, "message": fmt.Sprint(msg...)})
	postMsg(g)
	return
}

func sendForwardMsgCTX(ctx gocqMessage, forwardNode []map[string]any) {
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

func sendGroupForwardMsg(group_id int, forwardNode []map[string]any) {
	if group_id == 0 || len(forwardNode) == 0 {
		return
	}
	g := gson.NewFrom("")
	g.Set("action", "send_group_forward_msg")
	g.Set("params", map[string]any{"group_id": group_id, "messages": forwardNode})
	postMsg(g)
}

func sendPrivateForwardMsg(user_id int, forwardNode []map[string]any) {
	if user_id == 0 || len(forwardNode) == 0 {
		return
	}
	g := gson.NewFrom("")
	g.Set("action", "send_private_forward_msg")
	g.Set("params", map[string]any{"user_id": user_id, "messages": forwardNode})
	postMsg(g)
}

func postMsg(msg gson.JSON) {
	if heartbeatOK {
		if msg.Get("params.user_id").Int() != 0 {
			log.Info("[main] 发送消息到好友 ", msg.Get("params.user_id").Int(), "    ", msg.Get("params.message"))
		}
		if msg.Get("params.group_id").Int() != 0 {
			log.Info("[main] 发送消息到群聊 ", msg.Get("params.group_id").Int(), "    ", msg.Get("params.message"))
		}
		gocqConn.Write([]byte(msg.JSON("", "")))
	} else {
		log.Error("[main] 未连接到go-cqhttp")
	}
}

func matchSU(user_id int) bool {
	for _, superUser := range suID {
		if superUser == user_id {
			return true
		}
	}
	return false
}

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

func initConfig() {
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetConfigFile(configPath)
	_, err := os.Stat(configPath)
	if err != nil {
		os.WriteFile(configPath, []byte(defaultConfig), 0644)
		log.Error("缺失配置文件，已生成默认配置 ", configPath, " 请修改保存后重启程序, 参考: github.com/Miuzarte/NothingBot/blob/main/config.yaml")
		os.Exit(0)
	}
	v.ReadInConfig()
	v.WatchConfig()
	v.OnConfigChange(func(in fsnotify.Event) {
		log.SetLevel(log.Level(v.GetInt("main.logLevel")))
		suID = []int{}
		suList := v.GetStringSlice("main.superUsers")
		if len(suList) != 0 {
			for _, each := range suList { //[]string to []int
				superUser, err := strconv.Atoi(each)
				if err != nil {
					log.Fatal("[strconv.Atoi] ", err)
				}
				suID = append(suID, superUser)
			}
		}
		tempBlock <- struct{}{} //解除阻塞
	})
}

func main() {
	fmt.Println("        \n  Powered      \n         by    \n           GO  \n        ")
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: timeLayout.M24,
		LogFormat:       "[%time%] [%lvl%] %msg%\n",
	})
	initConfig()
	gocqUrl = v.GetString("main.websocket")
	log.SetLevel(log.Level(v.GetInt("main.logLevel")))
	suList := v.GetStringSlice("main.superUsers")
	if len(suList) != 0 {
		for _, each := range suList { //[]string to []int
			superUser, err := strconv.Atoi(each)
			if err != nil {
				log.Fatal("[strconv.Atoi] ", err)
			}
			suID = append(suID, superUser)
		}
	} else {
		log.Fatal("[main] 请指定至少一个超级用户")
	}
	log.Info("[init] superUsers: ", suID)
	func() {
		go connect(gocqUrl)
		for {
			if heartbeatOK {
				return
			}
		}
	}()
	initCorpus()
	initPush()
	signal.Notify(mainBlock, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGKILL)
	select {
	case <-mainBlock:
		runTime := timeFormat(time.Now().Unix() - startTime)
		log2SU.Info(fmt.Sprint("[exit] 已下线\n此次运行时长：", runTime))
		log.Info("[exit] 本次运行时长: ", runTime)
		log.Info("[exit] 心跳包接收计数: ", heartbeatCount)
		log.Info("[exit] 心跳包丢失计数: ", heartbeatLostCount)
	}
}
