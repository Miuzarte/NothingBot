package main

import (
	"NothinBot/EasyBot"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	"github.com/ysmood/gson"
)

//go:embed default_config.yml
var defaultConfig string

type Message struct {
	CQMessage *EasyBot.CQMessage
}

type Config struct {
	Main struct {
		WsUrl      string
		SuperUsers []int
	}
}

var (
	timeLayout = struct {
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
	iheaders = map[string]string{
		"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
		"Dnt":                "1",
		"Origin":             "https://www.bilibili.com",
		"Referer":            "https://www.bilibili.com/",
		"Sec-Ch-Ua":          "\"Not/A)Brand\";v=\"24\", \"Microsoft Edge\";v=\"116\", \"Chromium\";v=\"116\"",
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": "\"Windows\"",
		"Sec-Fetch-Dest":     "document",
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Site":     "none",
		"Sec-Fetch-User":     "?1",
		"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36 Edg/116.0.1938.62",
	}

	startTime         = time.Now().Unix()
	mainBlock         = make(chan os.Signal) //main阻塞
	v                 = viper.New()          //配置体
	customConfigPath  = ""                   //自定义配置文件路径
	configUpdateCount = 0                    //
	bot               = EasyBot.New()        //BOT
)

func main() {
	log.SetLevel(log.TraceLevel)
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: timeLayout.M24,
		LogFormat:       "[%time%] [%lvl%] %msg%\n",
	})

	initFlag()
	initConfig()

	bot.
		SetLogLevel(log.TraceLevel).
		EnableOnlineNotification(true).
		EnableOfflineNotification(false).
		OnData(func(data *EasyBot.CQRecv) {
			// log.Debug("[NothingBot] gocq下发数据: ", string(data.Raw))
		}).
		OnTerminateUnexpectedly(func() {
			bot.Connect(true)
		}).
		OnMessage(func(msg *EasyBot.CQMessage) {
			handleMessage(msg)
		}).
		OnFriendRecall(func(fr *EasyBot.CQNoticeFriendRecall) {
			handleFriendRecall(fr)
		}).
		OnGroupRecall(func(gr *EasyBot.CQNoticeGroupRecall) {
			handleGroupRecall(gr)
		}).
		OnGroupCard(func(gc *EasyBot.CQNoticeGroupCard) {
			handleGroupCard(gc)
		}).
		OnGroupUpload(func(gu *EasyBot.CQNoticeGroupUpload) {
			handleGroupUpload(gu)
		}).
		OnOfflineFile(func(of *EasyBot.CQNoticeOfflineFile) {
			handleOfflineFile(of)
		}).
		OnRequestGroup(func(rg *EasyBot.CQRequestGroup) {
			handleRequestGroup(rg)
		}).
		OnPoke(func(pk *EasyBot.CQNoticeNotifyPoke) {
			handlePoke(pk)
		})

	err := bot.Connect(true)
	if err != nil {
		log.Error("bot.Connect err: ", err)
	}
	defer bot.Disconnect()

	initModules()
	exitJobs()
}

// 初始化启动参数
func initFlag() {
	c := flag.String("c", "", "配置文件路径, 默认为./config.yaml")
	flag.Parse()
	if *c != "" {
		customConfigPath = *c
	}
}

