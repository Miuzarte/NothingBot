package main

import (
	"fmt"
	"regexp"
	"strconv"

	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

// 推送消息
func (p push) send(msg ...any) {
	go func() {
		err := bot.SendPrivateMsgs(p.userID, fmt.Sprint(msg...))
		if err != nil {
			log.Error("[Push] err: ", err)
		}
	}()
	go func() {
		err := bot.SendGroupMsgs(p.groupID, fmt.Sprint(msg...))
		if err != nil {
			log.Error("[Push] err: ", err)
		}
	}()
}

// 生成推送对象
func genPush(i int) (p push) {
	//读StringSlice再转成IntSlice实现同时支持输入单个和多个数据
	userList := v.GetStringSlice(fmt.Sprint("push.list.", i, ".user"))
	if len(userList) > 0 {
		for _, each := range userList {
			user, err := strconv.Atoi(each)
			if err != nil {
				log.Error("[strconv.Atoi] ", err)
				bot.Log2SU.Error(fmt.Sprint("[strconv.Atoi] ", err))
			}
			p.userID = append(p.userID, user)
		}
	}
	log.Debug("[push] 推送用户: ", p.userID)
	groupList := v.GetStringSlice(fmt.Sprint("push.list.", i, ".group"))
	if len(groupList) > 0 {
		for _, each := range groupList {
			group, err := strconv.Atoi(each)
			if err != nil {
				log.Error("[strconv.Atoi] ", err)
				bot.Log2SU.Error(fmt.Sprint("[strconv.Atoi] ", err))
			}
			p.groupID = append(p.groupID, group)
		}
	}
	log.Debug("[push] 推送群组: ", p.groupID)
	return
}

func extractUid(c string) (uid int) {
	//DedeUserID=59442895;
	match := regexp.MustCompile(`DedeUserID=([0-9]+);`).FindAllStringSubmatch(c, -1)
	if len(match) > 0 {
		if uidS := match[0][1]; len(uidS) != 0 {
			uid, _ = strconv.Atoi(uidS)
		}
	}
	return
}

func extractBuvid(c string) (bvuid string) {
	// _uuid=91F87C44-8B65-64C4-296C-B102F459941CF05635infoc;
	match := regexp.MustCompile(`_uuid=(.+?);`).FindAllStringSubmatch(c, -1)
	if len(match) > 0 {
		if b := match[0][1]; len(b) != 0 {
			bvuid = b
		}
	}
	return
}

// // 管理员私聊更新cookie
// func checkCookieUpdate(ctx *EasyBot.CQMessage) {
// 	if !ctx.IsPrivateSU() {
// 		return
// 	}
// 	match := ctx.RegexpFindAllStringSubmatch(`(查看|check|view)\s*(饼干|cookie)`)
// 	if len(match) > 0 {
// 		ctx.SendMsg(biliIdentity.Cookie, "\n\n", biliIdentity.RefreshToken)
// 	}
// 	match = ctx.RegexpFindAllStringSubmatch(`(设置|set)\s*(饼干|cookie)\s*(.*)`)
// 	if len(match) > 0 {
// 		if c := match[0][3]; len(c) != 0 {
// 			cookie = c
// 			ctx.SendMsg("[bilibili] 设置cookie成功")
// 		} else {
// 			ctx.SendMsg("[bilibili] 设置cookie失败\nmatch: ", match)
// 		}
// 		if cookieUid = extractUid(cookie); cookieUid != 0 {
// 			ctx.SendMsg("[bilibili] uid获取成功\n", cookieUid)
// 		} else {
// 			ctx.SendMsg("[bilibili] uid获取失败")
// 		}
// 		if cookieBuvid = extractBuvid(cookie); cookieBuvid != "" {
// 			ctx.SendMsg("[bilibili] buvid获取成功\n", cookieBuvid)
// 		} else {
// 			ctx.SendMsg("[bilibili] buvid获取失败")
// 		}
// 		if !cookieValidity {
// 			pushWait.Done()
// 		}
// 	}
// }

// 初始化推送
func initPush() {
	dynamicCheckDuration = time.Millisecond * time.Duration(v.GetFloat64("push.settings.dynamicUpdateInterval")*1000)
	if len(biliIdentity.Cookie) != 0 {
		log.Trace("[bilibili] cookie:\n", biliIdentity.Cookie)
		if cookieValidity = validateCookie(biliIdentity.Cookie); cookieValidity {
			if cookieUid = extractUid(biliIdentity.Cookie); cookieUid != 0 {
				log.Info("[bilibili] cookie所属uid: ", cookieUid)
			} else {
				log.Warn("[bilibili] uid识别失败! 请确保cookie完整")
			}
			go initLive()
			// if cookieBuvid = extractBuvid(biliIdentity.Cookie); cookieBuvid != "" {
			// 	log.Info("[bilibili] cookie buvid: ", cookieBuvid)
			// 	if configUpdateCount == 0 {
			// 		go initLive() //弹幕监听需要buvid
			// 	}
			// } else {
			// 	log.Warn("[bilibili] buvid识别失败! 将不监听直播推送")
			// }
			if configUpdateCount == 0 {
				go dynamicMonitor()
			}
		} else {
			log.Warn("[bilibili] cookie无效! 不进行推送监听")
		}
	} else {
		log.Warn("[bilibili] 未配置cookie! 不进行推送监听")
	}
}

// 初始化直播监听并建立连接
func initLive() {
	newLiveList := make(map[int]liveInfo)
	j := 0
	k := 0
	nowTime := time.Now().Unix()
	for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
		j++
		if v.GetInt(fmt.Sprint("push.list.", i, ".live")) != 0 {
			k++
			uid := v.GetInt(fmt.Sprint("push.list.", i, ".uid"))
			roomid := v.GetInt(fmt.Sprint("push.list.", i, ".live"))
			live := NewDanmaku(uid, roomid).OnDanmakuRecv(func(recv gson.JSON, uid, roomid int) {
				go checkLive(recv, uid, roomid)
			})
			g, _ := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
			state := g.Get("live_status").Int()
			var timeS int64
			if g.Get("live_time").Int() != 0 { //开播状态可以获取开播时间
				timeS = int64(g.Get("live_time").Int())
				log.Debug("[push] 直播间 ", roomid, "(uid: ", uid, ") 此次开播时间: ", time.Unix(timeS, 0).Format(timeLayout.S24C))
			} else {
				timeS = nowTime
				log.Debug("[push] 直播间 ", roomid, "(uid: ", uid, ") 未开播")
			}
			newLiveList[roomid] = liveInfo{
				live:   live,
				uid:    uid,
				roomid: roomid,
				state:  state,
				time:   timeS,
			}
		}
	}
	log.Info("[push] 动态推送 ", j, " 个")
	log.Info("[push] 直播间监听 ", k, " 个")
	//热更新处理
	if len(liveList) == 0 { //初始状态
		liveList = newLiveList
		for _, l := range liveList {
			log.Info("[push] 直播间 ", l.roomid, "(uid: ", l.uid, ") 正在建立监听连接  目前状态: ", l.state)
			l.live.Start()
			time.Sleep(time.Second)
		}
	} else { //找出新增, 减少的直播间
		added := make(map[int]liveInfo)
		removed := make(map[int]liveInfo)
		for key, l := range liveList { //找出减少的键值对
			if _, ok := newLiveList[key]; !ok {
				removed[key] = l
			}
		}
		for key, l := range newLiveList { //找出新增的键值对
			if _, ok := liveList[key]; !ok {
				added[key] = l
			}
		}
		for _, l := range removed {
			log.Info("[push] 移除的直播间 ", l.roomid, "(uid: ", l.uid, ") 已断开监听连接  目前状态: ", l.state)
			l.live.Stop()
			time.Sleep(time.Second)
		}
		for _, l := range added {
			log.Info("[push] 新增的直播间 ", l.roomid, "(uid: ", l.uid, ") 正在建立监听连接  目前状态: ", l.state)
			l.live.Start()
			time.Sleep(time.Second)
		}
		liveList = newLiveList
	}
}

