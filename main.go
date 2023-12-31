package main

import (
	"NothinBot/EasyBot"
	"NothinBot/SimpleLogFormatter"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/ysmood/gson"
)

//go:embed default_config.yml
var defaultConfig string

var (
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

	unescape = strings.NewReplacer(
		"&amp;", "&", "&#44;", ",", "&#91;", "[", "&#93;", "]",
	)
	startTime            = time.Now().Unix()
	mainBlock            = make(chan os.Signal) //main阻塞
	v                    = viper.New()          //配置体
	customConfigPath     = ""                   //自定义配置文件路径
	configUpdateCount    = 0                    //config更新计数
	lastConfigChangeTime = time.Now().Unix()    //上次config保存时间
	bot                  = EasyBot.New()        //BOT
)

func loopTitle() {
	titles := []string{"O.o", "o.O", "O.O", "o.o"}
	previous := -1
	for {
		r := rand.Intn(4)
		if r == previous {
			continue
		}
		previous = r

		err := setConsoleTitle(titles[r])
		if err != nil {
			log.Error("[main] 设置窗口标题失败: ", err)
			return
		}
		time.Sleep(time.Second * 5)
	}
}

func main() {
	log.SetLevel(log.TraceLevel)
	log.SetFormatter(&SimpleLogFormatter.LogFormat{})
	go loopTitle()

	initFlag()
	initConfig()

	bot.
		SetLogLevel(log.TraceLevel).
		EnableOnlineNotification(true).
		EnableOfflineNotification(false).
		OnRecv(
			func(data *EasyBot.CQRecv) {
				// log.Debug("[NothingBot] gocq下发数据: ", string(data.Raw))
			},
		).
		OnTerminateUnexpectedly(
			func() {
				err := bot.Connect(true)
				if err != nil {
					log.Error("[main] 建立ws连接失败")
					return
				}
			},
		).
		OnMessage(
			func(msg *EasyBot.CQMessage) {
				handleMessage(msg)
			},
		).
		OnFriendRecall(
			func(fr *EasyBot.CQNoticeFriendRecall) {
				handleFriendRecall(fr)
			},
		).
		OnGroupRecall(
			func(gr *EasyBot.CQNoticeGroupRecall) {
				handleGroupRecall(gr)
			},
		).
		OnGroupCard(
			func(gc *EasyBot.CQNoticeGroupCard) {
				handleGroupCard(gc)
			},
		).
		OnGroupUpload(
			func(gu *EasyBot.CQNoticeGroupUpload) {
				handleGroupUpload(gu)
			},
		).
		OnOfflineFile(
			func(of *EasyBot.CQNoticeOfflineFile) {
				handleOfflineFile(of)
			},
		).
		OnRequestGroup(
			func(rg *EasyBot.CQRequestGroup) {
				handleRequestGroup(rg)
			},
		).
		OnPoke(
			func(pk *EasyBot.CQNoticeNotifyPoke) {
				handlePoke(pk)
			},
		)

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
	c := flag.String("c", "./config.yml", "配置文件路径, 默认为./config.yaml")
	flag.Parse()
	if *c != "" {
		customConfigPath = *c
	}
}

// 初始化配置
func initConfig() {
	before := func() { //只执行一次
		if customConfigPath == "" {
			customConfigPath = "./config.yml"
			log.Info("[Init] 读取默认配置文件: ", customConfigPath)
		} else {
			log.Info("[Init] 读取自定义配置文件: ", customConfigPath)
		}
		v.SetConfigFile(customConfigPath)
		if err := v.ReadInConfig(); err != nil {
			if err = os.WriteFile("./config.yml", []byte(defaultConfig), 0o664); err != nil {
				log.Fatal("[Init] 尝试写入默认配置文件时发生错误: ", err)
			}
			log.Info("[Init] 缺失配置文件, 已生成默认配置, 请修改保存后重启程序")
			os.Exit(0)
		}

		WatchFile(
			customConfigPath,
			func(event fsnotify.Event) {
				nowTime := time.Now().Unix()
				if nowTime-lastConfigChangeTime < 1 {
					log.Info("[main] 无视一次配置文件更新")
					return
				}
				lastConfigChangeTime = nowTime
				//log.Info("[main] 更新了配置文件")
				//configUpdateCount++
				//initConfig()
			},
		)
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
			log.Info("[Init] 机器人别称: ", bot.GetBotNickName())
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
	initLogin()
	initRecall()
	initPixiv()
	initSetu()
	initQianfan()
	initCache()
	initParse()
	initPush()
	initListen()

	initCorpus()
}

// 结束运行前报告
func exitJobs() {
	signal.Notify(mainBlock, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	<-mainBlock
	runTime := formatTime(int64(bot.GetRunningTime().Seconds()))
	err := bot.Log2SU.Info(
		"[Exit]",
		"\n此次运行时长：", runTime,
		"\n心跳包接收计数：", bot.HeartbeatCount,
		"\n心跳包丢失计数：", bot.HeartbeatLostCount,
		"\nShamrock重连计数", bot.RetryCount,
	)
	log.Info("[Exit] 此次运行时长: ", runTime)
	log.Info("[Exit] 心跳包接收计数: ", bot.HeartbeatCount)
	log.Info("[Exit] 心跳包丢失计数: ", bot.HeartbeatLostCount)
	log.Info("[Exit] Shamrock重连计数: ", bot.RetryCount)
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
		go checkDebug(ctx)
		go checkApiCallingTesting(ctx)
		go checkAIReply2077(ctx)
		go checkBotInternal(ctx)
		go checkCardParse(ctx)
		go checkCorpus(ctx)
		go checkBiliLogin(ctx)
		// go checkCookieUpdate(ctx)
		go checkParse(ctx)
		go checkSearch(ctx)
		go checkWhoAtMe(ctx)
		go checkRecall(ctx)
		go checkSetu(ctx)
		go checkPixiv(ctx)
		go checkBertVITS2(ctx)
		go checkInfo(ctx)
		go checkQRCode(ctx)
		// go checkDoLua(ctx)
	}(msg)
}

// Deprecated
func gocqIsLocalOrRemote() string {
	return "remote"
	if lor := v.GetString("main.localOrRemote"); lor == "local" || lor == "remote" {
		return lor
	} else {
		log.Fatal("[Init] config.main.localOrRemote: 错误的参数\"", lor, "\"")
	}
	return ""
}

func checkDebug(ctx *EasyBot.CQMessage) {
	if !ctx.IsToMe() {
		return
	}
	matches := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`DEBUG`))
	if len(matches) > 0 {
		b, err := json.Marshal(ctx)
		if err != nil {
			ctx.SendMsgReply(err.Error())
		}
		ctx.SendMsgReply(BytesToString(b))
	}
}

