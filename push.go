package main

import (
	"fmt"
	"strconv"

	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var cookie string
var liveListUID []int
var liveList []int
var liveState = make(map[int]int)
var disconnected bool
var configChanged bool

func initPush() { //初始化推送
	cookie = v.GetString("push.settings.cookie")
	log.Traceln("[push] cookie:\n", cookie)
	if cookie == "" || cookie == "<nil>" {
		log.Warningln("[push] 未配置cookie!")
	} else {
		go dynamicMonitor()
	}
	disconnected = true
	go liveMonitor()
	v.OnConfigChange(func(in fsnotify.Event) { //获取监听列表
		configChanged = true
	})
}

func getBaseline() string { //返回baseline用于监听更新
	url := "https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all?timezone_offset=-480&type=all&page=1&features=itemOpusStyle"
	body := httpsGet(url, cookie)
	if body == errJson404 {
		return "-1"
	}
	update_baseline := gson.NewFrom(body).Get("data.update_baseline").Str()
	if update_baseline == "<nil>" {
		log.Error("[push] 未知数据:", body)
		return "-1"
	}
	return update_baseline
}

func getUpdate(update_baseline string) string { //是否有新动态
	url := fmt.Sprintf("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all/update?update_baseline=%s", update_baseline)
	body := httpsGet(url, cookie)
	if body == errJson404 {
		return "-1"
	}
	update_num := gson.NewFrom(body).Get("data.update_num").Str()
	if update_num == "<nil>" {
		log.Error("[push] 未知数据:", body)
		return "-1"
	}
	return update_num
}

func cookieChecker() bool { //检测cookie有效性
	url := "https://passport.bilibili.com/x/passport-login/web/cookie/info"
	body := httpsGet(url, cookie)
	bodyJson := gson.NewFrom(body)
	switch bodyJson.Get("code").Int() {
	case 0:
		log.Warningln("[push] cookie未过期但触发了有效性检测")
		sendMsg2Admin("[push] cookie未过期但触发了有效性检测")
		return true
	case -101:
		log.Errorln("[push] cookie已过期")
		sendMsg2Admin("[push] cookie已过期")
		return false
	default:
		log.Errorln("[push] 非正常cookie状态:", body)
		sendMsg2Admin("[push] 非正常cookie状态: " + body)
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
				log.Errorln("[push] 停止拉取动态更新")
				<-block
			}
			failureCount++
			if failureCount >= 10 {
				println("[push] 尝试更新失败", failureCount, "次, 停止拉取动态更新")
				sendMsg2Admin("[push] 连续更新失败十次但cookie未失效, 已停止拉取动态更新")
				<-block
			}
			duration := time.Duration(failureCount * 30)
			println("[push] 获取更新失败", failureCount, "次, 将在", duration, "秒后重试")
			time.Sleep(time.Second * duration)
		case "0":
			log.Debugf("[push] 没有新动态    update_num = %s    update_baseline = %s", update_num, update_baseline)
		default:
			new_baseline = getBaseline()
			log.Infof("[push] 有新动态！    update_num = %s    update_baseline = %s => %s", update_num, update_baseline, new_baseline)
			update_baseline = new_baseline
			go func(dynamicID string) { //异步检测推送
				rawJson := getDynamicJson(dynamicID)
				switch rawJson.Get("code").Int() {
				case 4101131: //动态已删除，不推送
					sendMsg(fmt.Sprintf("[Info] [push] 记录到一条已删除的动态，dynamicID = %s", dynamicID), "", adminID, []int{})
					break
				case 404: //网络请求错误
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

func dynamicChecker(mainJson gson.JSON) { //mainJson = data.item
	uid := mainJson.Get("modules.module_author.mid").Int()
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
			log.Debugln("[push] up uid:", uid)
			log.Infoln("[push] 处于推送列表:", uid)
			at, userID, groupID := sendListGen(i)
			sendMsg(formatDynamic(mainJson), at, userID, groupID)
			return
		}
	}
	log.Infoln("[push] 不处于推送列表:", uid)
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
			log.Debugln("[push] uid为", uid, "的直播间", roomID, "加入监听列表")
			liveListUID = append(liveListUID, uid)
			liveList = append(liveList, roomID)
			k += 1
		}
		j += 1
	}
	log.Infoln("[push] 动态推送", j, "个")
	log.Infoln("[push] 直播间监听", k, "个")
}

