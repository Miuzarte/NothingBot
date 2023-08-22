package main

import (
	"fmt"
	"strconv"
	"time"

	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

type corpus struct {
	regStr    string
	regexp    *regexp.Regexp
	reply     string
	replyNode []map[string]any
	scene     string
	delay     time.Duration
}

var corpuses []corpus

// 初始化语料库
func initCorpus() {
	for { //拿到selfID才能存合并转发的自身uin
		if selfID != 0 {
			break
		}
	}
	corpuses = []corpus{}
	corpusFound := len(v.GetStringSlice("corpus")) //[]Int没长度
	log.Info("[corpus] 语料库找到 ", corpusFound, " 条")
	var errorLog string
	for i := 0; i < corpusFound; i++ { //读取语料库并验证合法性
		c := corpus{}
		regexpOK := true
		replyOK := true
		sceneOK := true
		delayOK := true

		regRaw := v.Get(fmt.Sprint("corpus.", i, ".regexp"))
		regStr, ok := regRaw.(string)
		regExpression, err := regexp.Compile(regStr)
		switch {
		case !ok:
			regexpOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.regexp: 正则表达式项格式错误!")
		case err != nil:
			regexpOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.regexp: 正则表达式内容语法错误!")
		default:
			regexpOK = true
			c.regStr = regStr
			c.regexp = regExpression
		}

		replyRaw := v.Get(fmt.Sprint("corpus.", i, ".reply"))
		replyString, isString := replyRaw.(string)
		replySlice, isSlice := replyRaw.([]any)
		switch {
		case !isString && !isSlice:
			replyOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.reply: 回复项格式错误!")
		default:
			replyOK = true
			switch {
			case isString:
				c.reply = replyString
			case isSlice:
				for _, a := range replySlice {
					var nodeData gocqNodeData
					str, isString := a.(string)
					node, isCustomNode := a.(map[string]any)
					if isString {
						nodeData.content = []string{str}
					} else if isCustomNode {
						if node["name"] != nil {
							nodeData.name = fmt.Sprint(node["name"])
						}
						if node["uin"] != nil {
							uin, isInt := node["uin"].(int)
							str, isString := node["uin"].(string)
							if isInt {
								nodeData.uin = uin
							} else if isString {
								atoi, err := strconv.Atoi(str)
								if err == nil {
									nodeData.uin = atoi
								} else {
									nodeData.uin = selfID
								}
							}
						}
						if node["content"] != nil {
							contentString, isString := node["content"].(string)
							contentNode, isSlice := node["content"].([]any)
							if isString {
								nodeData.content = []string{contentString}
							} else if isSlice {
								for _, s := range contentNode {
									nodeData.content = append(nodeData.content, fmt.Sprint(s))
								}
							}
						}
					} else {
						nodeData.content = []string{fmt.Sprint(a)}
					}
					c.replyNode = appendForwardNode(c.replyNode, nodeData)
				}
			}
		}

		sceneRaw := v.Get(fmt.Sprint("corpus.", i, ".scene"))
		scene := fmt.Sprint(sceneRaw)
		switch scene {
		case "a", "all", "g", "group", "p", "private":
			sceneOK = true
			c.scene = scene
		default:
			sceneOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.scene: 需要在\"a\",\"g\",\"p\"之间指定一个触发场景!")
		}

		delayRaw := v.Get(fmt.Sprint("corpus.", i, ".delay"))
		delayInt, isInt := delayRaw.(int)
		delayFloat, isFloat := delayRaw.(float64)
		delayString, isString := delayRaw.(string)
		delayParseFloat, err := strconv.ParseFloat(delayString, 64)
		switch {
		case delayRaw == nil:
			delayOK = true
			c.delay = 0
		case (!isInt && !isFloat && !isString) || (isString && err != nil):
			delayOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.delay: 延迟格式错误!  isInt: ", isInt, "  isFloat: ", isFloat, "  isString: ", isString, "  err: ", err)
		default: //(isInt || isFloat || isString) && (!isString || err == nil)
			delayOK = true
			switch {
			case isInt:
				c.delay = time.Millisecond * time.Duration(delayInt*1000)
			case isFloat:
				c.delay = time.Millisecond * time.Duration(delayFloat*1000)
			case isString:
				c.delay = time.Millisecond * time.Duration(delayParseFloat*1000)
			}
		}

		if regexpOK && replyOK && sceneOK && delayOK {
			corpuses = append(corpuses, c)
		} else {
			errorLog += fmt.Sprintf(`
corpus[%d]    regexpOK: %t  replyOK: %t  sceneOK: %t  delayOK: %t`,
				i, regexpOK, replyOK, sceneOK, delayOK)
		}
	}
	corpusAdded := len(corpuses)
	if corpusAdded == corpusFound {
		log.Info("[corpus] 成功解析所有 ", corpusAdded, " 条语料库")
	} else {
		log.Warn("[corpus] 仅成功解析 ", corpusAdded, " 条语料库, 错误列表: ", errorLog)
		log2SU.Warn("仅成功解析 ", corpusAdded, " 条语料库，详细错误信息请查看控制台输出")
	}
	for i, c := range corpuses {
		log.Trace("[corpus] ", i, "  -  regexp: ", c.regStr, "  reply: ", c.reply, func(str string) string {
			if str == "null" {
				return "  "
			}
			return "\n" + str + "\n"
		}(gson.New(c.replyNode).JSON("", "")), "scene: ", c.scene, "  delay: ", c.delay)
	}
}

// 语料库
func checkCorpus(ctx gocqMessage) {
	for i, c := range corpuses {
		log.Trace("[corpus] 匹配语料库: ", i, "   正则: ", c.regStr)
		result := c.regexp.FindAllStringSubmatch(ctx.message, -1)
		if result != nil {
			var ok bool
			switch c.scene {
			case "a", "all":
				ok = true
			case "p", "private":
				if ctx.message_type == "private" {
					ok = true
				}
			case "g", "group":
				if ctx.message_type == "group" {
					ok = true
				}
			}
			if ok {
				log.Info("[corpus] 成功匹配语料库 ", i, "   正则: ", c.regStr)
				go func(c corpus) {
					time.Sleep(c.delay)
					if c.reply != "" {
						ctx.sendMsg(c.reply[0])
					} else {
						ctx.sendForwardMsg(c.replyNode)
					}
				}(c)
			}
		}
	}
}