// 测试接口调用
func checkApiCallingTesting(ctx *EasyBot.CQMessage) {
	if !ctx.IsPrivateSU() {
		return
	}
	post := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`(?s)post\s*(.*)`))
	if len(post) > 0 {
		data := unescape.Replace(post[0][1])
		n, err := bot.Conn.Write(StringToBytes(data))
		if err != nil {
			ctx.SendMsg("err: ", err)
		} else {
			ctx.SendMsg("n: ", n)
		}
	}
	getMsg := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`get_msg\s?(.*)`))
	if len(getMsg) > 0 {
		msgId, _ := strconv.Atoi(getMsg[0][1])
		msg, err := bot.GetMsg(msgId)
		if err != nil {
			log.Error("err: ", err)
		}
		log.Debug(gson.New(msg).JSON("", ""))
	}
	downloadFile := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`download_file\s?(.*)`))
	if len(downloadFile) > 0 {
		url := downloadFile[0][1]
		file, err := bot.DownloadFile(url, 2, iheaders)
		if err != nil {
			log.Error("err: ", err)
		}
		log.Debug(file)
	}
	getLoginInfo := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`get_login_info`))
	if len(getLoginInfo) > 0 {
		ctx.SendMsg(bot.GetLoginInfo())
	}

	//escapeTest1 := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`转义测试`))
	//if len(escapeTest1) > 0 {
	//}
}

// formatTimeSimple 格式化秒级时间戳至 时分秒 x:x:x
func formatTimeSimple(timestamp int64) (format string) {
	h := (timestamp / (60 * 60)) % 24
	m := (timestamp / 60) % 60
	s := timestamp % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// formatTime 格式化秒级时间戳至 x天x小时x分钟x秒
func formatTime(timestamp int64) (format string) {
	if timestamp == 0 {
		return "0秒"
	}
	itoa := func(i int64) string {
		return strconv.Itoa(int(i))
	}
	days := timestamp / (24 * 60 * 60)
	hours := (timestamp / (60 * 60)) % 24
	minutes := (timestamp / 60) % 60
	seconds := timestamp % 60
	switch {
	case days > 0:
		format += itoa(days) + "天"
		fallthrough
	case hours > 0:
		format += itoa(hours) + "小时"
		fallthrough
	case minutes > 0:
		format += itoa(minutes) + "分钟"
		fallthrough
	default:
		if seconds != 0 {
			format += itoa(seconds) + "秒"
		}
	}
	return format
}

// formatTimeMs 格式化毫秒级时间戳至 x天x小时x分钟x秒x毫秒
func formatTimeMs(timestamp int64) (format string) {
	if timestamp == 0 {
		return "0毫秒"
	}
	itoa := func(i int64) string {
		return strconv.Itoa(int(i))
	}
	milliseconds := timestamp % 1000
	seconds := (timestamp / 1000) % 60
	minutes := (timestamp / (1000 * 60)) % 60
	hours := (timestamp / (1000 * 60 * 60)) % 24
	days := timestamp / (1000 * 60 * 60 * 24)
	switch {
	case days > 0:
		format += itoa(days) + "天"
		fallthrough
	case hours > 0:
		format += itoa(hours) + "小时"
		fallthrough
	case minutes > 0:
		format += itoa(minutes) + "分钟"
		fallthrough
	case seconds > 0:
		format += itoa(seconds) + "秒"
		fallthrough
	default:
		if milliseconds != 0 {
			format += itoa(milliseconds) + "毫秒"
		}
	}
	return format
}

func formatNumber(number float64, decimalSave int, trimTailZeros bool) string {
	symbol := fmt.Sprint("%." + strconv.Itoa(decimalSave) + "f")
	s := fmt.Sprintf(symbol, number)
	if trimTailZeros {
		s = strings.TrimRight(s, "0")
	}
	return s
}

// ConvertToCSV 格式化至逗号分隔符格式，带换行
func ConvertToCSV(items ...any) (outputWithNewLine string) {
	count := len(items)
	for i := 0; i < count; i++ {
		outputWithNewLine += fmt.Sprint(items[i])
		if i < count-1 {
			outputWithNewLine += ","
		}
	}
	return outputWithNewLine + "\n"
}

// BytesToString 没有内存开销的转换
// https://github.com/wdvxdr1123/ZeroBot/blob/main/utils/helper/helper.go
func BytesToString(b []byte) (s string) {
	return *(*string)(unsafe.Pointer(&b))
}

// StringToBytes 没有内存开销的转换
// https://github.com/wdvxdr1123/ZeroBot/blob/main/utils/helper/helper.go
func StringToBytes(s string) (b []byte) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh.Data = sh.Data
	bh.Len = sh.Len
	bh.Cap = sh.Len
	return b
}

