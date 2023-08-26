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
	SHORT     string
	DYNAMIC   string
	ARCHIVEav string
	ARCHIVEbv string
	ARTICLE   string
	SPACE     string
	LIVE      string
}{
	SHORT:     `(总结一下)?.*(b23|acg)\.tv\\?/(BV[1-9A-HJ-NP-Za-km-z]{10}|av[0-9]{1,10}|[0-9A-Za-z]{7})`, //暂时应该只有7位  也有可能是av/bv号
	DYNAMIC:   `(总结一下)?.*(t.bilibili.com|dynamic|opus)\\?/([0-9]{18,19})`,                            //应该不会有17位的，可能要有19位
	ARCHIVEav: `(总结一下)?.*video\\?/av([0-9]{1,10})`,                                                   //9位 预留10
	ARCHIVEbv: `(总结一下)?.*video\\?/(BV[1-9A-HJ-NP-Za-km-z]{10})`,                                      //恒定BV + 10位base58
	ARTICLE:   `(总结一下)?.*(read\\?/cv|read\\?/mobile\\?/)([0-9]{1,9})`,                                //8位 预留9
	SPACE:     `(总结一下)?.*space\.bilibili\.com\\?/([0-9]{1,16})`,                                      //新uid 16位
	LIVE:      `(总结一下)?.*live\.bilibili\.com\\?/([0-9]{1,9})`,                                        //8位 预留9
}

var everyBiliLinkRegexp = func() (everyBiliLinkRegexp string) {
	structValue := reflect.ValueOf(biliLinkRegexp)
	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		if everyBiliLinkRegexp != "" {
			everyBiliLinkRegexp += "|"
		}
		everyBiliLinkRegexp += field.Interface().(string)
	}
	return
}()

var groupParseHistory = make(map[int]parseHistory) //group:

type parseHistory struct {
	parse string
	time  int64
}

// 链接提取
func extractBiliLink(str string) (id string, kind string, summary bool) {
	short := regexp.MustCompile(biliLinkRegexp.SHORT).FindAllStringSubmatch(str, -1)
	dynamic := regexp.MustCompile(biliLinkRegexp.DYNAMIC).FindAllStringSubmatch(str, -1)
	av := regexp.MustCompile(biliLinkRegexp.ARCHIVEav).FindAllStringSubmatch(str, -1)
	bv := regexp.MustCompile(biliLinkRegexp.ARCHIVEbv).FindAllStringSubmatch(str, -1)
	cv := regexp.MustCompile(biliLinkRegexp.ARTICLE).FindAllStringSubmatch(str, -1)
	space := regexp.MustCompile(biliLinkRegexp.SPACE).FindAllStringSubmatch(str, -1)
	live := regexp.MustCompile(biliLinkRegexp.LIVE).FindAllStringSubmatch(str, -1)
	log.Trace("[parse] short: ", short)
	log.Trace("[parse] dynamic: ", dynamic)
	log.Trace("[parse] av: ", av)
	log.Trace("[parse] bv: ", bv)
	log.Trace("[parse] cv: ", cv)
	log.Trace("[parse] space: ", space)
	log.Trace("[parse] live: ", live)
	sumTest := func(sumStr string) (summary bool) {
		if sumStr == "总结一下" {
			summary = true
		}
		return
	}
	switch {
	case len(short) > 0:
		log.Debug("[parse] 识别到一个短链, short[0][3]: ", short[0][3])
		id = short[0][3]
		kind = "SHORT"
		summary = sumTest(short[0][1])
	case len(dynamic) > 0:
		log.Debug("[parse] 识别到一个动态, dynamic[0][3]: ", dynamic[0][3])
		id = dynamic[0][3]
		kind = "DYNAMIC"
		summary = sumTest(dynamic[0][1])
	case len(av) > 0:
		log.Debug("[parse] 识别到一个视频(av), av[0][2]: ", av[0][2])
		id = av[0][2]
		kind = "ARCHIVE"
		summary = sumTest(av[0][1])
	case len(bv) > 0:
		log.Debug("[parse] 识别到一个视频(bv), bv[0][2]: ", bv[0][2])
		id = strconv.Itoa(bv2av(bv[0][2]))
		kind = "ARCHIVE"
		summary = sumTest(bv[0][1])
	case len(cv) > 0:
		log.Debug("[parse] 识别到一个专栏, cv[0][3]: ", cv[0][3])
		id = cv[0][3]
		kind = "ARTICLE"
		summary = sumTest(cv[0][1])
	case len(space) > 0:
		log.Debug("[parse] 识别到一个用户空间, space[0][2]: ", space[0][2])
		id = space[0][2]
		kind = "SPACE"
		summary = sumTest(space[0][1])
	case len(live) > 0:
		log.Debug("[parse] 识别到一个直播, live[0][2]: ", live[0][2])
		id = live[0][2]
		kind = "LIVE"
		summary = sumTest(live[0][1])
	}
	return
}