func liveMonitor() { //建立监听连接
	for {
		log.Debugln("[push] 检测直播监听重连    disconnected:", disconnected, "   configChanged:", configChanged)
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

func liveChecker(pktJson gson.JSON) { //判断数据包类型
	cmd := pktJson.Get("cmd").Str() //"LIVE"/"PREPARING"
	switch cmd {
	case "LIVE":
		roomID := pktJson.Get("roomid").Int()
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == v.GetInt(fmt.Sprintf("push.list.%d.live", i)) {
				if (int(time.Now().Unix()) - liveState[roomID]) < 60 { //防止重复推送开播
					log.Warningln("[push] 屏蔽了一次间隔小于60秒的开播推送")
					return
				}
				liveState[roomID] = int(time.Now().Unix()) //记录开播时间
				uid := v.GetInt(fmt.Sprintf("push.list.%d.uid", i))
				log.Infoln("[push] 推送", uid, "的直播间", roomID, "开播")
				log.Infoln("[push] 记录开播时间:", time.Unix(int64(liveState[roomID]), 0).Format("2006-01-02 15:04:05"))
				roomJson, _ := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
				name := roomJson.Get("uname").Str()
				cover := roomJson.Get("cover_from_user").Str()
				title := roomJson.Get("title").Str()
				msg := name + "开播了！\n[CQ:image,file=" + cover + "]\n" + title + "\nlive.bilibili.com/" + strconv.Itoa(roomID)
				userID, groupID, at := sendListGen(i)
				sendMsg(msg, userID, groupID, at)
				return
			}
		}
	case "PREPARING":
		roomID, _ := strconv.Atoi(pktJson.Get("roomid").Str())
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomID == v.GetInt(fmt.Sprintf("push.list.%d.live", i)) {
				uid := v.GetInt(fmt.Sprintf("push.list.%d.uid", i))
				log.Infoln("[push] 推送", uid, "的直播间", roomID, "下播")
				log.Infoln("[push] 缓存的开播时间:", time.Unix(int64(liveState[roomID]), 0).Format("2006-01-02 15:04:05"))
				roomJson, _ := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
				name := roomJson.Get("uname").Str()
				cover := roomJson.Get("keyframe").Str()
				title := roomJson.Get("title").Str()
				msg := name + "下播了~\n[CQ:image,file=" + cover + "]\n" + title
				if liveState[roomID] != 0 {
					msg += "\n本次直播持续了" + timeFormatter(int(time.Now().Unix())-liveState[roomID])
					delete(liveState, roomID)
				} else {
					msg += "\n未记录本次开播时间"
				}
				userID, groupID, at := sendListGen(i)
				sendMsg(msg, userID, groupID, at)
				return
			}
		}
	}
}

func sendListGen(i int) (string, []int, []int) { //生成发送队列
	userID := []int{} // 用户列表
	userList := v.GetStringSlice(fmt.Sprintf("push.list.%d.user", i))
	if len(userList) != 0 {
		for _, each := range userList { //[]string to []int
			user, _ := strconv.Atoi(each)
			userID = append(userID, user)
		}
	}
	log.Debugln("[push] 推送用户:", userID)
	groupID := []int{} // 群组列表
	groupList := v.GetStringSlice(fmt.Sprintf("push.list.%d.group", i))
	if len(groupList) != 0 {
		for _, each := range groupList { //[]string to []int
			group, _ := strconv.Atoi(each)
			groupID = append(groupID, group)
		}
	}
	log.Debugln("[push] 推送群组:", groupID)
	at := "" // at序列
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
