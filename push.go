package main

import (
	"fmt"
	"strconv"

	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var disconnected bool
var configChanged bool
var cookie string
var liveListUID []int
var liveList []int
var liveStateList = make(map[string]liveState)

type liveState struct {
	STATE int
	TIME  int64
}

var streamState = struct {
	UNKNOWN int
	OFFLINE int
	ONLINE  int
	ROTATE  int
}{
	UNKNOWN: -1,
	OFFLINE: 0,
	ONLINE:  1,
	ROTATE:  2,
}

func initPush() { //初始化推送
	cookie = v.GetString("push.settings.cookie")
	log.Traceln("[push] cookie:\n", cookie)
	if cookie == "" || cookie == "<nil>" {
		log.Warnln("[push] 未配置cookie!")
	} else {
		go dynamicMonitor()
	}
	disconnected = true
	go liveMonitor()
	v.OnConfigChange(func(in fsnotify.Event) {
		cookie = v.GetString("push.settings.cookie")
		configChanged = true
	})
}

func getBaseline() string { //返回baseline用于监听更新
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all").
		WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getBaseline().ihttp请求错误:", err) }).ToString()
	g := gson.NewFrom(body)
	update_baseline := g.Get("data.update_baseline").Str()
	if g.Get("code").Int() != 0 || g.Get("data.update_baseline").Nil() {
		log.Errorln("[push] update_baseline获取错误:", body)
		return "-1"
	}
	return update_baseline
}

func getUpdate(update_baseline string) string { //是否有新动态
	body := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all/update").
		WithAddQuery("update_baseline", update_baseline).WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[bilibili] getUpdate().ihttp请求错误:", err) }).ToString()
	g := gson.NewFrom(body)
	update_num := g.Get("data.update_num").Str()
	if g.Get("code").Int() != 0 || g.Get("data.update_num").Nil() {
		log.Errorln("[push] getUpdate获取错误:", body)
		return "-1"
	}
	return update_num
}

func cookieChecker() bool { //检测cookie有效性
	body := ihttp.New().WithUrl("https://passport.bilibili.com/x/passport-login/web/cookie/info").
		WithHeaders(iheaders).WithCookie(cookie).
		Get().WithError(func(err error) { log.Errorln("[bilibili] cookieChecker().ihttp请求错误:", err) }).ToString()
	g := gson.NewFrom(body)
	switch g.Get("code").Int() {
	case 0:
		log.Warnln("[push] cookie未过期但触发了有效性检测")
		sendMsg2Admin("[WARN] [push] cookie未过期但触发了有效性检测")
		return true
	case -101:
		log.Errorln("[push] cookie已过期")
		sendMsg2Admin("[ERROR] [push] cookie已过期")
		return false
	default:
		log.Errorln("[push] 非正常cookie状态:", body)
		sendMsg2Admin("[ERROR] [push] 非正常cookie状态：" + body)
		return false
	}
}

func dynamicMonitor() { //监听动态流
	var (
		update_num      = "0"
		update_baseline = "0"
		new_baseline    = "0"
		failureCount    = 0
	)
	update_baseline = getBaseline()
	if update_baseline == "-1" {
		log.Errorln("[push] 获取update_baseline时出现错误")
	}
	for {
		update_num = getUpdate(update_baseline)
		switch update_num {
		case "-1":
			errInfo := fmt.Sprintf("[push] 获取update_num时出现错误    update_num = %s    update_baseline = %s", update_num, update_baseline)
			log.Errorln(errInfo)
			if !cookieChecker() {
				<-tempBlock
				failureCount = 0
			}
			failureCount++
			if failureCount >= 10 {
				log.Errorln("[push] 尝试更新失败", failureCount, "次, 暂停拉取动态更新")
				sendMsg2Admin("[push] 连续更新失败十次但cookie未失效，已暂停拉取动态更新")
				<-tempBlock
				failureCount = 0
			}
			duration := time.Duration(time.Second * time.Duration(failureCount) * 30)
			log.Errorln("[push] 获取更新失败", failureCount, "次, 将在", duration, "秒后重试")
			time.Sleep(time.Second * duration)
		case "0":
			log.Debugln("[push] 没有新动态    update_num =", update_num, "   update_baseline =", update_baseline)
		default:
			new_baseline = getBaseline()
			log.Infoln("[push] 有新动态！    update_num =", update_num, "   update_baseline =", update_baseline, "=>", new_baseline)
			update_baseline = new_baseline
			go func(dynamicID string) { //异步检测推送
				rawJson := getDynamicJson(dynamicID)
				switch rawJson.Get("code").Int() {
				case 4101131: //动态已删除，不推送
					log.Warnln("[push] 记录到一条已删除的动态, dynamicID =", dynamicID)
					sendMsg2Admin(fmt.Sprintf("[push] 记录到一条已删除的动态，dynamicID = %s", dynamicID))
					break
				case 500: //加载错误，请稍后再试
					break
				default:
					mainJson := rawJson.Get("data.item")
					log.Debugln("[push] mainJson:", mainJson.JSON("", ""))
					dynamicChecker(mainJson)
				}
			}(new_baseline)
		}
		time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64("push.settings.dynamicUpdateInterval")*1000)))
	}
}

