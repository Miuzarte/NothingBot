package main

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

var biliLinkRegexp = struct {
	SHORT    string
	DYNAMIC  string
	ARCHIVEa string
	ARCHIVEb string
	ARTICLE  string
	SPACE    string
	LIVE     string
}{
	SHORT:    `(b23|acg)\.tv\\?/(BV[1-9A-HJ-NP-Za-km-z]{10}|av[0-9]{1,10}|[0-9A-Za-z]{7})`, //暂时应该只有7位  也有可能是av/bv号
	DYNAMIC:  `(t.bilibili.com|dynamic|opus)\\?/([0-9]{18,19})`,                            //应该不会有17位的，可能要有19位
	ARCHIVEa: `video\\?/av([0-9]{1,10})`,                                                   //9位 预留10
	ARCHIVEb: `video\\?/(BV[1-9A-HJ-NP-Za-km-z]{10})`,                                      //恒定BV + 10位base58
	ARTICLE:  `(read\\?/cv|read\\?/mobile\\?/)([0-9]{1,9})`,                                //8位 预留9
	SPACE:    `space\.bilibili\.com\\?/([0-9]{1,16})`,                                      //新uid 16位
	LIVE:     `live\.bilibili\.com\\?/([0-9]{1,9})`,                                        //8位 预留9
}

var everyBiliLinkRegexp = func() string {
	combinedRegex := ""
	structValue := reflect.ValueOf(biliLinkRegexp)
	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		if combinedRegex != "" {
			combinedRegex += "|"
		}
		combinedRegex += field.Interface().(string)
	}
	return combinedRegex
}()

var groupParseHistory = make(map[int]parseHistory) //group/user : av/bv, time

type parseHistory struct {
	parseID string
	TIME    int64
}

//base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz

// 链接提取
func biliLinkExtractor(str string) (id string, kind string) {
	short := regexp.MustCompile(biliLinkRegexp.SHORT).FindAllStringSubmatch(str, -1)
	dynamicID := regexp.MustCompile(biliLinkRegexp.DYNAMIC).FindAllStringSubmatch(str, -1)
	aid := regexp.MustCompile(biliLinkRegexp.ARCHIVEa).FindAllStringSubmatch(str, -1)
	bvid := regexp.MustCompile(biliLinkRegexp.ARCHIVEb).FindAllStringSubmatch(str, -1)
	cvid := regexp.MustCompile(biliLinkRegexp.ARTICLE).FindAllStringSubmatch(str, -1)
	uid := regexp.MustCompile(biliLinkRegexp.SPACE).FindAllStringSubmatch(str, -1)
	roomID := regexp.MustCompile(biliLinkRegexp.LIVE).FindAllStringSubmatch(str, -1)
	log.Trace("[parse] short: ", short)
	log.Trace("[parse] dynamicID: ", dynamicID)
	log.Trace("[parse] aid: ", aid)
	log.Trace("[parse] bvid: ", bvid)
	log.Trace("[parse] cvid: ", cvid)
	log.Trace("[parse] uid: ", uid)
	log.Trace("[parse] roomID: ", roomID)
	switch {
	case len(short) > 0:
		log.Debug("[parse] 识别到一个短链, short[0][2]: ", short[0][2])
		return short[0][2], "SHORT"
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
	}
	return str, ""
}

// 短链解析
func deShortLink(slug string) (location string) {
	url := "https://b23.tv/" + slug
	var statusCode string
	header, err := ihttp.New().WithUrl(url).
		WithHijackRedirect().Head().ToHeader()
	if err != nil {
		log.Error("[parse] deShortLink().ihttp请求错误: ", err)
	}
	if len(header["Location"]) > 0 {
		location = header["Location"][0]
		log.Debug("[parse] 短链解析结果: ", location[0:32])
	}
	if len(header["Bili-Status-Code"]) > 0 {
		statusCode = header["Bili-Status-Code"][0]
	}
	switch statusCode {
	case "-404":
		log.Warn("[parse] 短链解析失败: ", statusCode, "  location: ", location)
	}
	return
}

// 链接内容解析
func biliLinkParse(id string, kind string) string {
	switch kind {
	case "":
		return ""
	case "SHORT":
		id, kind := biliLinkExtractor(deShortLink(id))
		return biliLinkParse(id, kind)
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
	}
	return ""
}

// 短时间重复解析屏蔽
func checkOverParse(ctx gocqMessage, id string, kind string) bool {
	if ctx.message_type == "group" { //只有群聊有限制
		duration := int64(v.GetFloat64("parse.settings.sameParseInterval"))
		during := time.Now().Unix()-groupParseHistory[ctx.group_id].TIME < duration
		same := id == groupParseHistory[ctx.group_id].parseID
		if during && same {
			log.Info("[parse] 在群 ", ctx.group_id, " 屏蔽了一次小于 ", duration, " 秒的相同解析 ", kind, id)
			return false
		} else {
			log.Trace("[parse] 记录了一次在 ", ctx.group_id, " 的解析 ", id)
			groupParseHistory[ctx.group_id] = parseHistory{ //记录解析历史
				parseID: id,
				TIME:    time.Now().Unix(),
			}
		}
	}
	return true
}

// 哔哩哔哩链接解析
func checkParse(ctx gocqMessage) {
	reg := regexp.MustCompile(everyBiliLinkRegexp)
	match := reg.FindAllStringSubmatch(ctx.message, -1)
	if len(match) > 0 {
		log.Debug("[parse] 识别到哔哩哔哩链接: ", match[0][0])
		id, kind := biliLinkExtractor(match[0][0])
		if kind == "SHORT" { //短链先解析提取再往下
			loc := deShortLink(id)
			match = reg.FindAllStringSubmatch(loc, -1)
			if len(match) > 0 {
				log.Debug("[parse] 短链解析结果: ", match[0][0])
				id, kind = biliLinkExtractor(match[0][0])
			} else {
				log.Debug("[parse] 短链解析失败: ", loc)
				return
			}
		}
		if !checkOverParse(ctx, id, kind) {
			return
		}
		ctx.sendMsg(biliLinkParse(id, kind))
	}
}
