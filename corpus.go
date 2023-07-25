package main

import (
	"fmt"
	"time"

	"regexp"

	log "github.com/sirupsen/logrus"
)

func initCorpus() {
	log.Infoln("[corpus] 语料库找到", len(v.GetStringSlice("corpus")), "条")
}

func corpusChecker(msg gocqMessage) { //msg.message_type: "private"/"group"
	for i := 0; i < len(v.GetStringSlice("corpus")); i++ { //匹配语料库
		log.Traceln("[corpus] 匹配语料库:", i)
		reg := v.GetString(fmt.Sprintf("corpus.%d.regexp", i))
		scene := v.GetString(fmt.Sprintf("corpus.%d.scene", i))
		log.Traceln("[corpus] 正则:", reg)
		matcher := func() bool {
			result := regexp.MustCompile(reg).FindAllStringSubmatch(msg.message, -1)
			if result != nil {
				switch {
				case scene == "a" || scene == "all":
					return true
				case (scene == "p" || scene == "private") && msg.message_type == "private":
					return true
				case (scene == "g" || scene == "group") && msg.message_type == "group":
					return true
				}
			}
			return false
		}()
		log.Traceln("[corpus] 匹配结果:", matcher)
		if matcher {
			go func(i int) {
				time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64(fmt.Sprintf("corpus.%d.delay", i))*1000)))
				sendMsgSingle(v.GetString(fmt.Sprintf("corpus.%d.reply", i)), msg.user_id, msg.group_id)
			}(i)
		}
	}
	return
}
