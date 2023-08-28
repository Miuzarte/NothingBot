package main

import (
	"encoding/json"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

var glmUrl = "http://127.0.0.1:8000"

type chatglmPost struct {
	Prompt  string   `json:"prompt"`
	History []string `json:"history"`
}

type chatglmResp struct {
	Response string
	History  []string
	OK       bool
}

func (p *chatglmPost) post() *chatglmResp {
	postData, _ := json.Marshal(p)
	log.Debug("[ChatGLM2] 上报至ChatGLM: ", p)
	post := ihttp.New().WithUrl(glmUrl).
		WithHeader("Content-Type", "application/json").
		WithBody(postData)
	resp, err := post.Post().ToString()
	if resp == "Internal Server Error" { //初始化重试
		log.Info("[ChatGLM2] 初始化")
		time.Sleep(time.Second)
		new := ihttp.New().WithUrl(glmUrl).
			WithHeader("Content-Type", "application/json").
			WithBody(postData)
		resp, err = new.Post().ToString()
	}
	if err != nil {
		log.Error("[ChatGLM2] err: ", err)
		return &chatglmResp{
			Response: "[NothingBot] [ChatGLM2] [Error] ChatGLM2后端连接失败",
			OK:       false,
		}
	}
	g := gson.NewFrom(resp)
	history := []string{}
	if !g.Get("history").Nil() {
		for _, h := range g.Get("history").Arr() {
			for _, i := range h.Arr() {
				history = append(history, i.Str())
				// g.Get("history").Arr()[x].Arr()[x].Str()
			}
		}
	}
	return &chatglmResp{
		Response: g.Get("response").Str(),
		History:  history,
		OK:       true,
	}
}

func sendToChatGLM(input string) (output string, ok bool) {
	post := &chatglmPost{
		Prompt:  input,
		History: []string{},
	}
	resp := post.post()
	output = resp.Response
	ok = resp.OK
	return
}
