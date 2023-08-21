package main

import (
	"fmt"
	"strconv"
	"time"

	"regexp"

	log "github.com/sirupsen/logrus"
)

type corpus struct {
	regStr    string
	regexp    *regexp.Regexp
	regexpOK  bool
	reply     string
	replyNode []map[string]any
	replyOK   bool
	scene     string
	sceneOK   bool
	delay     time.Duration
	delayOK   bool
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
		c := corpus{
			regexpOK: true,
			replyOK:  true,
			sceneOK:  true,
			delayOK:  true,
		}

		regRaw := v.Get(fmt.Sprint("corpus.", i, ".regexp"))
		regStr, ok := regRaw.(string)
		regExpression, err := regexp.Compile(regStr)
		switch {
		case !ok:
			c.regexpOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库: 正则表达式项格式错误!")
		case err != nil:
			c.regexpOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库: 正则表达式内容语法错误!")
		default:
			c.regexpOK = true
			c.regStr = regStr
			c.regexp = regExpression
		}

		replyRaw := v.Get(fmt.Sprint("corpus.", i, ".reply"))
		replyString, isString := replyRaw.(string)
		replySlice, isSlice := replyRaw.([]any)

		switch {
		case !isString && !isSlice:
			c.replyOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库: 回复项格式错误!")
		default:
			c.replyOK = true
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
		case "a", "all", "p", "private", "g", "group":
			c.sceneOK = true
			c.scene = scene
		default:
			c.sceneOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库: 必须指定触发场景!")
		}

		delayRaw := v.Get(fmt.Sprint("corpus.", i, ".delay"))
		delayInt, isInt := delayRaw.(int)
		delayFloat, isFloat := delayRaw.(float64)
		delayString, isString := delayRaw.(string)
		delayParseFloat, err := strconv.ParseFloat(delayString, 64)
		switch {
		case delayRaw == nil:
			c.delayOK = true
			c.delay = 0
		case (!isInt && !isFloat && !isString) || (isString && err != nil):
			c.delayOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库: 延迟格式错误!  isInt: ", isInt, "  isFloat: ", isFloat, "  isString: ", isString, "  err: ", err)
		default: //(isInt || isFloat || isString) && err == nil
			c.delayOK = true
			switch {
			case isInt:
				c.delay = time.Millisecond * time.Duration(delayInt*1000)
			case isFloat:
				c.delay = time.Millisecond * time.Duration(delayFloat*1000)
			case isString:
				c.delay = time.Millisecond * time.Duration(delayParseFloat*1000)
			}
		}

		allOK := c.regexpOK && c.replyOK && c.sceneOK && c.delayOK
		if allOK {
			corpuses = append(corpuses, c)
		} else {
			switch {
			case !c.regexpOK:
				fallthrough
			case !c.replyOK:
				fallthrough
			case !c.sceneOK:
				fallthrough
			case !c.delayOK:
				fallthrough
			default:
				errorLog += fmt.Sprintf(`
corpus[%d]    regexpOK: %t  replyOK: %t  sceneOK: %t  delayOK: %t`,
					i, c.regexpOK, c.replyOK, c.sceneOK, c.delayOK)
			}
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
		log.Trace("[corpus] ", i, ": ", c)
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
				if c.reply != "" {
					go func(c corpus) {
						time.Sleep(c.delay)
						ctx.sendMsg(c.reply[0])
					}(c)
				} else {
					go func(c corpus) {
						time.Sleep(c.delay)
						ctx.sendForwardMsg(c.replyNode)
					}(c)
				}
			}
		}
	}
}
