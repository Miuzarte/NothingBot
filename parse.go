package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

var biliLinkRegexp = struct {
	DYNAMIC  string
	ARCHIVEa string
	ARCHIVEb string
	ARTICLE  string
	SPACE    string
	LIVE     string
	SHORT    string
}{
	DYNAMIC:  `(t.bilibili.com|dynamic|opus)\\?/([0-9]{18,19})`,                            //应该不会有17位的，可能要有19位
	ARCHIVEa: `video\\?/av([0-9]{1,10})`,                                                   //9位 预留10
	ARCHIVEb: `video\\?/(BV[1-9A-HJ-NP-Za-km-z]{10})`,                                      //恒定BV + 10位base58
	ARTICLE:  `(read\\?/cv|read\\?/mobile\\?/)([0-9]{1,9})`,                                //8位 预留9
	SPACE:    `space\.bilibili\.com\\?/([0-9]{1,16})`,                                      //新uid 16位
	LIVE:     `live\.bilibili\.com\\?/([0-9]{1,9})`,                                        //8位 预留9
	SHORT:    `(b23|acg)\.tv\\?/(BV[1-9A-HJ-NP-Za-km-z]{10}|av[0-9]{1,10}|[0-9A-Za-z]{7})`, //暂时应该只有7位  也有可能是av/bv号
}

var parseHistoryList = make(map[string]parseHistory) //av/bv : group/user, time

type parseHistory struct {
	WHERE int
	TIME  int
}

//base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz

func extractor(str string) (id string, kind string) {
	dynamicID := regexp.MustCompile(biliLinkRegexp.DYNAMIC).FindAllStringSubmatch(str, -1)
	aid := regexp.MustCompile(biliLinkRegexp.ARCHIVEa).FindAllStringSubmatch(str, -1)
	bvid := regexp.MustCompile(biliLinkRegexp.ARCHIVEb).FindAllStringSubmatch(str, -1)
	cvid := regexp.MustCompile(biliLinkRegexp.ARTICLE).FindAllStringSubmatch(str, -1)
	uid := regexp.MustCompile(biliLinkRegexp.SPACE).FindAllStringSubmatch(str, -1)
	roomID := regexp.MustCompile(biliLinkRegexp.LIVE).FindAllStringSubmatch(str, -1)
	log.Trace("[parse] dynamicID: ", dynamicID)
	log.Trace("[parse] aid: ", aid)
	log.Trace("[parse] bvid: ", bvid)
	log.Trace("[parse] cvid: ", cvid)
	log.Trace("[parse] uid: ", uid)
	log.Trace("[parse] roomID: ", roomID)
	switch {
	case len(dynamicID) > 0:
		log.Debug("[parse] 识别到一个动态, dynamicID[0][2]: ", dynamicID[0][2])
		return dynamicID[0][2], "DYNAMIC"
	case len(aid) > 0:
		log.Debug("[parse] 识别到一个视频(a), aid[0][1]: ", aid[0][1])
		return aid[0][1], "ARCHIVEa"
	case len(bvid) > 0:
		log.Debug("[parse] 识别到一个视频(b), bvid[0][1]: ", bvid[0][1])
		return bvid[0][1], "ARCHIVEb"
	case len(cvid) > 0:
		log.Debug("[parse] 识别到一个专栏, cvid[0][2]: ", cvid[0][2])
		return cvid[0][2], "ARTICLE"
	case len(uid) > 0:
		log.Debug("[parse] 识别到一个用户空间, uid[0][1]: ", uid[0][1])
		return uid[0][1], "SPACE"
	case len(roomID) > 0:
		log.Debug("[parse] 识别到一个直播, roomID[0][1]: ", roomID[0][1])
		return roomID[0][1], "LIVE"
	default:
		return str, ""
	}
}