// 初始化baseline用于监听更新
func getBaseline() (update_baseline string) {
	g, err := ihttp.New().WithUrl("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all").
		WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
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
		WithAddQuery("update_baseline", update_baseline).WithHeaders(iheaders).WithCookie(biliIdentity.Cookie).
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
		update_num = getUpdate(update_baseline)
		switch update_num {
		case "-1":
			log.Error("[push] 获取update_num时出现错误    update_num = ", update_num, "  update_baseline = ", update_baseline)
			if cookieValidity = validateCookie(biliIdentity.Cookie); !cookieValidity {
				pushWait.Add(1)
				pushWait.Wait()
				failureCount = 0
				continue
			}
			failureCount++
			if failureCount >= 10 {
				log.Error("[push] 尝试更新失败 ", failureCount, " 次, 暂停拉取动态更新")
				bot.Log2SU.Error(fmt.Sprint("[push] 连续更新失败 ", failureCount, " 次，暂停拉取动态更新"))
				pushWait.Add(1)
				pushWait.Wait()
				failureCount = 0
				continue
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
					stateCode := rawJson.Get("code").Int()
					mainJson := rawJson.Get("data.item")
					log.Debug("[push] mainJson: ", mainJson.JSON("", ""))
					switch stateCode {
					case 4101131: //动态已删除，不推送
						if dynamicHistrory[dynamicID] != "" {
							log.Info("[push] 明确记录到一条来自 ", dynamicHistrory[dynamicID], " 的已删除动态 ", dynamicID)
							bot.Log2SU.Info(fmt.Sprint("[push] 明确记录到一条来自 ", dynamicHistrory[dynamicID], " 的已删除动态 ", dynamicID))
						}
					case 500: //加载错误，请稍后再试
						if dynamicHistrory[dynamicID] == "" { //检测是否为重复动态
							go func(dynamicID string) {
								for i := 0; i < 3; i++ { //重试三次
									log.Warn("[push] (RETRY_", i+1, ") 将在 ", dynamicCheckDuration*3, " 后重试动态 ", dynamicID)
									time.Sleep(dynamicCheckDuration * 3)
									rawJson := getDynamicJson(dynamicID)
									stateCode := rawJson.Get("code").Int()
									mainJson := rawJson.Get("data.item")
									log.Debug("[push] (RETRY) mainJson: ", mainJson.JSON("", ""))
									if stateCode == 0 {
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
						log.Warn("[push] other code: ", stateCode)
					}
				}(new_baseline)
			}
		}
		time.Sleep(dynamicCheckDuration)
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
				genPush(i).send(formatDynamic(mainJson))
				return
			}
		}
		log.Info("[push] 不处于推送列表: ", name, " ", uid)
	} else {
		log.Error("[push] 动态信息获取错误: ", mainJson.JSON("", ""))
	}
}

