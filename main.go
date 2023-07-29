package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"github.com/ysmood/gson"
	"golang.org/x/net/websocket"
)

const errJson404 string = `{"code": 404,"message": "错误: 网络请求异常","ttl": 1}`

const defaultConfig string = `main: #冷更新
  websocket: "ws://127.0.0.1:9820" #go-cqhttp
  admin:  #管理者QQ  int / []int
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
	PanicLevel = 0
	FatalLevel = 1
	ErrorLevel = 2
	WarnLevel  = 3
	InfoLevel  = 4
	DebugLevel = 5
	TraceLevel = 6
)

var (
	gocqConn          *websocket.Conn   //
	block             = make(chan any)  //main阻塞
	logLever          = DebugLevel      //日志等级
	configPath        = "./config.yaml" //配置路径
	v                 = viper.New()     //配置体
	heartbeatInterval = 0               //
	heartbeatLive     = 0               //
	heartbeatOK       = false           //
	adminID           = []int{}         //超管QQ
)

var headers = struct { //请求头
	Accept          string
	AcceptLanguage  string
	Dnt             string
	Origin          string
	Referer         string
	SecChUa         string
	SecChUaMobile   string
	SecChUaPlatform string
	SecFetchDest    string
	SecFetchMode    string
	SecFetchSite    string
	UserAgent       string
}{
	Accept:          "application/json, text/plain, */*",
	AcceptLanguage:  "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	Dnt:             "1",
	Origin:          "https://t.bilibili.com",
	Referer:         "https://t.bilibili.com/",
	SecChUa:         "\"Not/A)Brand\";v=\"99\", \"Microsoft Edge\";v=\"115\", \"Chromium\";v=\"115\"",
	SecChUaMobile:   "?0",
	SecChUaPlatform: "\"Windows\"",
	SecFetchDest:    "empty",
	SecFetchMode:    "cors",
	SecFetchSite:    "same-site",
	UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36 Edg/115.0.1901.183",
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

type gocqRequest struct {
	request_type string //请求类型: "friend"好友请求, "group"群请求
}

type gocqMessageSent struct {
}

type gocqMessage struct {
	message_type    string //消息类型: "private"私聊消息, "group"群消息
	sub_type        string //消息子类型: "friend"好友, "normal"群聊, "anonymous"匿名, "group_self"群中自身发送, "group"群临时会话, "notice"系统提示, "connect"建立ws连接
	time            int    //时间戳
	timeF           string //格式化的时间
	user_id         int    //来源用户
	group_id        int    //来源群聊
	message_id      int    //消息ID
	message_seq     int    //消息片
	raw_message     string //消息内容
	message         string //消息内容
	sender_nickname string //QQ昵称
	sender_card     string //群昵称
	sender_rold     string //群身份: "owner", "admin", "member"
}

func connect(url string) {
	for {
		c, err := websocket.Dial(url, "", "http://127.0.0.1")
		if err == nil {
			log.Infoln("[main] 与go-cqhttp建立ws连接成功")
			heartbeatOK = true
			heartbeatLive = 5
			gocqConn = c
			go heartbeatCheck()
			break
		}
		log.Errorln("[main] 与go-cqhttp建立ws连接失败, 5秒后重试")
		time.Sleep(time.Second * 5)
	}
	for {
		var rawPost string
		err := websocket.Message.Receive(gocqConn, &rawPost)
		if err == io.EOF {
			break
		}
		if !heartbeatOK {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		jsonPost := gson.NewFrom(rawPost)
		log.Traceln("[gocq] 上报:", rawPost)
		var msg gocqMessage
		var msgSent gocqMessageSent
		var request gocqRequest
		switch jsonPost.Get("post_type").Str() { //上报类型: "message"消息, "message_sent"消息发送, "request"请求, "notice"通知, "meta_event"
		case "message":
			msg = gocqMessage{ //消息内容
				message_type:    jsonPost.Get("message_type").Str(),
				sub_type:        jsonPost.Get("sub_type").Str(),
				time:            jsonPost.Get("time").Int(),
				timeF:           time.Unix(int64(jsonPost.Get("time").Int()), 0).Format("2006-01-02 15:04:05"),
				user_id:         jsonPost.Get("user_id").Int(),
				group_id:        jsonPost.Get("group_id").Int(),
				message_id:      jsonPost.Get("message_id").Int(),
				message_seq:     jsonPost.Get("message_seq").Int(),
				raw_message:     jsonPost.Get("raw_message").Str(),
				message:         jsonPost.Get("message").Str(),
				sender_nickname: jsonPost.Get("sender.nickname").Str(),
				sender_card:     jsonPost.Get("sender.card").Str(),
				sender_rold:     jsonPost.Get("sender.role").Str(),
			}
			switch msg.message_type {
			case "private":
				log.Infoln("[gocq] 收到", msg.sender_nickname, "(", msg.user_id, ")的私聊消息", msg.message)
			case "group":
				log.Infoln("[gocq] 在", msg.group_id, "收到", msg.sender_card, "(", msg.sender_nickname, msg.user_id, ")的群聊消息", msg.message)
			}
			go checkCorpus(msg)
			go checkParse(msg)
		case "message_sent":
			msgSent = gocqMessageSent{}
			_ = msgSent
			log.Infoln("[gocq] message_sent", rawPost)
		case "request":
			request = gocqRequest{}
			_ = request
			log.Infoln("[gocq] request", rawPost)
		case "notice":
			switch jsonPost.Get("notice_type").Str() { //https://docs.go-cqhttp.org/reference/data_struct.html#post-notice-type
			case "notify":
				switch jsonPost.Get("sub_type").Str() {
				case "poke":
					poke := gocqPoke{
						group_id:  jsonPost.Get("group_id").Int(),
						sender_id: jsonPost.Get("sender_id").Int(),
						target_id: jsonPost.Get("target_id").Int(),
					}
					log.Infoln("[gocq] 收到", poke.sender_id, "对", poke.target_id, "的戳一戳")
				default:
					log.Infoln("[gocq] notice", rawPost)
					log.Infoln("[gocq] notice.notify.sub_type:", jsonPost.Get("sub_type").Str())
				}
			default:
				log.Infoln("[gocq] notice", rawPost)
			}
		case "meta_event":
			switch jsonPost.Get("meta_event_type").Str() { //"lifecycle"/"heartbeat"
			case "heartbeat":
				heartbeat := gocqHeartbeat{
					self_id:  jsonPost.Get("self_id").Int(),
					interval: jsonPost.Get("interval").Int(),
				}
				heartbeatLive = 5
				heartbeatInterval = heartbeat.interval
				log.Debugln("[gocq] heartbeat", heartbeat)
			case "lifecycle":
				lifecycle := gocqLifecycle{
					self_id:      jsonPost.Get("self_id").Int(),
					_post_method: jsonPost.Get("_post_method").Int(),
				}
				log.Infoln("[gocq] lifecycle", lifecycle)
			default:
				log.Infoln("[gocq] meta_event", jsonPost)
			}
		default:
			log.Debugln("[gocq] raw:", rawPost)
		}
	}
}

func heartbeatCheck() {
	time.Sleep(time.Second * 60)
	for {
		time.Sleep(time.Millisecond * (time.Duration(heartbeatInterval)))
		heartbeatLive -= 1
		if heartbeatLive <= 0 {
			heartbeatOK = false
			log.Errorln("[main] 连续五次丢失心跳包, 5秒后尝试重连")
			time.Sleep(time.Second * 5)
			go connect(v.GetString("main.websocket"))
			break
		}
	}
}

func httpsGet(url string, cookie string) string {
	log.Traceln("[push] 发起了请求:", url)
	method := "GET"
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Errorln("[push] httpsGet().http.NewRequest()发生错误:", err)
		return errJson404
	}
	req.Header.Add("Accept", headers.Accept)
	req.Header.Add("Accept-Language", headers.AcceptLanguage)
	req.Header.Add("Cookie", cookie)
	req.Header.Add("Dnt", headers.Dnt)
	req.Header.Add("Origin", headers.Origin)
	req.Header.Add("Referer", headers.Referer)
	req.Header.Add("Sec-Ch-Ua", headers.SecChUa)
	req.Header.Add("Sec-Ch-Ua-Mobile", headers.SecChUaMobile)
	req.Header.Add("Sec-Ch-Ua-Platform", headers.SecChUaPlatform)
	req.Header.Add("Sec-Fetch-Dest", headers.SecFetchDest)
	req.Header.Add("Sec-Fetch-Mode", headers.SecFetchMode)
	req.Header.Add("Sec-Fetch-Site", headers.SecFetchSite)
	req.Header.Add("User-Agent", headers.UserAgent)
	res, err := client.Do(req)
	if err != nil {
		log.Errorln("[push] httpsGet().client.Do()发生错误:", err)
		return errJson404
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Errorln("[push] httpsGet().ioutil.ReadAll()发生错误:", err)
		return errJson404
	}
	return string(body)
}

func sendMsg2Admin(msg string) {
	if msg == "" {
		return
	}
	sendMsg(adminID, []int{}, "", msg)
}

func sendMsg(userID []int, groupID []int, at string, msg string) {
	if msg == "" {
		return
	}
	if len(userID) != 0 { //有私聊发私聊，不带at
		for _, user := range userID {
			sendMsgSingle(user, 0, msg)
		}
	}
	if len(groupID) != 0 { //有群聊也发群聊，消息追加at
		msg += at
		for _, group := range groupID {
			sendMsgSingle(0, group, msg)
		}
	}
	return
}

func sendMsgSingle(user int, group int, msg string) {
	if msg == "" {
		return
	}
	if user != 0 {
		g := gson.NewFrom("")
		g.Set("action", "send_private_msg")
		g.Set("params", map[string]any{
			"user_id": user,
			"message": msg,
		})
		gocqConn.Write([]byte(g.JSON("", "")))
		log.Infoln("[main] 发送消息到用户:", user, "   内容:", msg)
	}
	if group != 0 {
		g := gson.NewFrom("")
		g.Set("action", "send_group_msg")
		g.Set("params", map[string]any{
			"group_id": group,
			"message":  msg,
		})
		gocqConn.Write([]byte(g.JSON("", "")))
		log.Infoln("[main] 发送消息到群聊:", group, "   内容:", msg)
	}
	return
}

func timeFormatter(timeS int) string {
	seconds := timeS % 60 / 1
	minutes := ((timeS - (seconds * 1)) % 3600) / 60
	hours := ((timeS - ((seconds * 1) + (minutes * 60))) % 216000) / 3600
	days := (timeS - ((seconds * 1) + (minutes * 60) + (hours * 3600))) / 86400
	switch {
	case days > 0:
		return strconv.Itoa(days) + "天" + strconv.Itoa(hours) + "小时" + strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	case hours > 0:
		return strconv.Itoa(hours) + "小时" + strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	case minutes > 0:
		return strconv.Itoa(minutes) + "分钟" + strconv.Itoa(seconds) + "秒"
	default:
		return strconv.Itoa(timeS) + "秒"
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
		log.Errorln("缺失配置文件，已生成默认配置", configPath, "请修改保存后重启程序")
		log.Errorln("参考: github.com/Miuzarte/NothingBot/blob/main/config.yaml")
		os.Exit(0)
	}
	v.ReadInConfig()
	v.WatchConfig()
}

func main() {
	fmt.Println("        \n  Powered      \n         by    \n           GO  \n        ")
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[%time%] [%lvl%] %msg%\n",
	})
	initConfig()
	log.SetLevel(log.Level(v.GetInt("main.logLevel")))
	adminList := v.GetStringSlice("main.admin")
	if len(adminList) != 0 {
		for _, each := range adminList { //[]string to []int
			admin, err := strconv.Atoi(each)
			if err != nil {
				log.Panicln("[strconv.Atoi]", err)
			}
			adminID = append(adminID, admin)
		}
	}
	log.Infoln("[init] 管理者QQ:", adminID)
	go connect(v.GetString("main.websocket"))
	initCorpus()
	initPush()
	<-block
}
