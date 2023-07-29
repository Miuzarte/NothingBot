package main

import (
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

func getDynamicJson(dynamicID string) gson.JSON { //获取动态数据
	url := fmt.Sprintf("https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id=%s", dynamicID)
	body := httpsGet(url, cookie)
	log.Traceln("[bilibili] rawDynamicJson:", body)
	dynamicJson := gson.NewFrom(body)
	if dynamicJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 动态", dynamicID, "信息获取错误:", body)
	}
	return dynamicJson
}

func getVoteJson(voteID int) gson.JSON { //.Get("data.info")
	url := fmt.Sprintf("https://api.vc.bilibili.com/vote_svr/v1/vote_svr/vote_info?vote_id=%d", voteID)
	body := httpsGet(url, cookie)
	log.Traceln("[bilibili] rawVoteJson:", body)
	voteJson := gson.NewFrom(body)
	if voteJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 投票", voteID, "信息获取错误:", body)
		return gson.JSON{}
	}
	return voteJson
}

func formatDynamic(json gson.JSON) string { //主动态"data.item", 转发原动态"data.item.orig"
	var content string
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
	appendVote := func(voteID int) string { //投票格式化
		var content string
		voteJson := getVoteJson(voteID).Get("data.info")
		name := voteJson.Get("name").Str()   //发起者
		title := voteJson.Get("title").Str() //标题
		//desc := voteJson.Get("desc").Str()             //简介
		starttime := time.Unix(int64(voteJson.Get("starttime").Int()), 0).Format("2006-01-02 15:04:05") //开始时间
		endtime := time.Unix(int64(voteJson.Get("endtime").Int()), 0).Format("2006-01-02 15:04:05")     //结束时间
		choice_cnt := strconv.Itoa(voteJson.Get("choice_cnt").Int())                                    //最大多选数
		cnt := strconv.Itoa(voteJson.Get("cnt").Int())                                                  //参与数
		options_cnt := voteJson.Get("options_cnt").Int()                                                //选项数
		content += "\n\n" + name + "发起的投票：\n" + title + "\n" + starttime + "开始\n" + endtime + "结束\n最多选" + choice_cnt + "项    " + cnt + "人参与"
		for i := 0; i < options_cnt; i++ { //选项序列
			content += "\n" + fmt.Sprintf("%d. ", i+1) + voteJson.Get(fmt.Sprintf("options.%d.desc", i)).Str() + "  " + strconv.Itoa(voteJson.Get(fmt.Sprintf("options.%d.cnt", i)).Int()) + "人选择"
		}
		return content
	}
	log.Debugln("[bilibili] dynamicType:", dynamicType)
	switch dynamicType {
	case "DYNAMIC_TYPE_FORWARD": //转发
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		if !dynamic.Get("topic.name").Nil() {
			content += "#" + topic + "#\n"
		}
		content += text + "\n\n" + formatDynamic(json.Get("orig"))
		return head + content
	case "DYNAMIC_TYPE_NONE": //转发的动态已删除
		return json.Get("modules.module_dynamic.major.none.tips").Str() //错误提示: "源动态已被作者删除"
	case "DYNAMIC_TYPE_WORD": //文本
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		if !dynamic.Get("topic.name").Nil() {
			content += "#" + topic + "#\n"
		}
		content += text
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			content += appendVote(vote.Get("vote_id").Int())
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := reserve.Get("title").Str()
			desc1 := reserve.Get("desc1.text").Str() //"预计xxx发布"
			desc2 := reserve.Get("desc2.text").Str() //"xx人预约"/"xx观看"
			content += "\n" + title + "\n" + desc1 + "    " + desc2
		}
		return head + content
	case "DYNAMIC_TYPE_DRAW": //图文
		topic := dynamic.Get("topic.name").Str() //话题
		text := dynamic.Get("desc.text").Str()   //文本
		image := ""                              //图片
		for i := 0; i < len(draw.Get("items").Arr()); i++ {
			image += "[CQ:image,file=" + draw.Get(fmt.Sprintf("items.%d.src", i)).Str() + "]"
		}
		if !dynamic.Get("topic.name").Nil() {
			content += "#" + topic + "#\n"
		}
		content += text + "\n" + image
		if additionalType == "ADDITIONAL_TYPE_VOTE" {
			content += appendVote(vote.Get("vote_id").Int())
		}
		if additionalType == "ADDITIONAL_TYPE_RESERVE" {
			title := reserve.Get("title").Str()
			desc1 := reserve.Get("desc1.text").Str() //"预计xxx发布"
			desc2 := reserve.Get("desc2.text").Str() //"xx人预约"/"xx观看"
			content += "\n" + title + "\n" + desc1 + "    " + desc2
		}
		return head + content
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
		//desc := archive.Get("desc").Str()                      //简介
		content = action + "\n\n"
		if !dynamic.Get("topic.name").Nil() {
			content += "#" + topic + "#\n"
		}
		if !dynamic.Get("desc.text").Nil() {
			content += text + "\n"
		}
		content += "[CQ:image,file=" + cover + "]\nav" + aid + "\n" + title + "\n" + play + "播放  " + danmaku + "弹幕" + "\nwww.bilibili.com/video/" + bvid
		return head + content
	case "DYNAMIC_TYPE_ARTICLE": //文章
		action := author.Get("pub_action").Str()           //"投稿了文章"
		cover := ""                                        // 图片序列
		for _, each := range article.Get("covers").Arr() { //封面组
			cover += "[CQ:image,file=" + each.Str() + "]"
		}
		cvid := strconv.Itoa(article.Get("id").Int()) //cv号数字
		title := article.Get("title").Str()           //标题
		label := article.Get("label").Str()           //xxx阅读
		desc := article.Get("desc").Str()             //简介
		content += action + "\n\n" + cover + "\ncv" + cvid + "\n" + title + "\n" + label + "\n简介: \n" + desc + "\nwww.bilibili.com/read/cv" + cvid
		return head + content
	case "DYNAMIC_TYPE_LIVE_RCMD": //直播（动态流拿不到）
		action := author.Get("pub_action").Str()                             //"直播了"
		cover := live.Get("live_play_info.cover").Str()                      //封面
		title := live.Get("live_play_info.title").Str()                      //房间名
		parent_area := live.Get("live_play_info.parent_area_name").Str()     //大分区
		area := live.Get("live_play_info.area_name").Str()                   //小分区
		whatched := live.Get("live_play_info.watched_show.text_large").Str() //xxx人看过
		id := strconv.Itoa(live.Get("live_play_info.room_id").Int())         //房间号
		content += action + "\n[CQ:image,file=" + cover + "]\n" + title + "\n" + parent_area + " - " + area + "\n" + whatched + "\nlive.bilibili.com/" + id
		return head + content
	case "DYNAMIC_TYPE_COMMON_SQUARE": //应用装扮同步动态
		return head + "这是一条应用装扮同步动态"
	}
	log.Error("[bilibili] 得到了未知的动态类型")
	return head + "未知的动态类型"
}

