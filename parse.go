package main

import (
	"net/http"
	"regexp"
	"strconv"

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
	DYNAMIC:  `(t.bilibili.com|dynamic|opus)/([0-9]{18,19})`,                            //应该不会有17位的，可能要有19位
	ARCHIVEa: `video/av([0-9]{1,10})`,                                                   //9位 预留10
	ARCHIVEb: `video/(BV[1-9A-HJ-NP-Za-km-z]{10})`,                                      //恒定BV + 10位base58
	ARTICLE:  `(read/cv|read/mobile/)([0-9]{1,9})`,                                      //8位 预留9
	SPACE:    `space\.bilibili\.com/([0-9]{1,16})`,                                      //新uid 16位
	LIVE:     `live\.bilibili\.com/([0-9]{1,9})`,                                        //8位 预留9
	SHORT:    `(b23|acg)\.tv/(BV[1-9A-HJ-NP-Za-km-z]{10}|av[0-9]{1,10}|[0-9A-Za-z]{7})`, //暂时应该只有7位  也有可能是av/bv号
}

// base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz

func extractor(str string) (id string, kind string) {
	dynamicID := regexp.MustCompile(biliLinkRegexp.DYNAMIC).FindAllStringSubmatch(str, -1)
	aid := regexp.MustCompile(biliLinkRegexp.ARCHIVEa).FindAllStringSubmatch(str, -1)
	bvid := regexp.MustCompile(biliLinkRegexp.ARCHIVEb).FindAllStringSubmatch(str, -1)
	cvid := regexp.MustCompile(biliLinkRegexp.ARTICLE).FindAllStringSubmatch(str, -1)
	uid := regexp.MustCompile(biliLinkRegexp.SPACE).FindAllStringSubmatch(str, -1)
	roomID := regexp.MustCompile(biliLinkRegexp.LIVE).FindAllStringSubmatch(str, -1)
	log.Traceln("[parse] dynamicID:", dynamicID)
	log.Traceln("[parse] aid:", aid)
	log.Traceln("[parse] bvid:", bvid)
	log.Traceln("[parse] cvid:", cvid)
	log.Traceln("[parse] uid:", uid)
	log.Traceln("[parse] roomID:", roomID)
	switch {
	case len(dynamicID) > 0:
		log.Debugln("[parse] 识别到一个动态, dynamicID[0][2]:", dynamicID[0][2])
		return dynamicID[0][2], "DYNAMIC"
	case len(aid) > 0:
		log.Debugln("[parse] 识别到一个视频(a), aid[0][1]:", aid[0][1])
		return aid[0][1], "ARCHIVEa"
	case len(bvid) > 0:
		log.Debugln("[parse] 识别到一个视频(b), bvid[0][1]:", bvid[0][1])
		return bvid[0][1], "ARCHIVEb"
	case len(cvid) > 0:
		log.Debugln("[parse] 识别到一个专栏, cvid[0][2]:", cvid[0][2])
		return cvid[0][2], "ARTICLE"
	case len(uid) > 0:
		log.Debugln("[parse] 识别到一个用户空间, uid[0][1]:", uid[0][1])
		return uid[0][1], "SPACE"
	case len(roomID) > 0:
		log.Debugln("[parse] 识别到一个直播, roomID[0][1]:", roomID[0][1])
		return roomID[0][1], "LIVE"
	default:
		return str, ""
	}
}

func deShortLink(slug string) string { //短链解析
	url := "https://b23.tv/" + slug
	//base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz
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
		log.Debugln("[parse] 短链解析结果:", location[0:32])
	}
	if len(resp.Header["Bili-Status-Code"]) > 0 {
		statusCode = resp.Header["Bili-Status-Code"][0]
	}
	switch statusCode {
	case "-404":
		log.Warningln("[parse] 短链解析失败:", statusCode)
		log.Warningln("[parse] location:", location)
		return ""
	}
	return location
}

func normalParse(id string, kind string) string { //拿到id直接解析
	switch kind {
	case "DYNAMIC":
		return formatDynamic(getDynamicJson(id).Get("data.item"))
	case "ARCHIVEa":
		return formatArchive(getArchiveJsonA(id).Get("data"))
	case "ARCHIVEb":
		return formatArchive(getArchiveJsonB(id).Get("data"))
	case "ARTICLE":
		return formatArticle(getArticleJson(id).Get("data"), id) //文章信息拿不到自己的cv号
	case "SPACE":
		return formatSpace(getSpaceJson(id).Get("data.card"))
	case "LIVE":
		roomID, _ := strconv.Atoi(id)
		uid := getRoomJsonRoomID(roomID).Get("data.uid").Int()
		roomJson, _ := getRoomJsonUID(uid).Gets("data", strconv.Itoa(uid))
		return formatLive(roomJson)
	case "SHORT":
		return normalParse(extractor(deShortLink(id)))
	default:
		return ""
	}
}

func parseChecker(msg gocqMessage) {
	var slug string
	var message string
	result := regexp.MustCompile(biliLinkRegexp.SHORT).FindAllStringSubmatch(msg.message, -1)
	if len(result) > 0 {
		slug = result[0][2]
		log.Debugln("[parse] 识别到短链:", slug)
		message = normalParse(slug, "SHORT")
	} else {
		message = normalParse(extractor(msg.message))
	}
	switch msg.message_type {
	case "group":
		sendMsgSingle(message, 0, msg.group_id)
	case "private":
		sendMsgSingle(message, msg.user_id, 0)
	}
}