// 初始化配置
func initConfig() {
	before := func() { //只执行一次
		if customConfigPath == "" {
			log.Info("[Init] 读取默认配置文件: ./config.yml")
			v.SetConfigFile("./config.yml")
		} else {
			log.Info("[Init] 读取自定义配置文件: ", customConfigPath)
			v.SetConfigFile(customConfigPath)
		}
		if err := v.ReadInConfig(); err != nil {
			if err = os.WriteFile("./config.yml", []byte(defaultConfig), 0664); err != nil {
				log.Fatal("[Init] 尝试写入默认配置文件时发生错误: ", err)
			}
			log.Info("[Init] 缺失配置文件, 已生成默认配置, 请修改保存后重启程序")
			os.Exit(0)
		}
	}

	after := func() { //热更新也执行
		log.SetLevel(log.Level(v.GetInt("main.logLevel")))

		bot.SetWsUrl(v.GetString("main.wsUrl"))

		if suList := v.GetStringSlice("main.superUsers"); len(suList) > 0 {
			for _, each := range suList { //[]string to []int
				if each == "" {
					continue
				}
				su, err := strconv.Atoi(each)
				if err != nil {
					log.Fatal("[Init] main.superUsers 内容格式有误 err: ", err)
				}
				bot.AddSU(su)
			}
			log.Info("[Init] superUsers: ", bot.GetSU())
		} else {
			log.Fatal("[Init] 请指定至少一个超级用户")
		}

		if nickName := v.GetStringSlice("main.nickName"); len(nickName) > 0 {
			bot.AddNickName(nickName...)
			log.Info("[Init] 机器人别称: ", bot.GetNickName())
		}

		if privateBanList := v.GetStringSlice("main.ban.private"); len(privateBanList) > 0 {
			for _, each := range privateBanList {
				if each == "" {
					continue
				}
				uid, err := strconv.Atoi(each)
				if err != nil {
					log.Fatal("[Init] main.ban.private 内容格式有误 err: ", err)
				}
				bot.AddPrivateBan(uid)
			}
			log.Info("[Init] 私聊屏蔽列表: ", bot.GetPrivateBan())
		}

		if groupBanList := v.GetStringSlice("main.ban.group"); len(groupBanList) > 0 {
			for _, each := range groupBanList {
				if each == "" {
					continue
				}
				gid, err := strconv.Atoi(each)
				if err != nil {
					log.Fatal("[Init] main.ban.group 内容格式有误 err: ", err)
				}
				bot.AddGroupBan(gid)
			}
			log.Info("[Init] 群聊屏蔽列表: ", bot.GetGroupBan())
		}

	}

	if configUpdateCount == 0 {
		before()
		after()
		v.WatchConfig()
	} else {
		after()
	}
}

func initModules() {
	initRecall()
	initPixiv()
	initSetu()
	initQianfan()
	initCache()
	initParse()
	initPush()

	initCorpus()
}

// 结束运行前报告
func exitJobs() {
	signal.Notify(mainBlock, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	<-mainBlock
	runTime := timeFormat(bot.GetRunningTime())
	err := bot.Log2SU.Info("[Exit]",
		"\n此次运行时长：", runTime,
		"\n心跳包接收计数：", bot.HeartbeatCount,
		"\n心跳包丢失计数：", bot.HeartbeatLostCount,
		"\ngo-cqhttp重连计数", bot.RetryCount)
	log.Info("[Exit] 此次运行时长: ", runTime)
	log.Info("[Exit] 心跳包接收计数: ", bot.HeartbeatCount)
	log.Info("[Exit] 心跳包丢失计数: ", bot.HeartbeatLostCount)
	log.Info("[Exit] go-cqhttp重连计数: ", bot.RetryCount)
	if err != nil {
		log.Error("[Exit] 下线消息发送失败, err: ", err)
	}
}

func handleMessage(msg *EasyBot.CQMessage) {
	switch msg.MessageType {
	case "private":
		// log.Info("[NothingBot] 收到 ", msg.Sender.NickName, "(", msg.UserID, ") 的消息(", msg.MessageID, "): ", msg.RawMessage)
	case "group":
		// log.Info("[NothingBot] 在 ", msg.GroupID, " 收到 ", msg.Sender.CardName, "(", msg.Sender.NickName, " ", msg.UserID, ") 的群聊消息(", msg.MessageID, "): ", msg.RawMessage)
	}
	go func(ctx *EasyBot.CQMessage) {
		go checkApiCallingTesting(ctx)
		go checkAIReply2077(ctx)
		go checkBotInternal(ctx)
		go checkCardParse(ctx)
		go checkCorpus(ctx)
		go checkCookieUpdate(ctx)
		go checkParse(ctx)
		go checkSearch(ctx)
		go checkWhoAtMe(ctx)
		go checkRecall(ctx)
		go checkSetu(ctx)
		go checkPixiv(ctx)
		go checkBertVITS2(ctx)
		go checkInfo(ctx)
	}(msg)
}

// 测试接口调用
func checkApiCallingTesting(ctx *EasyBot.CQMessage) {
	if !ctx.IsPrivateSU() {
		return
	}
	get_msg := ctx.RegexpMustCompile(`get_msg\s?(.*)`)
	if len(get_msg) > 0 {
		msgId, _ := strconv.Atoi(get_msg[0][1])
		msg, err := bot.GetMsg(msgId)
		if err != nil {
			log.Error("err: ", err)
		}
		log.Debug(gson.New(msg).JSON("", ""))
	}
}

// 格式化时间戳至 x天x小时x分钟x秒
func timeFormat(timestamp int64) string {
	itoa := func(i int64) string {
		return strconv.Itoa(int(i))
	}
	days := timestamp / (24 * 60 * 60)
	hours := (timestamp / (60 * 60)) % 24
	minutes := (timestamp / 60) % 60
	seconds := timestamp % 60
	switch {
	case days > 0:
		return itoa(days) + "天" + itoa(hours) + "小时" + itoa(minutes) + "分钟" + itoa(seconds) + "秒"
	case hours > 0:
		return itoa(hours) + "小时" + itoa(minutes) + "分钟" + itoa(seconds) + "秒"
	case minutes > 0:
		return itoa(minutes) + "分钟" + itoa(seconds) + "秒"
	default:
		return itoa(timestamp) + "秒"
	}
}

func checkDir(path string) (err error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.Mkdir(path, 0755)
		if err != nil {
			log.Error("无法创建文件夹: ", err)
		} else {
			log.Info("文件夹 ", path, " 创建成功")
		}
	} else {
		log.Debug("文件夹 ", path, " 已存在")
	}
	return
}

