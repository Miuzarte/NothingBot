package main

import (
	"fmt"
	"time"

	"regexp"

	log "github.com/sirupsen/logrus"
)

func initCorpus() {
	log.Info("[corpus] 语料库找到 ", len(v.GetStringSlice("corpus")), " 条")
}

func checkCorpus(msg gocqMessage) { //msg.message_type: "private"/"group"
	for i := 0; i < len(v.GetStringSlice("corpus")); i++ { //匹配语料库
		reg := v.GetString(fmt.Sprintf("corpus.%d.regexp", i))
		scene := v.GetString(fmt.Sprintf("corpus.%d.scene", i))
		log.Trace("[corpus] 匹配语料库: ", i, "  正则: ", reg)
		result := regexp.MustCompile(reg).FindAllStringSubmatch(msg.message, -1)
		if result != nil {
			switch {
			case scene == "a" || scene == "all":
				go sendCorpusResponse(i, msg)
			case (scene == "p" || scene == "private") && msg.message_type == "private":
				go sendCorpusResponse(i, msg)
			case (scene == "g" || scene == "group") && msg.message_type == "group":
				go sendCorpusResponse(i, msg)
			}
		}
	}
}

func sendCorpusResponse(i int, msg gocqMessage) {
	time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64(fmt.Sprintf("corpus.%d.delay", i))*1000)))
	switch msg.message_type {
	case "private":
		sendMsgSingle(msg.user_id, 0, v.GetString(fmt.Sprintf("corpus.%d.reply", i)))
	case "group":
		sendMsgSingle(0, msg.group_id, v.GetString(fmt.Sprintf("corpus.%d.reply", i)))
	}
}
