package main

import (
	"encoding/json"
	"fmt"
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

var numberMap = map[string]int{
	"一": 1, "二": 2, "两": 2, "三": 3, "四": 4, "五": 5,
	"六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
	"十一": 11, "十二": 12, "十三": 13, "十四": 14, "十五": 15,
	"十六": 16, "十七": 17, "十八": 18, "十九": 19, "二十": 20,
}

func checkSetu(ctx gocqMessage) {
	msg := unescape.Replace(ctx.message)
	reg := regexp.MustCompile(`^(?P<r18>[Rr]18)?的?(?P<tag>.*)?的?[色瑟涩铯][图圖]来(?P<num>点|.*张)?$|^来(?P<num>点|.*张)?(?P<r18>[Rr]18)?的?(?P<tag>.*)?的?[色瑟涩铯][图圖]$`)
	match := reg.FindAllStringSubmatch(msg, -1) // 一条正则多个同名捕获组只会索引第一个
	var ok bool
	reqR18 := 0
	reqNum := 1
	reqTag := []string{}
	if len(match) > 0 {
		r18 := match[0][1] + match[0][5]
		if r18 != "" {
			reqR18 = 1
		}
		num := strings.ReplaceAll(match[0][3]+match[0][4], "张", "")
		if num == "" || num == "点" {
			reqNum = 1
			ok = true
		} else {
			numChar, found := numberMap[num]
			if !found {
				numInt, err := strconv.Atoi(num)
				if err != nil {
				} else {
					if numInt >= 1 && numInt <= 20 {
						reqNum = numInt
						ok = true
					}
				}
			} else {
				if numChar >= 1 && numChar <= 20 {
					reqNum = numChar
					ok = true
				}
			}
		}
		reqTag = regexp.MustCompile(`&`).Split(match[0][2]+match[0][6], -1)
		if !ok {
			ctx.sendMsg("[setu] 请在1-20之间选择数量")
			return
		} else {
			var forwardNode []map[string]any
			setu := setu{
				R18:  reqR18,
				Num:  reqNum,
				Tag:  reqTag,
				Size: []string{"original", "regular"},
			}
			results, errMsg := setu.get()
			if errMsg == "" {
				resultsCount := len(results)
				content := []string{fmt.Sprintf("r18: %t\nnum: %d\ntag: %v", func() bool {
					if reqR18 == 0 {
						return false
					}
					return true
				}(), reqNum, reqTag)}
				content = append(content, func() (head string) {
					head += fmt.Sprint("在api.lolicon.app/setu/v2根据以上条件搜索到了", resultsCount, "张setu")
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
				ctx.sendMsg(errMsg)
			}
		}
	}
}

func (param setu) get() (results []string, errMsg string) {
	postData, _ := json.Marshal(param)
	log.Debug("[setu] 请求setu: ", param)
	setu, err := ihttp.New().WithUrl("https://api.lolicon.app/setu/v2").
		WithHeader("Content-Type", "application/json").
		WithBody(postData).Post().ToGson()
	if err != nil {
		log.Error("[setu] getSetu().ihttp请求错误: ", err)
		errMsg += fmt.Sprint("[setu] getSetu().ihttp请求错误: ", err)
	} else if setu.Get("error").Str() != "" && setu.Get("error").Str() != "<nil>" {
		log.Error("[setu] getSetu().ihttp响应错误: ", setu.Get("error").Str())
		errMsg += fmt.Sprint("[setu] getSetu().ihttp响应错误: ", setu.Get("error").Str())
	}
	data, _ := setu.Gets("data")
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
