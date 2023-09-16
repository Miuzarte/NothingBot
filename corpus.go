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
	gbwg.Wait() //拿到selfID才能存合并转发的自身uin
	corpuses = []corpus{}
	corpusFound := len(v.GetStringSlice("corpus")) //[]Int没长度
	log.Info("[corpus] 语料库找到 ", corpusFound, " 条")
	var errorLog string
	for i := 0; i < corpusFound; i++ { //读取语料库并验证合法性
		c := corpus{}
		var regexpOK bool
		var replyOK bool
		var sceneOK bool
		var delayOK bool

		regRaw := v.Get(fmt.Sprint("corpus.", i, ".regexp"))
		regStr, ok := regRaw.(string)
		regCompiler, err := regexp.Compile(regStr)
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
			c.regexp = regCompiler
		}

		replyRaw := v.Get(fmt.Sprint("corpus.", i, ".reply"))
		replyInt, isInt := replyRaw.(int)
		replyFloat, isFloat := replyRaw.(float64)
		replyString, isString := replyRaw.(string)
		replySlice, isSlice := replyRaw.([]any)
		switch {
		case !isInt && !isFloat && !isString && !isSlice:
			replyOK = false
			log.Error("[corpus] 第 ", i+1, " 条语料库.reply: 回复项格式错误!")
		default:
			replyOK = true
			switch {
			case isInt:
				c.reply = fmt.Sprint(replyInt)
			case isFloat:
				c.reply = fmt.Sprint(replyFloat)
			case isString:
				c.reply = replyString
			case isSlice:
				for _, a := range replySlice {
					var nodeData gocqNodeData
					aInt, isInt := a.(int)
					aFloat, isFloat := a.(float64)
					aStr, isString := a.(string)
					aNode, isCustomNode := a.(map[string]any)
					switch {
					case isInt:
						nodeData.content = []string{fmt.Sprint(aInt)}
					case isFloat:
						nodeData.content = []string{fmt.Sprint(aFloat)}
					case isString:
						nodeData.content = []string{aStr}
					case isCustomNode:
						if aNode["name"] != nil {
							nodeData.name = fmt.Sprint(aNode["name"])
						}
						if aNode["uin"] != nil {
							uinInt, isInt := aNode["uin"].(int)
							uinStr, isString := aNode["uin"].(string)
							if isInt {
								nodeData.uin = uinInt
							} else if isString {
								uinAtoi, err := strconv.Atoi(uinStr)
								if err == nil {
									nodeData.uin = uinAtoi
								}
							}
						}
						if aNode["content"] != nil {
							contentString, isString := aNode["content"].(string)
							contentNode, isSlice := aNode["content"].([]any)
							if isString {
								nodeData.content = []string{contentString}
							} else if isSlice {
								for _, s := range contentNode {
									nodeData.content = append(nodeData.content, fmt.Sprint(s))
								}
							}
						}
					default:
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
		default: // (isInt || isFloat || isString) && (!isString || err == nil)
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
func checkCorpus(ctx *gocqMessage) {
	for i, c := range corpuses {
		log.Trace("[corpus] 匹配语料库: ", i, "   正则: ", c.regStr)
		match := c.regexp.FindAllStringSubmatch(ctx.message, -1)
		if len(match) > 0 {
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
