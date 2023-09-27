package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"strconv"
	"time"

	"regexp"

	log "github.com/sirupsen/logrus"
)

type corpus struct {
	regstr          string
	regexp          *regexp.Regexp
	reply           string
	replyForwardMsg EasyBot.CQForwardMsg
	scene           string
	delay           time.Duration
}

var allCorpuses []corpus

// 初始化语料库
func initCorpus() {
	allCorpuses = []corpus{}
	configsRaw := v.Get("corpus")
	if configsRaw == nil {
		log.Info("[Init] 未找到语料库")
		return
	}
	configs, isArr := configsRaw.([]any)
	if !isArr {
		log.Error("[Init] corpus 内容应为数组")
		return
	}
	for i, config := range configs {
		configMap, isMap := config.(map[string]any)
		if !isMap {
			log.Error("[Init] corpus.", i, " 内容数据类型错误")
			continue
		}

		corpus := corpus{}

		if regStr := fmt.Sprint(configMap["regexp"]); regStr != "" {
			corpus.regstr = regStr
		} else {
			log.Error("[Init] corpus.", i, ".regexp 正则表达式非法, 禁止为空")
			continue
		}

		if regExp, err := regexp.Compile(corpus.regstr); err == nil {
			corpus.regexp = regExp
		} else {
			log.Error("[Init] corpus.", i, ".regexp 正则表达式非法, err: ", err)
			continue
		}

		if sceneStr := fmt.Sprint(configMap["scene"]); sceneStr == "a" || sceneStr == "all" || sceneStr == "g" || sceneStr == "group" || sceneStr == "p" || sceneStr == "private" {
			corpus.scene = sceneStr
		} else {
			log.Error("[Init] corpus.", i, ".scene 需要在\"a\",\"g\",\"p\"之间指定一个触发场景")
			continue
		}

		corpus.delay = func() (delay time.Duration) {
			delayRaw := v.Get(fmt.Sprint("corpus.", i, ".delay"))
			delayInt, isInt := delayRaw.(int)
			delayFloat, isFloat := delayRaw.(float64)
			delayString, isString := delayRaw.(string)
			delayParseFloat, err := strconv.ParseFloat(delayString, 64)
			switch {
			case delayRaw == nil:
				return 0
			case (!isInt && !isFloat && !isString) || (isString && err != nil):
				log.Error("[Init] corpus.", i, ".scene 延迟格式错误    isInt: ", isInt, "  isFloat: ", isFloat, "  isString: ", isString, "  err: ", err)
				return 0
			default: // (isInt || isFloat || isString) && (!isString || err == nil)
				switch {
				case isInt:
					return time.Millisecond * 1000 * time.Duration(delayInt)
				case isFloat:
					return time.Millisecond * 1000 * time.Duration(delayFloat)
				case isString:
					return time.Millisecond * 1000 * time.Duration(delayParseFloat)
				}
			}
			return 0
		}()

		corpus.reply, corpus.replyForwardMsg = func() (string, EasyBot.CQForwardMsg) {
			replySlice, isSlice := configMap["reply"].([]any)
			switch {
			default: //corpus.reply
				return fmt.Sprint(configMap["reply"]), nil

			case isSlice: //corpus.replyForwardMsg
				return "", func() (forwardMsg EasyBot.CQForwardMsg) {

					for _, eachReply := range replySlice {

						if eachReplyMap, isMap := eachReply.(map[string]any); isMap { //自定义名字、头像
							//使用自定义信息
							name := func() (name string) {
								if nameAny := eachReplyMap["name"]; nameAny != nil {
									return fmt.Sprint(nameAny)
								}
								return "NothingBot"
							}()
							uin := func() (uin int) {
								if uinAny := eachReplyMap["uin"]; uinAny != nil {
									if uinInt, isInt := uinAny.(int); isInt {
										return uinInt
									}
								}
								return bot.GetSelfID()
							}()
							timestamp := func() (timestamp int64) {
								if tsAny := eachReplyMap["time"]; tsAny != nil {
									if tsInt, isInt := tsAny.(int); isInt {
										return int64(tsInt)
									}
								}
								return 0
							}()
							seq := func() (seq int64) {
								if seqAny := eachReplyMap["time"]; seqAny != nil {
									if seqInt, isInt := seqAny.(int); isInt {
										return int64(seqInt)
									}
								}
								return 0
							}()

							eachReplyMapContentSlice, isSlice := eachReplyMap["content"].([]any)
							if !isSlice {
								forwardMsg = EasyBot.AppendForwardMsg(forwardMsg,
									EasyBot.NewForwardNode(
										name,
										uin,
										fmt.Sprint(eachReplyMap["content"]),
										timestamp, seq,
									))
							}
							if isSlice {
								for _, eachEachReplyMapContentSlice := range eachReplyMapContentSlice {
									forwardMsg = EasyBot.AppendForwardMsg(forwardMsg,
										EasyBot.NewForwardNode(
											name,
											uin,
											fmt.Sprint(eachEachReplyMapContentSlice),
											timestamp, seq,
										))
								}
							}

						} else { //未自定义名字、头像

							//使用bot信息
							forwardMsg = EasyBot.AppendForwardMsg(forwardMsg,
								EasyBot.NewForwardNode(
									"NothingBot",
									bot.GetSelfID(),
									fmt.Sprint(eachReply),
									0, 0,
								))

						}
					}
					return
				}()
			}
		}()

		allCorpuses = append(allCorpuses, corpus)
	}
}

// 语料库
func checkCorpus(ctx *EasyBot.CQMessage) {
	for i, c := range allCorpuses {
		log.Trace("[corpus] 匹配语料库: ", i, "   正则: ", c.regstr)
		matches := c.regexp.FindAllStringSubmatch(ctx.RawMessage, -1)
		if len(matches) > 0 {
			var ok bool
			switch c.scene {
			case "a", "all":
				ok = true
			case "p", "private":
				if ctx.MessageType == "private" {
					ok = true
				}
			case "g", "group":
				if ctx.MessageType == "group" {
					ok = true
				}
			}
			if ok {
				log.Info("[corpus] 成功匹配语料库 ", i, "   正则: ", c.regstr)
				go func(c corpus) {
					time.Sleep(c.delay)
					if c.reply != "" {
						ctx.SendMsg(c.reply)
					} else {
						ctx.SendForwardMsg(c.replyForwardMsg)
					}
				}(c)
			}
		}
	}
}