func handleFriendRecall(fr *EasyBot.CQNoticeFriendRecall) {
	msg, err := bot.FetchPrivateMsg(fr.UserID, fr.MessageID)
	if err != nil {
		log.Warn("[NothingBot] 调用 bot.FetchPrivateMsg() 时发生错误")
	}
	if msg != nil {
		log.Info("[NothingBot] ", fr.UserID, " 撤回了一条消息: ", msg.RawMessage, " (", fr.MessageID, ")")
	} else {
		log.Info("[NothingBot] ", fr.UserID, " 撤回了一条消息, ID: ", fr.MessageID)
	}
}

func handleGroupRecall(gr *EasyBot.CQNoticeGroupRecall) {
	msg, err := bot.FetchGroupMsg(gr.GroupID, gr.MessageID)
	if err != nil {
		log.Warn("[NothingBot] 调用 bot.FetchGroupMsg() 时发生错误")
	}
	if msg != nil {
		log.Info("[NothingBot] 群 ", gr.GroupID, " 中 ", gr.UserID, " 撤回了一条消息: ", msg.RawMessage, " (", gr.MessageID, ")")
	} else {
		log.Info("[NothingBot] 群 ", gr.GroupID, " 中 ", gr.UserID, " 撤回了一条消息, ID: ", gr.MessageID, ")")
	}
}

// 群名片变更
func handleGroupCard(gc *EasyBot.CQNoticeGroupCard) {
	avatar := bot.Utils.Format.ImageUrl(fmt.Sprintf(
		"http://q.qlogo.cn/headimg_dl?dst_uin=%d&spec=640&img_type=jpg", gc.UserID))
	bot.SendGroupMsg(gc.GroupID, fmt.Sprint(
		avatar, gc.UserID, " 变更了群名片：\n", gc.CardOld, " -> ", gc.CardNew))
}

// 群文件上传
func handleGroupUpload(gu *EasyBot.CQNoticeGroupUpload) {
	bot.SendGroupMsg(gu.GroupID, fmt.Sprintf(
		"%d上传了新的群文件！\n%s（%.2fMB）\n%s",
		gu.UserID,
		gu.File.Name, float64(gu.File.Size)/1024.0/1024.0,
		gu.File.Url))
}

// 离线文件上传
func handleOfflineFile(of *EasyBot.CQNoticeOfflineFile) {
	bot.SendPrivateMsg(of.UserID, fmt.Sprintf(
		"%s（%.2fMB）\n%s",
		of.File.Name, float64(of.File.Size)/1024.0/1024.0,
		of.File.Url))
}

// 加群请求/邀请
func handleRequestGroup(rg *EasyBot.CQRequestGroup) {
	switch rg.SubType {
	case "add":
		bot.Log2SU.Info("[NothingBot] 群", rg.GroupID, "收到了来自", rg.UserID, "的加群申请：", rg.Comment)
	case "invite":
		bot.Log2SU.Info("[NothingBot] 被", rg.UserID, "邀请至群", rg.GroupID)
	}
}

// 戳一戳
func handlePoke(pk *EasyBot.CQNoticeNotifyPoke) {
	if pk.TargetID == bot.GetSelfID() && pk.SenderID != pk.TargetID && pk.GroupID != 0 {
		bot.SendGroupMsg(pk.GroupID, "[NothingBot] 在一条消息内只at我两次可以获取帮助信息～")
	}
}
