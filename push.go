package main

import (
	"fmt"
	"strconv"

	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var disconnected bool
var configChanged bool
var cookie string
var dynamicHistrory = make(map[string]string)
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

// 初始化推送
func initPush() {
	disconnected = true
	configChanged = true
	cookie = v.GetString("push.settings.cookie")
	if initCount != 0 {
		time.Sleep(time.Second * 2 * time.Duration(v.GetInt("push.settings.dynamicUpdateInterval")))
	}
	go liveMonitor()
	log.Trace("[push] cookie:\n", cookie)
	if cookie == "" || cookie == "<nil>" {
		log.Warn("[push] 未配置cookie!")
	} else {
		go dynamicMonitor()
	}
}

// 初始化baseline用于监听更新
func getBaseline() (update_baseline string) {
	g, err := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all").
		WithHeaders(iheaders).WithCookie(cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getBaseline().ihttp请求错误: ", err)
	}
	update_baseline = g.Get("data.update_baseline").Str()
	if g.Get("code").Int() != 0 || g.Get("data.update_baseline").Nil() {
		log.Error("[push] update_baseline获取错误: ", g.JSON("", ""))
		return "-1"
	}
	return
}

// 是否有新动态
func getUpdate(update_baseline string) (update_num string) {
	g, err := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all/update").
		WithAddQuery("update_baseline", update_baseline).WithHeaders(iheaders).WithCookie(cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] getUpdate().ihttp请求错误: ", err)
	}
	update_num = g.Get("data.update_num").Str()
	if g.Get("code").Int() != 0 || g.Get("data.update_num").Nil() {
		log.Error("[push] getUpdate获取错误: ", g.JSON("", ""))
		return "-1"
	}
	return
}

// 检测cookie有效性
func cookieChecker() bool {
	g, err := ihttp.New().WithUrl("https://passport.bilibili.com/x/passport-login/web/cookie/info").
		WithHeaders(iheaders).WithCookie(cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] cookieChecker().ihttp请求错误: ", err)
	}
	switch g.Get("code").Int() {
	case 0:
		log.Warn("[push] cookie未过期但触发了有效性检测")
		log2SU.Warn("[push] cookie未过期但触发了有效性检测")
		return true
	case -101:
		log.Error("[push] cookie已过期")
		log2SU.Error("[push] cookie已过期")
		return false
	default:
		log.Error("[push] 非正常cookie状态: ", g.JSON("", ""))
		log2SU.Error(fmt.Sprint("[push] 非正常cookie状态：", g.JSON("", "")))
		return false
	}
}

// 监听动态流
func dynamicMonitor() {
	var (
		update_num      = "0"
		update_baseline = "0"
		new_baseline    = "0"
		failureCount    = 0
	)
	update_baseline = getBaseline()
	if update_baseline != "-1" {
		log.Info("[push] update_baseline: ", update_baseline)
	}
	for {
		if configChanged {
			break
		}
		update_num = getUpdate(update_baseline)
		switch update_num {
		case "-1":
			log.Error("[push] 获取update_num时出现错误    update_num = ", update_num, "  update_baseline = ", update_baseline)
			if !cookieChecker() {
				<-tempBlock
				failureCount = 0
			}
			failureCount++
			if failureCount >= 10 {
				log.Error("[push] 尝试更新失败 ", failureCount, " 次, 暂停拉取动态更新")
				log2SU.Error(fmt.Sprint("[push] 连续更新失败 ", failureCount, " 次，暂停拉取动态更新"))
				<-tempBlock
				failureCount = 0
			}
			duration := time.Duration(time.Second * time.Duration(failureCount) * 30)
			log.Error("[push] 获取更新失败 ", failureCount, " 次, 将在 ", duration, " 后重试")
			time.Sleep(duration)
		case "0":
			log.Debug("[push] 没有新动态    update_num = ", update_num, "  update_baseline = ", update_baseline)
		default:
			new_baseline = getBaseline()
			if update_baseline == new_baseline { //重复动态
				log.Debug("[push] 假新动态    update_num = ", update_num, "  update_baseline = ", update_baseline)
			} else { //非重复动态
				log.Info("[push] 有新动态!    update_num = ", update_num, "  update_baseline = ", update_baseline, " => ", new_baseline)
				update_baseline = new_baseline
				go func(dynamicID string) { //检测推送
					rawJson := getDynamicJson(dynamicID)
					mainJson := rawJson.Get("data.item")
					log.Debug("[push] mainJson: ", mainJson.JSON("", ""))
					switch rawJson.Get("code").Int() {
					case 4101131: //动态已删除，不推送
						if dynamicHistrory[dynamicID] != "" {
							log.Info("[push] 明确记录到一条来自 ", dynamicHistrory[dynamicID], " 的已删除动态 ", dynamicID)
							log2SU.Info(fmt.Sprint("[push] 明确记录到一条来自 ", dynamicHistrory[dynamicID], " 的已删除动态 ", dynamicID))
						}
						break
					case 500: //加载错误，请稍后再试
						if dynamicHistrory[dynamicID] == "" { //检测是否为重复动态
							go func(dynamicID string) {
								for i := 0; i < 3; i++ { //重试三次
									log.Warn("[push] (RETRY_", i+1, ") 将在10秒后重试动态 ", dynamicID)
									time.Sleep(time.Second * 10)
									rawJson := getDynamicJson(dynamicID)
									mainJson := rawJson.Get("data.item")
									log.Debug("[push] (RETRY) mainJson: ", mainJson.JSON("", ""))
									if rawJson.Get("code").Int() == 0 {
										log.Warn("[push] (RETRY) 成功获取动态 ", dynamicID)
										dynamicChecker(mainJson)
										dynamicHistrory[dynamicID] = mainJson.Get("modules.module_author.name").Str() //记录历史
										break
									}
								}
							}(dynamicID)
						}
					case 0: //正常
						if dynamicHistrory[dynamicID] == "" { //检测是否为重复动态
							dynamicChecker(mainJson)
							dynamicHistrory[dynamicID] = mainJson.Get("modules.module_author.name").Str() //记录历史
						}
					default:
						break
					}
				}(new_baseline)
			}
		}
		time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64("push.settings.dynamicUpdateInterval")*1000)))
	}
}

