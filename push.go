package main

import (
	"fmt"
	"net/url"
	"strconv"

	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var cookie string

func initPush() {
	cookie = v.GetString("push.settings.cookie")
	log.Traceln("[push] cookie:\n", cookie)
	if cookie == "" || cookie == "<nil>" {
		log.Warningln("[push] 未配置cookie!")
	} else {
		go dynamicMonitor()
		//go liveMonitor()
	}
}

func liveMonitor() { // 不知道怎么做热更新 先摆了
	v.OnConfigChange(func(in fsnotify.Event) {
		go liveMonitor()
	})
}

func dynamicMonitor() {
	var (
		update_num      = "0" //更新数
		update_baseline = "0" //更新基线
		new_baseline    = "0" //新基线
	)
	update_baseline = getBaseline()
	if update_baseline == "-1" {
		log.Errorln("[push] 获取update_baseline时出现错误")
	}
	for {
		update_num = getUpdate(update_baseline)
		switch update_num {
		case "-1":
			log.Errorln("[push] 获取update_num时出现错误")
			log.Infof("update_num = %s\nupdate_baseline = %s\n", update_num, update_baseline)
			//sendErrToAdmin
			time.Sleep(time.Second * 30) //可能黑名单了吧
		case "0":
			log.Infof("[push] 没有新动态    update_num = %s    update_baseline = %s", update_num, update_baseline)
		default:
			new_baseline = getBaseline()
			log.Infof("[push] 有新动态！    update_num = %s    update_baseline = %s => %s", update_num, update_baseline, new_baseline)
			update_baseline = new_baseline
			go func(dynamicID string) { //异步检测推送
				rawJson := getDynamicJson(dynamicID)
				switch rawJson.Get("code").Int() {
				case 4101131: //动态已删除，不推送
					break
				case 404: //网络请求错误
					break
				default:
					mainJson := rawJson.Get("data.item")
					log.Debugln("[push] mainJson:\n", mainJson)
					dynamicChecker(mainJson)
				}
			}(new_baseline)
		}
		time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64("push.settings.dynamicUpdateInterval")*1000)))
	}
}

func getBaseline() string { //返回baseline用于监听更新
	url := "https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all?timezone_offset=-480&type=all&page=1&features=itemOpusStyle"
	body := httpsGet(url, cookie)
	if body == errJson404 {
		return "-1"
	}
	g := gson.NewFrom(body)
	return g.Get("data.update_baseline").Str()
}

func getUpdate(update_baseline string) string { //是否有新动态
	if update_baseline == "-1" {
		return "-1"
	}
	u, err := url.Parse("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/all/update?")
	if err != nil {
		log.Errorf("[push] getUpdate().url.Parse()发生错误: %v\n", err)
		return "-1"
	}
	values := url.Values{}
	values.Add("update_baseline", update_baseline)
	u.RawQuery = values.Encode()
	body := httpsGet(u.String(), cookie)
	if body == errJson404 {
		return "-1"
	}
	g := gson.NewFrom(body)
	return g.Get("data.update_num").Str()
}

func getDynamicJson(dynamicID string) gson.JSON { //获取动态信息
	u, err := url.Parse("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?")
	if err != nil {
		log.Errorf("[push] getDynamicJson().url.Parse()发生错误: %v\n", err)
		return gson.NewFrom(errJson404)
	}
	values := url.Values{}
	values.Add("id", dynamicID)
	u.RawQuery = values.Encode()
	body := httpsGet(u.String(), cookie)
	return gson.NewFrom(body)
}

func getVoteJson(voteID string) gson.JSON { //获取投票信息
	u, err := url.Parse("https://api.vc.bilibili.com/vote_svr/v1/vote_svr/vote_info?")
	if err != nil {
		log.Errorf("[push] getVoteJson().url.Parse()发生错误: %v\n", err)
		return gson.NewFrom(errJson404)
	}
	values := url.Values{}
	values.Add("vote_id", voteID)
	u.RawQuery = values.Encode()
	body := httpsGet(u.String(), cookie)
	return gson.NewFrom(body)
}

