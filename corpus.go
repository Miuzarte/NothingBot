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

func checkCorpus(ctx gocqMessage) {
	for i := 0; i < len(v.GetStringSlice("corpus")); i++ { //匹配语料库
		reg := v.GetString(fmt.Sprint("corpus.", i, ".regexp"))
		scene := v.GetString(fmt.Sprint("corpus.", i, ".scene"))
		log.Trace("[corpus] 匹配语料库: ", i, "  正则: ", reg)
		result := regexp.MustCompile(reg).FindAllStringSubmatch(ctx.message, -1)
		if result != nil {
			send := func(i int, ctx gocqMessage) {
				time.Sleep(time.Millisecond * time.Duration(int64(v.GetFloat64(fmt.Sprint("corpus.", i, ".delay"))*1000)))
				sendMsgCTX(ctx, v.GetString(fmt.Sprint("corpus.", i, ".reply")))
			}
			switch scene {
			case "a", "all":
				go send(i, ctx)
			case "p", "private":
				if ctx.message_type == "private" {
					go send(i, ctx)
				}
			case "g", "group":
				if ctx.message_type == "group" {
					go send(i, ctx)
				}
			}
		}
	}
}