// 匹配推送列表.Get("data.item")
func dynamicChecker(mainJson gson.JSON) {
	uid := mainJson.Get("modules.module_author.mid").Int()
	name := mainJson.Get("modules.module_author.name").Str()
	dynamicType := mainJson.Get("type").Str()
	if uid != 0 {
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ { //循环匹配
			log.Tracef("push.list.%d.uid: %d", i, v.GetInt(fmt.Sprint("push.list.", i, ".uid")))
			uidMatch := uid == v.GetInt(fmt.Sprint("push.list.", i, ".uid"))
			filterMatch := false
			if len(v.GetStringSlice(fmt.Sprint("push.list.", i, ".filter"))) == 0 {
				filterMatch = true
			} else {
				for j := 0; j < len(v.GetStringSlice(fmt.Sprint("push.list.", i, ".filter"))); j++ { //匹配推送过滤
					if dynamicType == v.GetString(fmt.Sprint("push.list.", i, ".filter.", j)) {
						filterMatch = true
					}
				}
			}
			if uidMatch && filterMatch {
				log.Info("[push] 处于推送列表: ", name, uid)
				at, userID, groupID := sendListGen(i)
				sendMsg(userID, groupID, at, formatDynamic(mainJson))
			}
		}
		log.Info("[push] 不处于推送列表: ", name, " ", uid)
	} else {
		log.Error("[push] 动态信息获取错误: ", mainJson.JSON("", ""))
	}
	return
}

// 初始化直播监听列表
func initLiveList() {
	liveListUID = []int{} //清空
	liveList = []int{}
	j := 0
	k := 0
	for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
		var uid int
		var roomID int
		if v.GetInt(fmt.Sprint("push.list.", i, ".live")) != 0 {
			uid = v.GetInt(fmt.Sprint("push.list.", i, ".uid"))
			roomID = v.GetInt(fmt.Sprint("push.list.", i, ".live"))
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
			log.Debug("[push] uid为 ", uid, " 的直播间 ", roomID, " 加入监听列表  目前状态: ", liveStateList[strconv.Itoa(roomID)].STATE)
			k += 1
		}
		j += 1
	}
	log.Info("[push] 动态推送 ", j, " 个")
	log.Info("[push] 直播间监听 ", k, " 个")
}

// 建立监听连接
func liveMonitor() {
	for {
		log.Debug("[push] 直播监听    disconnected: ", disconnected, "  configChanged: ", configChanged)
		if disconnected || configChanged {
			disconnected = false
			configChanged = false
			log.Info("[push] 开始建立监听连接")
			initLiveList()
			log.Trace("[push] len(liveList): ", len(liveList))
			for i := 0; i < len(liveList); i++ {
				log.Trace("[push] 建立监听连接    uid: ", liveListUID[i], "  roomID: ", liveList[i])
				go connectDanmu(liveListUID[i], liveList[i])
				time.Sleep(time.Second * 1)
			}
		}
		time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64("push.settings.resetCheckInterval")*1000)))
	}
}