// 判断直播间数据包类型，匹配推送
func checkLive(pktJson gson.JSON, uid int, roomid int) {
	minimumInterval := int64(v.GetFloat64("push.settings.livePushMinimumInterval"))
	cmd := pktJson.Get("cmd").Str()
	switch cmd {
	case "LIVE":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomid == v.GetInt(fmt.Sprint("push.list.", i, ".live")) {
				go func(time int64) { //记录开播, 强迫症, 跟下面下播的defer对应
					liveInfo := liveList[roomid]
					liveInfo.state = liveState.ONLINE
					liveInfo.time = time
					liveList[roomid] = liveInfo
				}(time.Now().Unix())
				if liveList[roomid].state == liveState.ONLINE {
					switch {
					case time.Now().Unix()-liveList[roomid].time < minimumInterval:
						log.Warn("[push] 屏蔽了一次间隔小于 ", minimumInterval, " 秒的开播推送")
						return
					}
				}
				push := genPush(i)
				log.Info("[push] 推送 ", uid, " 的直播间 ", roomid, " 开播")
				log.Info("[push] 记录开播时间: ", time.Unix(liveList[roomid].time, 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
				if ok {
					name := roomJson.Get("uname").Str()
					cover := roomJson.Get("cover_from_user").Str()
					title := roomJson.Get("title").Str()
					push.send(fmt.Sprintf(
						`%s开播了！
[CQ:image,file=%s]
%s
live.bilibili.com/%d`,
						name,
						cover,
						title,
						roomid))
				} else {
					log.Error("[push] 获取 ", uid, " 的直播间 ", roomid, " 信息失败")
					push.send("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomid, " 开播时无法获取直播间信息")
				}
				return
			}
		}
	case "PREPARING":
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomid == v.GetInt(fmt.Sprint("push.list.", i, ".live")) {
				defer func(time int64) { //记录下播
					liveInfo := liveList[roomid]
					liveInfo.state = liveState.OFFLINE
					liveInfo.time = time
					liveList[roomid] = liveInfo
				}(time.Now().Unix())
				if liveList[roomid].state == liveState.OFFLINE || liveList[roomid].state == liveState.ROTATE {
					switch {
					case time.Now().Unix()-liveList[roomid].time < minimumInterval:
						log.Warn("[push] 屏蔽了一次间隔小于 ", minimumInterval, " 秒的下播推送")
						return
					}
				}
				push := genPush(i)
				log.Info("[push] 推送 ", uid, " 的直播间", roomid, " 下播")
				log.Info("[push] 缓存的开播时间: ", time.Unix(int64(liveList[roomid].time), 0).Format(timeLayout.L24))
				roomJson, ok := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
				if ok {
					name := roomJson.Get("uname").Str()
					cover := roomJson.Get("keyframe").Str()
					title := roomJson.Get("title").Str()
					duration := func() string {
						if liveList[roomid].time != 0 {
							return "本次直播持续了" + formatTime(time.Now().Unix()-liveList[roomid].time)
						} else {
							return "未记录本次开播时间"
						}
					}()
					push.send(fmt.Sprintf(
						`%s下播了～
[CQ:image,file=%s]
%s
%s`,
						name,
						cover,
						title,
						duration))
				} else {
					log.Error("[push] 获取 ", uid, " 的直播间 ", roomid, " 信息失败")
					push.send("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomid, " 下播时无法获取直播间信息")
				}
				return
			}
		}
	case "ROOM_CHANGE":
		if !v.GetBool("push.settings.roomChangeInfo") {
			return
		}
		for i := 0; i < len(v.GetStringSlice("push.list")); i++ {
			if roomid == v.GetInt(fmt.Sprint("push.list.", i, ".live")) {
				push := genPush(i)
				log.Info("[push] 推送 ", uid, " 的直播间 ", roomid, " 房间信息更新")
				roomJson, ok := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
				if ok {
					area := fmt.Sprintf("%s - %s\n", //分区
						roomJson.Get("area_v2_parent_name").Str(),
						roomJson.Get("area_v2_name").Str())
					name := roomJson.Get("uname").Str()
					title := roomJson.Get("title").Str()
					push.send(fmt.Sprintf(
						`%s更改了房间信息
房间名：%s
%s
live.bilibili.com/%d`,
						name,
						title,
						area,
						roomid))
				} else {
					log.Error("[push] 获取直播间信息失败")
					push.send("[NothingBot] [ERROR] [push] 推送 ", uid, " 的直播间 ", roomid, " 房间信息更新时无法获取直播间信息")
				}
				return
			}
		}
	}
}
