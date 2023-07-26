package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"github.com/ysmood/gson"
	"golang.org/x/net/websocket"
)

const errJson404 string = `{"code": 404,"message": "错误: 网络请求异常","ttl": 1}`

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
	logLever   = DebugLevel      //日志等级
	configPath = "./config.yaml" //配置路径
	v          = viper.New()     //配置体
	adminID    = []int{}         //超管QQ
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
	"application/json, text/plain, */*",
	"zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	"1",
	"https://t.bilibili.com",
	"https://t.bilibili.com/",
	"\"Not.A/Brand\";v=\"8\", \"Chromium\";v=\"114\", \"Microsoft Edge\";v=\"114\"",
	"?0",
	"\"Windows\"",
	"empty",
	"cors",
	"same-site",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36 Edg/114.0.1823.86",
}

type gocqHeartbeat struct {
	selfID   int
	interval int
}

type gocqLifecycle struct {
	selfID     int
	postMethod int
}

type gocqPoke struct {
	groupID  int
	senderID int
	targetID int
}

type gocqNotice struct {
	notice_type           string //通知类型 https://docs.go-cqhttp.org/reference/data_struct.html#post-notice-type
	notice_notify_subtype string //系统通知子类型: "honor"群荣誉变更, "poke"戳一戳, "lucky_king"群红包幸运王, "title"群成员头衔变更
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

var gocqConn *websocket.Conn

func connect(url string) {
	for {
		c, err := websocket.Dial(url, "", "http://127.0.0.1")
		if err == nil {
			log.Infoln("[main] 与go-cqhttp建立ws连接成功")
			gocqConn = c
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
		if err != nil {
			fmt.Println(err)
			continue
		}
		jsonPost := gson.NewFrom(rawPost)
		log.Debugln("[gocq] raw:", rawPost)
		var msg gocqMessage
		var msgSent gocqMessageSent
		var request gocqRequest
		var notice gocqNotice
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
			log.Infoln("[gocq] 收到消息:", msg.message)
			go corpusChecker(msg)
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
						groupID:  jsonPost.Get("group_id").Int(),
						senderID: jsonPost.Get("sender_id").Int(),
						targetID: jsonPost.Get("target_id").Int(),
					}
					log.Infoln("[gocq] 收到", poke.senderID, "对", poke.targetID, "的戳一戳")
				default:
					log.Infoln("[gocq] notice", rawPost)
					log.Infoln("[gocq] notice.notify.sub_type:", jsonPost.Get("sub_type").Str())
				}
			default:
				log.Infoln("[gocq] notice", rawPost)
			}
			notice = gocqNotice{}
			_ = notice
		case "meta_event":
			switch jsonPost.Get("meta_event_type").Str() { //"lifecycle"/"heartbeat"
			case "heartbeat":
				heartbeat := gocqHeartbeat{
					selfID:   jsonPost.Get("self_id").Int(),
					interval: jsonPost.Get("interval").Int(),
				}
				log.Infoln("[gocq] heartbeat", heartbeat)
			case "lifecycle":
				lifecycle := gocqLifecycle{
					selfID:     jsonPost.Get("self_id").Int(),
					postMethod: jsonPost.Get("_post_method").Int(),
				}
				log.Infoln("[gocq] lifecycle", lifecycle)
			default:
				log.Infoln("[gocq] meta_event", jsonPost)
			}
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

func sendMsg(msg string, at string, userID []int, groupID []int) {
	if len(userID) != 0 { //有私聊发私聊，不带at
		for _, user := range userID {
			g := gson.NewFrom("")
			g.Set("action", "send_private_msg")
			g.Set("params", map[string]any{
				"group_id": user,
				"message":  msg,
			})
			gocqConn.Write([]byte(g.JSON("", "")))
			log.Infoln("[push] 发送消息到用户:", user, "   内容:", msg)
		}
	}
	if len(groupID) != 0 { //有群聊也发群聊，消息追加at
		msg += at
		for _, group := range groupID {
			g := gson.NewFrom("")
			g.Set("action", "send_group_msg")
			g.Set("params", map[string]any{
				"group_id": group,
				"message":  msg,
			})
			gocqConn.Write([]byte(g.JSON("", "")))
			log.Infoln("[push] 发送消息到群聊:", group, "   内容:", msg)
		}
	}
	return
}

func sendMsgSingle(msg string, user int, group int) {
	if user != 0 { //有私聊发私聊，不带at
		g := gson.NewFrom("")
		g.Set("action", "send_private_msg")
		g.Set("params", map[string]any{
			"user_id": user,
			"message": msg,
		})
		gocqConn.Write([]byte(g.JSON("", "")))
		log.Infoln("[push] 发送消息到用户:", user, "   内容:", msg)
	}
	if group != 0 { //有群聊也发群聊，消息追加at
		g := gson.NewFrom("")
		g.Set("action", "send_group_msg")
		g.Set("params", map[string]any{
			"group_id": group,
			"message":  msg,
		})
		gocqConn.Write([]byte(g.JSON("", "")))
		log.Infoln("[push] 发送消息到群聊:", group, "   内容:", msg)
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

func main() {
	fmt.Println("        \n  Powered      \n         by    \n           GO  \n        ")
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[%time%] [%lvl%] %msg%\n",
	})
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.SetConfigFile(configPath)
	v.ReadInConfig()
	v.WatchConfig()
	switch v.GetInt("main.logLevel") {
	case 0:
		log.SetLevel(log.PanicLevel)
	case 1:
		log.SetLevel(log.FatalLevel)
	case 2:
		log.SetLevel(log.ErrorLevel)
	case 3:
		log.SetLevel(log.WarnLevel)
	case 4:
		log.SetLevel(log.InfoLevel)
	case 5:
		log.SetLevel(log.DebugLevel)
	case 6:
		log.SetLevel(log.TraceLevel)
	}
	adminList := v.GetStringSlice("main.admin")
	if len(adminList) != 0 {
		for _, each := range adminList { //[]string to []int
			admin, _ := strconv.Atoi(each)
			adminID = append(adminID, admin)
		}
	}
	log.Infoln("[init] 管理者QQ:", adminID)
	initCorpus()
	initPush()
	connect(v.GetString("main.websocket"))
}