var (
	//go:linkname modkernel32 golang.org/x/sys/windows.modkernel32
	modkernel32         *windows.LazyDLL
	procSetConsoleTitle = modkernel32.NewProc("SetConsoleTitleW")
)

//go:linkname errnoErr golang.org/x/sys/windows.errnoErr
func errnoErr(e syscall.Errno) error

func setConsoleTitle(title string) (err error) {
	var p0 *uint16
	p0, err = syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	r1, _, e1 := syscall.Syscall(procSetConsoleTitle.Addr(), 1, uintptr(unsafe.Pointer(p0)), 0, 0)
	if r1 == 0 {
		err = errnoErr(e1)
	}
	return
}

func checkDir(path string) (err error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.Mkdir(path, 0o755)
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
		log.Info(
			"[NothingBot] 群 ", gr.GroupID, " 中 ", gr.UserID, " 撤回了一条消息: ", msg.RawMessage, " (", gr.MessageID, ")",
		)
	} else {
		log.Info("[NothingBot] 群 ", gr.GroupID, " 中 ", gr.UserID, " 撤回了一条消息, ID: ", gr.MessageID, ")")
	}
}

// 群名片变更
func handleGroupCard(gc *EasyBot.CQNoticeGroupCard) {
	if gc.CardOld == "" {
		return
	}
	avatar := bot.Utils.Format.ImageUrl(
		fmt.Sprintf(
			"https://q.qlogo.cn/headimg_dl?dst_uin=%d&spec=640&img_type=jpg", gc.UserID,
		), "cache=0",
	)
	err := bot.SendGroupMsg(
		gc.GroupID, fmt.Sprint(
			avatar, gc.UserID, " 变更了群名片：\n", gc.CardOld, " -> ", gc.CardNew,
		),
	)
	if err != nil {
		log.Error("[main] 消息发送失败: ", err)
		return
	}
}

// 群文件上传
func handleGroupUpload(gu *EasyBot.CQNoticeGroupUpload) {
	GroupUploadParse(gu)
}

// 离线文件上传
func handleOfflineFile(of *EasyBot.CQNoticeOfflineFile) {
	err := bot.SendPrivateMsg(
		of.UserID, fmt.Sprintf(
			"%s（%.2fMB）\n%s",
			of.File.Name, float64(of.File.Size)/1024.0/1024.0,
			of.File.Url,
		),
	)
	if err != nil {
		log.Error("[main] 消息发送失败: ", err)
		return
	}
}

// 加群请求/邀请
func handleRequestGroup(rg *EasyBot.CQRequestGroup) {
	switch rg.SubType {
	case "add":
		err := bot.Log2SU.Info("[NothingBot] 群", rg.GroupID, "收到了来自", rg.UserID, "的加群申请：", rg.Comment)
		if err != nil {
			log.Error("[main] 消息发送失败: ", err)
			return
		}
	case "invite":
		err := bot.Log2SU.Info("[NothingBot] 被", rg.UserID, "邀请至群", rg.GroupID)
		if err != nil {
			log.Error("[main] 消息发送失败: ", err)
			return
		}
	}
}

// 戳一戳
func handlePoke(pk *EasyBot.CQNoticeNotifyPoke) {
	if pk.TargetID == bot.GetSelfId() && pk.OperatorID != pk.TargetID && pk.GroupID != 0 {
		err := bot.SendGroupMsg(pk.GroupID, "[NothingBot] 在一条消息内只at我两次可以获取帮助信息～")
		if err != nil {
			log.Error("[main] 消息发送失败: ", err)
			return
		}
	}
}