func getArchiveJsonA(aid string) gson.JSON { //.Get("data"))
	url := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?aid=%s", aid)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawVideoJsonA", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", aid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return videoJson
}

func getArchiveJsonB(bvid string) gson.JSON { //.Get("data"))
	url := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawVideoJsonB", body)
	videoJson := gson.NewFrom(body)
	if videoJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 视频", bvid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return videoJson
}

func formatArchive(videoJson gson.JSON) string {
	var content string
	truncationLength := v.GetInt("parse.settings.descTruncationLength") //简介截断长度
	pic := videoJson.Get("pic").Str()                                   //封面
	aid := strconv.Itoa(videoJson.Get("aid").Int())                     //av号数字
	title := videoJson.Get("title").Str()                               //标题
	up := videoJson.Get("owner.name").Str()                             //up主
	desc := videoJson.Get("desc").Str()                                 //简介
	view := strconv.Itoa(videoJson.Get("stat.view").Int())              //再生
	danmaku := strconv.Itoa(videoJson.Get("stat.danmaku").Int())        //弹幕
	reply := strconv.Itoa(videoJson.Get("stat.reply").Int())            //回复
	like := strconv.Itoa(videoJson.Get("stat.like").Int())              //点赞
	coin := strconv.Itoa(videoJson.Get("stat.coin").Int())              //投币
	favorite := strconv.Itoa(videoJson.Get("stat.favorite").Int())      //收藏
	bvid := videoJson.Get("bvid").Str()                                 //bv号
	content += "[CQ:image,file=" + pic + "]\nav" + aid + "\n" + title + "\nUP：" + up + "\n"
	if (desc != "<nil>" && desc != "-") && truncationLength > 0 {
		if len([]rune(desc)) > truncationLength {
			content += "简介：" + string([]rune(desc)[0:truncationLength]) + "......\n"
		} else {
			content += "简介：" + string([]rune(desc)) + "\n"
		}
	}
	content += view + "播放  " + danmaku + "弹幕  " + reply + "回复\n" + like + "点赞  " + coin + "投币  " + favorite + "收藏\nwww.bilibili.com/video/" + bvid
	return content
}

func getArticleJson(cvid string) gson.JSON { //.Get("data")
	url := fmt.Sprintf("https://api.bilibili.com/x/article/viewinfo?id=%s", cvid)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawArticleJson:", body)
	articleJson := gson.NewFrom(body)
	if articleJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 文章", cvid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return articleJson
}