// 判断直播间数据包类型，匹配推送
func liveChecker(pktJson gson.JSON, uid string, roomID string) {
	minimumInterval := int64(v.GetFloat64("push.settings.livePushMinimumInterval"))
	cmd := pktJson.Get("cmd").Str()
	switch cmd {
	case "LIVE":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprint("push.list.", i, ".live"))) {
				at, userID, groupID := sendListGen(i)
				if liveStateList[roomID].STATE == streamState.ONLINE {
					switch {
					case time.Now().Unix()-liveStateList[roomID].TIME < minimumInterval:
						log.Warn("[push] 屏蔽了一次间隔小于 ", minimumInterval, " 秒的开播推送")
						return
					}
				}
				go func(roomID string, time int64) {
					liveStateList[roomID] = liveState{ //记录开播
						STATE: streamState.ONLINE,
						TIME:  time}
				}(roomID, time.Now().Unix())
				log.Info("[push] 推送 ", uid, " 的直播间 ", roomID, " 开播")
				log.Info("[push] 记录开播时间: ", time.Unix(int64(liveStateList[roomID].TIME), 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Error("[push] 获取 ", uid, " 的直播间 ", roomID, " 信息失败")
					sendMsg(userID, groupID, at, fmt.Sprint("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomID, " 开播时无法获取直播间信息"))
					return
				}
				name := roomJson.Get("uname").Str()
				cover := roomJson.Get("cover_from_user").Str()
				title := roomJson.Get("title").Str()
				sendMsg(userID, groupID, at, fmt.Sprintf(
					`%s开播了！
[CQ:image,file=%s]
%s
live.bilibili.com/%s`,
					name,
					cover,
					title,
					roomID))
				return
			}
		}
	case "PREPARING":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprint("push.list.", i, ".live"))) {
				at, userID, groupID := sendListGen(i)
				if liveStateList[roomID].STATE == streamState.OFFLINE || liveStateList[roomID].STATE == streamState.ROTATE {
					switch {
					case time.Now().Unix()-liveStateList[roomID].TIME < minimumInterval:
						log.Warn("[push] 屏蔽了一次间隔小于 ", minimumInterval, " 秒的下播推送")
						return
					}
				}
				defer func(roomID string, time int64) {
					liveStateList[roomID] = liveState{ //记录下播
						STATE: streamState.OFFLINE,
						TIME:  time}
				}(roomID, time.Now().Unix())
				log.Info("[push] 推送 ", uid, " 的直播间", roomID, " 下播")
				log.Info("[push] 缓存的开播时间: ", time.Unix(int64(liveStateList[roomID].TIME), 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Error("[push] 获取 ", uid, " 的直播间 ", roomID, " 信息失败")
					sendMsg(userID, groupID, at, fmt.Sprint("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomID, " 下播时无法获取直播间信息"))
					return
				}
				name := roomJson.Get("uname").Str()
				cover := roomJson.Get("keyframe").Str()
				title := roomJson.Get("title").Str()
				duration := func() string {
					if liveStateList[roomID].TIME != 0 {
						return "本次直播持续了" + timeFormat(time.Now().Unix()-liveStateList[roomID].TIME)
					} else {
						return "未记录本次开播时间"
					}
				}()
				sendMsg(userID, groupID, at, fmt.Sprintf(
					`%s下播了~
[CQ:image,file=%s]
%s
%s`,
					name,
					cover,
					title,
					duration))
				return
			}
		}
	case "ROOM_CHANGE":
		if !v.GetBool("push.settings.roomChangeInfo") {
			return
		}
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == strconv.Itoa(v.GetInt(fmt.Sprint("push.list.", i, ".live"))) {
				at, userID, groupID := sendListGen(i)
				log.Info("[push] 推送 ", uid, " 的直播间 ", roomID, " 房间信息更新")
				roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
				if !ok {
					log.Error("[push] 获取直播间信息失败")
					sendMsg(userID, groupID, at, fmt.Sprint("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomID, " 房间信息更新时无法获取直播间信息"))
					return
				}
				area := fmt.Sprintf("%s - %s\n", //分区
					roomJson.Get("area_v2_parent_name").Str(),
					roomJson.Get("area_v2_name").Str())
				name := roomJson.Get("uname").Str()
				title := roomJson.Get("title").Str()
				sendMsg(userID, groupID, at, fmt.Sprintf(
					`%s更改了房间信息
房间名：%s
%s
live.bilibili.com/%s`,
					name,
					title,
					area,
					roomID))
				return
			}
		}
	}
}

// 生成发送队列
func sendListGen(i int) (string, []int, []int) {
	//读StringSlice再转成IntSlice实现同时支持输入单个和多个数据
	userID := []int{}
	userList := v.GetStringSlice(fmt.Sprint("push.list.", i, ".user"))
	if len(userList) != 0 {
		for _, each := range userList {
			user, err := strconv.Atoi(each)
			if err != nil {
				log.Error("[strconv.Atoi] ", err)
				log2SU.Error(fmt.Sprint("[strconv.Atoi] ", err))
			}
			userID = append(userID, user)
		}
	}
	log.Debug("[push] 推送用户: ", userID)
	groupID := []int{}
	groupList := v.GetStringSlice(fmt.Sprint("push.list.", i, ".group"))
	if len(groupList) != 0 {
		for _, each := range groupList {
			group, err := strconv.Atoi(each)
			if err != nil {
				log.Error("[strconv.Atoi] ", err)
				log2SU.Error(fmt.Sprint("[strconv.Atoi] ", err))
			}
			groupID = append(groupID, group)
		}
	}
	log.Debug("[push] 推送群组: ", groupID)
	at := ""
	atList := v.GetStringSlice(fmt.Sprint("push.list.", i, ".at"))
	if len(atList) != 0 {
		at += "\n"
		for _, each := range atList {
			at += "[CQ:at,qq=" + each + "]"
		}
	}
	log.Debug("[push] at: ", atList)
	return at, userID, groupID
}