func dynamicChecker(mainJson gson.JSON) { //mainJson：data.item
	uid := mainJson.Get("modules.module_author.mid").Int()
	name := mainJson.Get("modules.module_author.name").Str()
	dynamicType := mainJson.Get("type").Str()
	for i := 0; i < len(v.GetStringSlice("push.list")); i++ { //循环匹配
		log.Tracef("push.list.%d.uid: %d", i, v.GetInt(fmt.Sprintf("push.list.%d.uid", i)))
		uidMatch := uid == v.GetInt(fmt.Sprintf("push.list.%d.uid", i))
		var filterMatch bool
		if len(v.GetStringSlice(fmt.Sprintf("push.list.%d.filter", i))) == 0 {
			filterMatch = true
		} else {
			for j := 0; j < len(v.GetStringSlice(fmt.Sprintf("push.list.%d.filter", i))); j++ { //匹配推送过滤
				if dynamicType == v.GetString(fmt.Sprintf("push.list.%d.filter.%d", i, j)) {
					filterMatch = true
				}
			}
		}
		if uidMatch && filterMatch {
			log.Infoln("[push] 处于推送列表:", name, uid)
			at, userID, groupID := sendListGen(i)
			sendMsg(userID, groupID, at, formatDynamic(mainJson))
			return
		}
	}
	log.Infoln("[push] 不处于推送列表:", name, uid)
	return
}

func initLiveList() { //初始化直播监听列表
	liveListUID = []int{} //清空
	liveList = []int{}
	j := 0
	k := 0
	for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
		var uid int
		var roomID int
		if v.GetInt(fmt.Sprintf("push.list.%d.live", i)) != 0 {
			uid = v.GetInt(fmt.Sprintf("push.list.%d.uid", i))
			roomID = v.GetInt(fmt.Sprintf("push.list.%d.live", i))
			liveListUID = append(liveListUID, uid)
			liveList = append(liveList, roomID)
			g, ok := getRoomJsonUID(strconv.Itoa(uid)).Gets("data", strconv.Itoa(uid))
			if ok {
				liveStateList[strconv.Itoa(roomID)] = liveState{ //开播状态可以获取开播时间
					STATE: g.Get("live_status").Int(),
					TIME:  int64(g.Get("live_time").Int())}
			} else {
				liveStateList[strconv.Itoa(roomID)] = liveState{
					STATE: streamState.UNKNOWN,
					TIME:  time.Now().Unix()}
			}
			log.Debugln("[push] uid为", uid, "的直播间", roomID, "加入监听列表  目前状态:", liveStateList[strconv.Itoa(roomID)].STATE)
			k += 1
		}
		j += 1
	}
	log.Infoln("[push] 动态推送", j, "个")
	log.Infoln("[push] 直播间监听", k, "个")
}

func liveMonitor() { //建立监听连接
	for {
		log.Debugln("[push] 直播监听    disconnected:", disconnected, "   configChanged:", configChanged)
		if disconnected || configChanged {
			disconnected = false
			configChanged = false
			log.Infoln("[push] 开始建立监听连接")
			initLiveList()
			log.Traceln("[push] len(liveList):", len(liveList))
			for i := 0; i < len(liveList); i++ {
				log.Traceln("[push] 建立监听连接    uid:", liveListUID[i], "   roomID:", liveList[i])
				go connectDanmu(liveListUID[i], liveList[i])
				time.Sleep(time.Second * 1)
			}
		}
		time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64("push.settings.resetCheckInterval")*1000)))
	}
}