func formatArticle(articleJson gson.JSON, cvid string) string { //文章信息拿不到自己的cv号
	var content string
	image := "" //头图
	for i := 0; i < len(articleJson.Get("image_urls").Arr()); i++ {
		image += "[CQ:image,file=" + articleJson.Get(fmt.Sprintf("image_urls.%d", i)).Str() + "]"
	}
	title := articleJson.Get("title").Str()             //标题
	author := articleJson.Get("author_name").Str()      //作者
	view := articleJson.Get("stats.view").Str()         //阅读
	reply := articleJson.Get("stats.reply").Str()       //回复
	share := articleJson.Get("stats.share").Str()       //分享
	like := articleJson.Get("stats.like").Str()         //点赞
	coin := articleJson.Get("stats.coin").Str()         //投币
	favorite := articleJson.Get("stats.favorite").Str() //收藏
	content += image + "\ncv" + cvid + "\n" + title + "\n作者：" + author + "\n" + view + "阅读  " + reply + "回复  " + share + "分享\n" + like + "点赞  " + coin + "投币  " + favorite + "收藏\nwww.bilibili.com/read/cv" + cvid
	return content
}

func getSpaceJson(uid string) gson.JSON { //.Get("data.card")
	url := fmt.Sprintf("https://api.bilibili.com/x/web-interface/card?mid=%s", uid)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawSpaceJson:", body)
	spaceJson := gson.NewFrom(body)
	if spaceJson.Get("code").Int() != 0 {
		log.Errorln("[parse] 空间", uid, "信息获取错误:", body)
		return gson.JSON{}
	}
	return spaceJson
}

func formatSpace(spaceJson gson.JSON) string {
	var content string
	face := spaceJson.Get("face").Str()                                    //头像
	name := spaceJson.Get("name").Str()                                    //用户名
	level := strconv.Itoa(spaceJson.Get("level_info.current_level").Int()) //账号等级
	pendant_name := spaceJson.Get("pendant.name").Str()                    //头像框所属装扮
	pendant_pid := strconv.Itoa(spaceJson.Get("pendant.pid").Int())        //装扮专属编号
	sign := spaceJson.Get("sign").Str()                                    //签名
	attention := strconv.Itoa(spaceJson.Get("attention").Int())            //关注
	fans := strconv.Itoa(spaceJson.Get("fans").Int())                      //粉丝
	mid := spaceJson.Get("mid").Str()                                      //uid
	content += "[CQ:image,file=" + face + "]\n" + name + "（LV" + level + "）\n"
	if pendant_name != "" && pendant_pid != "0" {
		content += "头像框：" + pendant_name + "（" + pendant_pid + "）\n"
	}
	if sign != "" {
		content += sign + "\n"
	}
	content += attention + "关注  " + fans + "粉丝\nspace.bilibili.com/" + mid
	return content
}

func getRoomJsonUID(uid int) gson.JSON { //uid获取直播间数据  .Gets("data", strconv.Itoa(uid))
	url := fmt.Sprintf("https://api.live.bilibili.com/room/v1/Room/get_status_info_by_uids?uids[]=%d", uid)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	return liveJson
}

func getRoomJsonRoomID(roomID int) gson.JSON { //房间号获取直播间数据（拿不到UP用户名）  .Get("data")
	url := fmt.Sprintf("https://api.live.bilibili.com/room/v1/Room/get_info?room_id=%d", roomID)
	body := httpsGet(url, "")
	log.Traceln("[bilibili] rawRoomJson:", body)
	liveJson := gson.NewFrom(body)
	return liveJson
}

func formatLive(roomJson gson.JSON) string {
	var content string
	cover := roomJson.Get("cover_from_user").Str()                //封面
	keyframe := roomJson.Get("keyframe").Str()                    //关键帧
	uname := roomJson.Get("uname").Str()                          //主播
	live_status := roomJson.Get("live_status").Int()              //房间状态:   0: "未开播"  1: "直播中 " 2: "轮播中"
	title := roomJson.Get("title").Str()                          //房间名
	area_parent_name := roomJson.Get("area_v2_parent_name").Str() //大分区
	area_name := roomJson.Get("area_v2_name").Str()               //小分区
	room_id := strconv.Itoa(roomJson.Get("room_id").Int())        //房间号
	content += "[CQ:image,file=" + cover + "][CQ:image,file=" + keyframe + "]\n" + uname + "的直播间"
	switch live_status {
	case 0:
		content += "（未开播）\n"
	case 1:
		content += "（直播中）\n"
	case 2:
		content += "（轮播中）\n"
	}
	content += title + "\n" + area_parent_name + " - " + area_name + "\nlive.bilibili.com/" + room_id
	return content
}