func dynamicChecker(mainJson gson.JSON) { //判断是否推送, mainJson = data.item
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
			log.Infof("[push] 处于推送列表: %d", uid)
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
			sendMsg(dynamicFormatter(mainJson), at, userID, groupID)
			return
		}
	}
	log.Infof("[push] 不处于推送列表: %d", uid)
	return
}

func dynamicFormatter(json gson.JSON) string { //动态格式化, 主动态"data.item", 转发原动态"data.item.orig"
	majorType := json.Get("modules.module_dynamic.major.type").Str()                       //暂时滞留
	dynamicType := json.Get("type").Str()                                                  //动态类型
	dynamic := json.Get("modules.module_dynamic")                                          //动态
	author := json.Get("modules.module_author")                                            //发布
	draw := json.Get("modules.module_dynamic.major.draw")                                  //图片
	archive := json.Get("modules.module_dynamic.major.archive")                            //视频
	article := json.Get("modules.module_dynamic.major.article")                            //文章
	live := gson.NewFrom(json.Get("modules.module_dynamic.major.live_rcmd.content").Str()) //直播
	additionalType := dynamic.Get("additional.type").Str()                                 //动态子项类型 投票/预约
	vote := dynamic.Get("additional.vote")                                                 //投票
	reserve := dynamic.Get("additional.reserve")                                           //预约
	id := json.Get("id_str").Str()
	name := author.Get("name").Str()
	head := "t.bilibili.com/" + id + "\n" + name + "：\n"
	appendVote := func(voteID string) string { //投票格式化
		voteJson := getVoteJson(voteID).Get("data.info")
		name := voteJson.Get("name").Str()   //发起者
		title := voteJson.Get("title").Str() //标题
		//desc := voteJson.Get("desc").Str()             //简介
		starttime := time.Unix(int64(voteJson.Get("starttime").Int()), 0).Format("2006-01-02 15:04:05") //开始时间
		endtime := time.Unix(int64(voteJson.Get("endtime").Int()), 0).Format("2006-01-02 15:04:05")     //结束时间
		choice_cnt := strconv.Itoa(voteJson.Get("choice_cnt").Int())                                    //最大多选数
		cnt := strconv.Itoa(voteJson.Get("cnt").Int())                                                  //参与数
		options_cnt := voteJson.Get("options_cnt").Int()                                                //选项数
		content := "\n" + name + "发起的投票：\n" + title + "\n" + starttime + "开始    " + endtime + "结束\n最多选" + choice_cnt + "项    " + cnt + "人参与\n"
		for i := 0; i < options_cnt; i++ { //选项序列
			content += fmt.Sprintf("%d. ", i+1) + voteJson.Get(fmt.Sprintf("options.%d.desc", i)).Str() + "  " + strconv.Itoa(voteJson.Get(fmt.Sprintf("options.%d.cnt", i)).Int()) + "人选择\n"
		}
		return content
	}
	log.Debugln("[push] dynamicType:", dynamicType)
	switch dynamicType {
	case "DYNAMIC_TYPE_FORWARD": //转发
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		first := ""
		after := text + "\n\n" + dynamicFormatter(json.Get("orig"))
		if !dynamic.Get("topic.name").Nil() {
			first += "#" + topic + "#\n"
		}
		return head + first + after
	case "DYNAMIC_TYPE_NONE": //转发的动态已删除
		return json.Get("modules.module_dynamic.major.none.tips").Str() //错误提示: "源动态已被作者删除"
	case "DYNAMIC_TYPE_WORD": //文本
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		first := ""
		after := text
		if !dynamic.Get("topic.name").Nil() {
			first += "#" + topic + "#\n"
		}
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			after += appendVote(strconv.Itoa(vote.Get("vote_id").Int()))
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := reserve.Get("title").Str()
			desc1 := reserve.Get("desc1.text").Str() //"预计xxx发布"
			desc2 := reserve.Get("desc2.text").Str() //"xx人预约"/"xx观看"
			after += "\n" + title + "\n" + desc1 + "    " + desc2
		}
		return head + first + after
	case "DYNAMIC_TYPE_DRAW": //图文
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		image := ""
		for i := 0; i < len(draw.Get("items").Arr()); i++ { //图片序列
			image += "[CQ:image,file=" + draw.Get(fmt.Sprintf("items.%d.src", i)).Str() + "]"
		}
		first := ""
		after := text + "\n" + image
		if !dynamic.Get("topic.name").Nil() {
			first += "#" + topic + "#\n"
		}
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			after += appendVote(strconv.Itoa(vote.Get("vote_id").Int()))
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := reserve.Get("title").Str()
			desc1 := reserve.Get("desc1.text").Str() //"预计xxx发布"
			desc2 := reserve.Get("desc2.text").Str() //"xx人预约"/"xx观看"
			after += "\n" + title + "\n" + desc1 + "    " + desc2
		}
		return head + first + after
	case "DYNAMIC_TYPE_AV": //视频
		action := author.Get("pub_action").Str()     //"投稿了视频"/"发布了动态视频"
		topic := dynamic.Get("topic.name").Str()     //话题
		text := dynamic.Get("desc.text").Str()       //文本
		cover := archive.Get("cover").Str()          //封面
		aid := archive.Get("aid").Str()              //av号数字
		title := archive.Get("title").Str()          //标题
		play := archive.Get("stat.play").Str()       //再生
		danmaku := archive.Get("stat.danmaku").Str() //弹幕
		bvid := archive.Get("bvid").Str()            //bv号
		//desc := archive.Get("desc").Str() //简介
		first := action + "\n\n"
		after := text + "\n[CQ:image,file=" + cover + "]\nav" + aid + "\n" + title + "\n" + play + "播放  " + danmaku + "弹幕" + "\nb23.tv/" + bvid
		if !dynamic.Get("topic.name").Nil() {
			first += "#" + topic + "#\n"
		}
		return head + first + after
	case "DYNAMIC_TYPE_ARTICLE": //文章
		action := author.Get("pub_action").Str()           //"投稿了文章"
		cover := ""                                        // 图片序列
		for _, each := range article.Get("covers").Arr() { //封面组
			cover += "[CQ:image,file=" + each.Str() + "]"
		}
		cid := strconv.Itoa(article.Get("id").Int()) //cv号数字
		title := article.Get("title").Str()          //标题
		label := article.Get("label").Str()          //xxx阅读
		desc := article.Get("desc").Str()            //简介
		return head + action + "\n\n" + cover + "\ncv" + cid + "\n" + title + "\n" + label + "\n简介: \n" + desc + "\nwww.bilibili.com/read/cv" + cid
	case "DYNAMIC_TYPE_LIVE_RCMD": //直播
		action := author.Get("pub_action").Str()                             //"直播了"
		cover := live.Get("live_play_info.cover").Str()                      //封面
		title := live.Get("live_play_info.title").Str()                      //房间名
		parent_area := live.Get("live_play_info.parent_area_name").Str()     //大分区
		area := live.Get("live_play_info.area_name").Str()                   //分区
		whatched := live.Get("live_play_info.watched_show.text_large").Str() //xxx人看过
		id := strconv.Itoa(live.Get("live_play_info.room_id").Int())         //房间号
		return head + action + "\n[CQ:image,file=" + cover + "]\n" + title + "\n" + parent_area + " - " + area + "\n" + whatched + "\nlive.bilibili.com/" + id
	}
	switch majorType { //主内容类型, 应该用不到
	case "<nil>": //文本 ↑
	case "MAJOR_TYPE_DRAW": //图文 ↑
	case "MAJOR_TYPE_ARCHIVE": //视频 ↑
	case "MAJOR_TYPE_ARTICLE": //文章 ↑
	case "MAJOR_TYPE_LIVE_RCMD": //直播 ↑
	case "MAJOR_TYPE_NONE": //转发的动态已删除 ↑
	}
	return "未知的动态类型"
}

func articleFormatter(json gson.JSON) string {
	return ""
}

func archiveFormatter(json gson.JSON) string {
	return ""
}