func deShortLink(slug string) string { //短链解析
	url := "https://b23.tv/" + slug
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, _ := client.Head(url)
	var location string
	var statusCode string
	if len(resp.Header["Location"]) > 0 {
		location = resp.Header["Location"][0]
		log.Debug("[parse] 短链解析结果: ", location[0:32])
	}
	if len(resp.Header["Bili-Status-Code"]) > 0 {
		statusCode = resp.Header["Bili-Status-Code"][0]
	}
	switch statusCode {
	case "-404":
		log.Warn("[parse] 短链解析失败: ", statusCode, "    location: ", location)
		return ""
	}
	return location
}

func normalParse(id string, kind string, msg gocqMessage) string { //拿到id直接解析
	if kind == "" {
		return ""
	}
	duration := int64(v.GetFloat64("parse.settings.sameParseInterval"))
	where := 0
	switch msg.message_type {
	case "group":
		where = msg.group_id
	case "private":
		where = msg.user_id
	}
	if (time.Now().Unix()-int64(parseHistoryList[id].TIME) < duration) && where == parseHistoryList[id].WHERE {
		log.Info("[parse] 在 ", where, " 屏蔽了一次小于 ", duration, " 秒的相同解析 ", kind, id)
		return ""
	}
	if kind != "SHORT" {
		log.Trace("[parse] 记录了一次在 ", where, " 的解析 ", id)
		parseHistoryList[id] = parseHistory{
			where,
			int(time.Now().Unix()),
		}
	}
	switch kind {
	case "DYNAMIC":
		g := getDynamicJson(id)
		if g.Get("code").Int() != 0 {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 动态%s信息获取错误: code%d", id, g.Get("code").Int())
		}
		return formatDynamic(g.Get("data.item"))
	case "ARCHIVEa":
		g, h := getArchiveJsonA(id)
		if g.Get("code").Int() != 0 {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 视频av%s信息获取错误: code%d", id, g.Get("code").Int())
		}
		return formatArchive(g.Get("data"), h.Get("data"))
	case "ARCHIVEb":
		g, h := getArchiveJsonB(id)
		if g.Get("code").Int() != 0 {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 视频%s信息获取错误: code%d", id, g.Get("code").Int())
		}
		return formatArchive(g.Get("data"), h.Get("data"))
	case "ARTICLE":
		g := getArticleJson(id)
		if g.Get("code").Int() != 0 {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 专栏cv%s信息获取错误: code%d", id, g.Get("code").Int())
		}
		return formatArticle(g.Get("data"), id) //专栏信息拿不到自身cv号
	case "SPACE":
		g := getSpaceJson(id)
		if g.Get("code").Int() != 0 {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 用户%s信息获取错误: code%d", id, g.Get("code").Int())
		}
		return formatSpace(g.Get("data.card"))
	case "LIVE":
		uid := strconv.Itoa(getRoomJsonRoomID(id).Get("data.uid").Int())
		if uid == "0" {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 直播间%s信息获取错误, uid == \"0\"", id)
		}
		roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
		if !ok {
			return fmt.Sprintf("[NothingBot] [ERROR] [parse] 直播间%s信息获取错误, !ok", id)
		}
		return formatLive(roomJson)
	case "SHORT":
		id, kind := extractor(deShortLink(id))
		return normalParse(id, kind, msg)
	default:
		return ""
	}
}

func checkParse(msg gocqMessage) {
	var slug string
	var message string
	result := regexp.MustCompile(biliLinkRegexp.SHORT).FindAllStringSubmatch(msg.message, -1)
	if len(result) > 0 {
		slug = result[0][2]
		log.Debug("[parse] 识别到短链: ", slug)
		message = normalParse(slug, "SHORT", msg)
	} else {
		i, k := extractor(msg.message)
		message = normalParse(i, k, msg)
	}
	switch msg.message_type {
	case "group":
		sendMsgSingle(0, msg.group_id, message)
	case "private":
		sendMsgSingle(msg.user_id, 0, message)
	}
}