func liveChecker(pktJson gson.JSON, uid string, roomID string) { //判断数据包类型
	minimumInterval := int64(v.GetFloat64("push.settings.livePushMinimumInterval"))
	cmd := pktJson.Get("cmd").Str()
	switch cmd {
	case "LIVE":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprintf("push.list.%d.live", i))) {
				at, userID, groupID := sendListGen(i)
				if liveStateList[roomID].STATE == streamState.ONLINE {
					switch {
					case time.Now().Unix()-liveStateList[roomID].TIME < minimumInterval:
						log.Warnln("[push] 屏蔽了一次间隔小于", minimumInterval, "秒的开播推送")
						return
					}
				}
				go func(roomID string, time int64) {
					liveStateList[roomID] = liveState{ //记录开播
						STATE: streamState.ONLINE,
						TIME:  time}
				}(roomID, time.Now().Unix())
				log.Infoln("[push] 推送", uid, "的直播间", roomID, "开播")
				log.Infoln("[push] 记录开播时间:", time.Unix(int64(liveStateList[roomID].TIME), 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Errorln("[push] 获取直播间信息失败")
					sendMsg(userID, groupID, at, fmt.Sprintf("[NothingBot] [ERROR] [push] 推送%s的直播间%s开播失败", uid, roomID))
					return
				}
				name := fmt.Sprintf("%s开播了！\n", roomJson.Get("uname").Str())
				cover := fmt.Sprintf("[CQ:image,file=%s]\n", roomJson.Get("cover_from_user").Str())
				title := fmt.Sprintf("%s\n", roomJson.Get("title").Str())
				link := fmt.Sprintf("live.bilibili.com/%s", roomID)
				msg := name + cover + title + link
				sendMsg(userID, groupID, at, msg)
				return
			}
		}
	case "PREPARING":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprintf("push.list.%d.live", i))) {
				at, userID, groupID := sendListGen(i)
				if liveStateList[roomID].STATE == streamState.OFFLINE || liveStateList[roomID].STATE == streamState.ROTATE {
					switch {
					case time.Now().Unix()-liveStateList[roomID].TIME < minimumInterval:
						log.Warnln("[push] 屏蔽了一次间隔小于", minimumInterval, "秒的下播推送")
						return
					}
				}
				defer func(roomID string, time int64) {
					liveStateList[roomID] = liveState{ //记录下播
						STATE: streamState.OFFLINE,
						TIME:  time}
				}(roomID, time.Now().Unix())
				log.Infoln("[push] 推送", uid, "的直播间", roomID, "下播")
				log.Infoln("[push] 缓存的开播时间:", time.Unix(int64(liveStateList[roomID].TIME), 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Errorln("[push] 获取直播间信息失败")
					sendMsg(userID, groupID, at, fmt.Sprintf("[NothingBot] [ERROR] [push] 推送%s的直播间%s下播失败", uid, roomID))
					return
				}
				name := fmt.Sprintf("%s下播了~\n", roomJson.Get("uname").Str())
				cover := fmt.Sprintf("[CQ:image,file=%s]\n", roomJson.Get("keyframe").Str())
				title := fmt.Sprintf("%s\n", roomJson.Get("title").Str())
				duration := func() string {
					if liveStateList[roomID].TIME != 0 {
						return "本次直播持续了" + timeFormat(time.Now().Unix()-liveStateList[roomID].TIME)
					} else {
						return "未记录本次开播时间"
					}
				}()
				msg := name + cover + title + duration
				sendMsg(userID, groupID, at, msg)
				return
			}
		}
	case "ROOM_CHANGE":
		if !v.GetBool("push.settings.roomChangeInfo") {
			return
		}
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprintf("push.list.%d.live", i))) {
				at, userID, groupID := sendListGen(i)
				log.Infoln("[push] 推送", uid, "的直播间", roomID, "房间信息更新")
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Errorln("[push] 获取直播间信息失败")
					sendMsg(userID, groupID, at, fmt.Sprintf("[NothingBot] [ERROR] [push] 推送%s的直播间%s房间信息更新失败", uid, roomID))
					return
				}
				area := fmt.Sprintf("%s - %s\n", //分区
					roomJson.Get("area_v2_parent_name").Str(),
					roomJson.Get("area_v2_name").Str())
				name := fmt.Sprintf("%s更改了房间信息\n", roomJson.Get("uname").Str())
				title := fmt.Sprintf("房间名：%s", roomJson.Get("title").Str())
				link := fmt.Sprintf("live.bilibili.com/%s", roomID)
				msg := name + title + area + link
				sendMsg(userID, groupID, at, msg)
				return
			}
		}
	}
}

func sendListGen(i int) (string, []int, []int) { //生成发送队列
	//读StringSlice再转成IntSlice实现同时支持输入单个和多个数据
	userID := []int{}
	userList := v.GetStringSlice(fmt.Sprintf("push.list.%d.user", i))
	if len(userList) != 0 {
		for _, each := range userList {
			user, err := strconv.Atoi(each)
			if err != nil {
				log.Errorln("[strconv.Atoi]", err)
				sendMsg2Admin(fmt.Sprintf("[ERROR] [strconv.Atoi] %v", err))
			}
			userID = append(userID, user)
		}
	}
	log.Debugln("[push] 推送用户:", userID)
	groupID := []int{}
	groupList := v.GetStringSlice(fmt.Sprintf("push.list.%d.group", i))
	if len(groupList) != 0 {
		for _, each := range groupList {
			group, err := strconv.Atoi(each)
			if err != nil {
				log.Errorln("[strconv.Atoi]", err)
				sendMsg2Admin(fmt.Sprintf("[ERROR] [strconv.Atoi] %v", err))
			}
			groupID = append(groupID, group)
		}
	}
	log.Debugln("[push] 推送群组:", groupID)
	at := ""
	atList := v.GetStringSlice(fmt.Sprintf("push.list.%d.at", i))
	if len(atList) != 0 {
		at += "\n"
		for _, each := range atList {
			at += "[CQ:at,qq=" + each + "]"
		}
	}
	log.Debugln("[push] at:", atList)
	return at, userID, groupID
}