// 内容解析并格式化
func parseAndFormatBiliLink(id string, kind string, summary bool) (content string) {
	switch kind {
	case "":
	case "SHORT":
		content = parseAndFormatBiliLink(extractBiliLink(deShortLink(id)))
	case "DYNAMIC":
		g := getDynamicJson(id)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 动态%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatDynamic(g.Get("data.item"))
			if summary {
				content += "\n动态暂时不支持总结捏"
			}
		}
	case "ARCHIVE":
		g, h := getArchiveJson(id)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 视频av%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatArchive(g.Get("data"), h.Get("data"))
			if summary {
				content += "\n由newbing总结的视频内容：" + newbingSummary("")
			}
		}
	case "ARTICLE":
		id, _ := strconv.Atoi(id)
		g := getArticleJson(id)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 专栏cv%d信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatArticle(g.Get("data"), id) //专栏信息拿不到自身cv号
		}
	case "SPACE":
		g := getSpaceJson(id)
		if g.Get("code").Int() != 0 {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 用户%s信息获取错误: code%d", id, g.Get("code").Int())
		} else {
			content = formatSpace(g.Get("data.card"))
		}
	case "LIVE":
		id, _ := strconv.Atoi(id)
		uid := getRoomJsonRoomid(id).Get("data.uid").Int()
		if uid != 0 {
			roomJson, ok := getRoomJsonUID(uid).Gets("data", uid)
			if ok {
				content = formatLive(roomJson)
			} else {
				content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 直播间%d信息获取错误, !ok", id)
			}
		} else {
			content = fmt.Sprintf("[NothingBot] [ERROR] [parse] 直播间%d信息获取错误, uid == \"0\"", id)
		}
	}
	return
}

// newbing总结
func newbingSummary(input string) (output string) {
	if input == "" {
		return ""
	}
	return
}

// 短链解析
func deShortLink(slug string) (location string) {
	header, err := ihttp.New().WithUrl("https://b23.tv/" + slug).
		WithHijackRedirect().Head().ToHeader()
	if err != nil {
		log.Error("[parse] deShortLink().ihttp请求错误: ", err)
	}
	if len(header["Location"]) > 0 {
		location = header["Location"][0]
		log.Debug("[parse] 短链解析结果: ", location[0:32])
	}
	var statusCode string
	if len(header["Bili-Status-Code"]) > 0 {
		statusCode = header["Bili-Status-Code"][0]
	}
	switch statusCode {
	case "-404":
		log.Warn("[parse] 短链解析失败: ", statusCode, "  location: ", location)
	}
	return
}

// 短时间重复解析屏蔽
func (ctx gocqMessage) isBiliLinkOverParse(id string, kind string) bool {
	if ctx.message_type == "group" { //只有群聊有限制
		duration := int64(v.GetFloat64("parse.settings.sameParseInterval"))
		during := time.Now().Unix()-groupParseHistory[ctx.group_id].time < duration
		same := id == groupParseHistory[ctx.group_id].parse
		if during && same {
			log.Info("[parse] 在群 ", ctx.group_id, " 屏蔽了一次小于 ", duration, " 秒的相同解析 ", kind, id)
			return true
		} else {
			log.Trace("[parse] 记录了一次在 ", ctx.group_id, " 的解析 ", id)
			groupParseHistory[ctx.group_id] = parseHistory{ //记录解析历史
				parse: id,
				time:  time.Now().Unix(),
			}
		}
	}
	return false
}

// 哔哩哔哩链接解析
func checkParse(ctx gocqMessage) {
	reg := regexp.MustCompile(everyBiliLinkRegexp)
	match := reg.FindAllStringSubmatch(ctx.message, -1)
	if len(match) > 0 {
		log.Debug("[parse] 识别到哔哩哔哩链接: ", match[0][0])
		id, kind, summary := extractBiliLink(match[0][0])
		if kind == "SHORT" { //短链先解析提取再往下, 保证parseHistory里没有短链
			loc := deShortLink(id)
			match = reg.FindAllStringSubmatch(loc, -1)
			if len(match) > 0 {
				log.Debug("[parse] 短链解析结果: ", match[0][0])
				id, kind, summary = extractBiliLink(match[0][0])
			} else {
				log.Debug("[parse] 短链解析失败: ", loc)
				return
			}
		}
		if ctx.isBiliLinkOverParse(id, kind) {
			return
		}
		ctx.sendMsg(parseAndFormatBiliLink(id, kind, summary))
	}
}
