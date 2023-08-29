package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

var glmUrl = "http://127.0.0.1:8000"

type chatglmPost struct {
	Prompt  string   `json:"prompt"`
	History []string `json:"history"`
}

type chatglmResp struct {
	Response string     `json:"response"`
	History  [][]string `json:"history"`
}

func (p *chatglmPost) post() (*chatglmResp, error) {
	postData, _ := json.Marshal(p)
	log.Debug("[ChatGLM2] post: ", *p)
	ipost := ihttp.New().WithUrl(glmUrl).
		WithHeader("Content-Type", "application/json").
		WithBody(postData)
	resp, err := ipost.Post().ToString()
	if resp == "Internal Server Error" { //初始化重试
		log.Info("[ChatGLM2] 初始化")
		time.Sleep(time.Second)
		newipost := ihttp.New().WithUrl(glmUrl).
			WithHeader("Content-Type", "application/json").
			WithBody(postData)
		resp, err = newipost.Post().ToString()
	}
	if err != nil {
		log.Error("[ChatGLM2] post err: ", err)
		return nil, errors.New("ChatGLM2后端连接失败  " + err.Error())
	}
	log.Debug("[ChatGLM2] resp: ", resp)
	r := &chatglmResp{}
	err = json.Unmarshal([]byte(resp), r)
	if err != nil {
		log.Error("[ChatGLM2] Unmarshal err: ", err)
	}
	return r, nil
}

func sendToChatGLMSingle(input string) (output string, err error) {
	post := &chatglmPost{
		Prompt:  input,
		History: []string{},
	}
	resp, err := post.post()
	if err != nil {
		return "", err
	}
	output = resp.Response
	return
}
