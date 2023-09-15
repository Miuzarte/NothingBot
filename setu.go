package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

type setu struct {
	R18        int      `json:"r18,omitempty"`
	Num        int      `json:"num,omitempty"`
	Uid        []int    `json:"uid,omitempty"`
	Keyword    string   `json:"keyword,omitempty"`
	Tag        []string `json:"tag,omitempty"`
	Size       []string `json:"size,omitempty"`
	Proxy      []string `json:"proxy,omitempty"`
	DateAfter  int      `json:"dateAfter,omitempty"`  //ms
	DateBefore int      `json:"dateBefore,omitempty"` //ms
	Desc       bool     `json:"desc,omitempty"`
	ExcludeAI  bool     `json:"excludeAI,omitempty"`
}

var useOriginalUrl bool

var numberMap = map[string]int{"两": 2,
	"一": 1, "二": 2, "三": 3, "四": 4, "五": 5,
	"壹": 1, "贰": 2, "叁": 3, "肆": 4, "伍": 5,
	"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	"陆": 6, "柒": 7, "捌": 8, "玖": 9, "拾": 10,
	"十一": 11, "十二": 12, "十三": 13, "十四": 14, "十五": 15,
	"拾壹": 11, "拾贰": 12, "拾叁": 13, "拾肆": 14, "拾伍": 15,
	"十六": 16, "十七": 17, "十八": 18, "十九": 19, "二十": 20,
	"拾陆": 16, "拾柒": 17, "拾捌": 18, "拾玖": 19, "贰拾": 20,
}

func checkSetu(ctx gocqMessage) {
	match := ctx.unescape().regexpMustCompile(`(来(?P<num>点|一点|几张|几份|.*张|.*份)?(?P<r18>[Rr]18)?的?(?P<tag>.*)?的?[色瑟涩铯][图圖])|((?P<r18>[Rr]18)?的?(?P<tag>.*)?的?[色瑟涩铯][图圖]来(?P<num>点|一点|几张|几份|.*张|.*份)?)`)
	// 一条正则多个同名捕获组只会索引到第一个, 所以下面直接把对应的捕获组全加起来
	if len(match) > 0 && ctx.isToMe() {
		var numOK bool
		reqR18 := 0
		reqNum := 1
		var reqTag []string
		r18 := match[0][3] + match[0][6]
		if r18 != "" {
			reqR18 = 1
		}
		log.Debug("[setu] r18: ", r18)
		log.Debug("[setu] reqR18: ", reqR18)

		num := match[0][2] + match[0][8]
		switch num {
		case "张", "份":
			reqNum = 1
			numOK = true
		case "点", "一点", "几张", "几份":
			reqNum = rand.Intn(4) + 3 // [3,6]
			numOK = true
		default:
			numNoUnit := strings.NewReplacer("张", "", "份", "").Replace(num)
			numChar, found := numberMap[numNoUnit]
			if !found {
				numInt, err := strconv.Atoi(numNoUnit)
				if err != nil {
				} else {
					if numInt >= 1 && numInt <= 20 {
						reqNum = numInt
						numOK = true
					}
				}
			} else {
				if numChar >= 1 && numChar <= 20 {
					reqNum = numChar
					numOK = true
				}
			}
		}
		log.Debug("[setu] num: ", num)
		log.Debug("[setu] reqNum: ", reqNum)

		tag := match[0][4] + match[0][7]
		reqTag = regexp.MustCompile(`&`).Split(tag, -1)
		log.Debug("[setu] tag: ", tag)
		log.Debug("[setu] reqTag: ", reqTag)

		if !numOK {
			ctx.sendMsgReply("[setu] 请在1-20之间选择数量")
			return
		} else {
			var forwardNode []map[string]any
			setu := setu{
				R18:  reqR18,
				Num:  reqNum,
				Tag:  reqTag,
				Size: []string{"original", "regular"},
			}
			results, err := setu.get()
			if err == nil {
				resultsCount := len(results)
				content := []string{fmt.Sprintf("r18: %t\nnum: %d\ntag: %v", func() bool {
					return reqR18 != 0
				}(), reqNum, reqTag)}
				content = append(content, func() (head string) {
					head += fmt.Sprint("在 api.lolicon.app/setu/v2 根据以上条件搜索到了", resultsCount, "张setu")
					if reqNum > resultsCount {
						head += "\nあれれ？似乎没有那么多符合这个条件的setu呢"
					}
					return
				}())
				content = append(content, results...)
				forwardNode = appendForwardNode(forwardNode, gocqNodeData{
					content: content,
				})
				ctx.sendForwardMsg(forwardNode)
			} else {
				ctx.sendMsg(err.Error())
			}
		}
	}
}

func (param setu) get() (results []string, err error) {
	postData, _ := json.Marshal(param)
	log.Debug("[setu] 请求setu: ", param)
	setu, err := ihttp.New().WithUrl("https://api.lolicon.app/setu/v2").
		WithHeader("Content-Type", "application/json").
		WithBody(postData).Post().ToGson()
	if err != nil {
		err = errors.New("[setu] getSetu().ihttp请求错误: " + err.Error())
		log.Error(err)
	} else if setu.Get("error").Str() != "" && setu.Get("error").Str() != "<nil>" {
		err = errors.New("[setu] getSetu().ihttp响应错误: " + setu.Get("error").Str())
		log.Error(err)
	}
	data, _ := setu.Gets("data")
	if len(data.Arr()) == 0 {
		return []string{}, errors.New("[NothingBot] [setu] 没有符合该条件的色图")
	}
	for _, g := range data.Arr() {
		urlOriginal := g.Get("urls.original").Str()
		urlRegular := g.Get("urls.regular").Str()
		url := func() (url string) {
			if useOriginalUrl {
				url = urlOriginal
			} else {
				url = urlRegular
			}
			return
		}()
		title := g.Get("title").Str()
		author := g.Get("author").Str()
		uid := g.Get("uid").Int()
		tag := func(tags []gson.JSON) (tag string) {
			tagsLen := len(tags)
			for i := 0; i < tagsLen; i++ {
				tag += tags[i].Str()
				if i+1 < tagsLen {
					tag += "，"
				}
			}
			return
		}(g.Get("tags").Arr())
		pid := g.Get("pid").Int()
		p := g.Get("p").Int()
		results = append(results, fmt.Sprintf(
			`[CQ:image,file=%s]
%s    （P%d）
%s
作者：%s
pixiv.net/u/%d
pixiv.net/i/%d
原图：%s`,
			url,
			title, p,
			tag,
			author,
			uid,
			pid,
			urlOriginal))
	}
	return
}
