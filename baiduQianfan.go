package main

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

const (
	tokenUrl   = "https://aip.baidubce.com/oauth/2.0/token"
	qianfanUrl = "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop/chat/" //+model
)

var (
	selectedModelStr string
	selectedModel    string
)
var qianfanModels = struct {
	ERNIE_Bot       string
	ERNIE_Bot_turbo string
	BLOOMZ_7B       string
	Llama_2_7b      string
	Llama_2_13b     string
	Llama_2_70b     string
}{
	ERNIE_Bot:       "completions",
	ERNIE_Bot_turbo: "eb-instant",
	BLOOMZ_7B:       "bloomz_7b1",
	Llama_2_7b:      "llama_2_7b",
	Llama_2_13b:     "llama_2_13b",
	Llama_2_70b:     "llama_2_70b",
}

type clientCredentials struct {
	RefreshToken  string `json:"refresh_token"`
	ExpiresIn     int64  `json:"expires_in"`
	ExpiredTime   int64  `json:"expired_time"`
	SessionKey    string `json:"session_key"`
	AccessToken   string `json:"access_token"`
	Scope         string `json:"scope"`
	SessionSecret string `json:"session_secret"`
}

type wenxinPost struct {
	Messages []wenxinMessages
}

type wenxinMessages struct {
	Role    string
	Content string
}

type wenxinResp struct {
	ID               string `json:"id"`
	Object           string `json:"object"`
	Created          int64  `json:"created"`
	Result           string `json:"result"`
	IsTruncated      bool   `json:"is_truncated"`
	NeedClearHistory bool   `json:"need_clear_history"`
	Usage            struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

var (
	apiKey    string
	secretKey string
	cc        *clientCredentials
)

// 读取key, token
func initQianfan() {
	cc = &clientCredentials{}
	apiKey = v.GetString("qianfan.keys.api")
	secretKey = v.GetString("qianfan.keys.secret")
	log.Trace("[qianfan] apiKey: ", apiKey)
	log.Trace("[qianfan] secretKey: ", secretKey)
	ccLocal, err := os.ReadFile("./baidu/client_credentials.json")
	if err == nil {
		err = json.Unmarshal(ccLocal, cc)
		if err != nil {
			log.Warn("[qianfan] cc Unmarshal err: ", err.Error())
		}
		log.Trace("[qianfan] cc: ", cc)
	} else {
		log.Warn("[qianfan] cc read err: ", err.Error()) //读不到
	}
	if ok, err := checkCC(); !ok {
		log.Error(err)
	}
}

// 检查凭据是否需要更新
func checkCC() (ok bool, err error) {
	expired := time.Now().Unix()+(60*60*24) > cc.ExpiredTime
	if expired { //不足一天
		log.Debug("expired: ", expired)
		ccNew, err := cc.updateCredentials()
		if err == nil {
			cc = ccNew
		} else {
			return false, errors.New("FAILED TO UPDATE CREDENTIALS")
		}
		ccByte, err := json.Marshal(cc)
		if err != nil {
			log.Warn("[qianfan] cc Marshal err: ", err.Error())
		}
		checkDir("./baidu/")
		os.WriteFile("./baidu/client_credentials.json", ccByte, 0664)
	}
	return true, nil
}

// 更新凭据
func (cc *clientCredentials) updateCredentials() (*clientCredentials, error) {
	resp, err := ihttp.New().WithUrl(tokenUrl).
		WithHeader("Content-Type", "application/json").
		WithAddQuerys(map[string]any{
			"grant_type":    "client_credentials",
			"client_id":     apiKey,
			"client_secret": secretKey,
		}).Post().ToString()
	if err != nil {
		log.Error("[qianfan] cc post err: ", err.Error())
		return nil, err
	}
	log.Debug("[qianfan] get cc: ", resp)
	cc = &clientCredentials{}
	err = json.Unmarshal([]byte(resp), cc)
	if err != nil {
		log.Warn("[qianfan] cc Unmarshal err: ", err.Error())
	}
	if cc.ExpiresIn == 0 || cc.AccessToken == "" {
		return nil, errors.New("failed to refresh token")
	}
	cc.ExpiredTime = cc.ExpiresIn + time.Now().Unix()
	return cc, nil
}

func (p *wenxinPost) post() (*wenxinResp, error) {
	if ok, err := checkCC(); !ok {
		log.Error(err)
		return nil, err
	}
	postData, _ := json.Marshal(p)
	log.Debug("[qianfan] post: ", *p)
	resp, err := ihttp.New().WithUrl(qianfanUrl+selectedModel).
		WithHeader("Content-Type", "application/json").
		WithAddQuery("access_token", cc.AccessToken).
		WithBody(postData).
		Post().ToString()
	if err != nil {
		log.Error("[qianfan] err: ", err.Error())
		return nil, err
	}
	log.Debug("[qianfan] resp: ", resp)
	if g := gson.NewFrom(resp); g.Get("error_code").Int() != 0 {
		return nil, errors.New(resp)
	}
	r := &wenxinResp{}
	json.Unmarshal([]byte(resp), r)
	return r, nil
}

func sendToWenxinSingle(input string) (output string, err error) {
	messages := wenxinMessages{
		Role:    "user",
		Content: input,
	}
	post := &wenxinPost{}
	post.Messages = append(post.Messages, messages)
	resp, err := post.post()
	if err != nil {
		return "", err
	}
	if resp.Result != "" {
		output = resp.Result
	} else {
		output = "<因未知原因接口回复为空>"
	}
	return
}
