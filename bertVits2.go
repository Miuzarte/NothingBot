package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

type bertVitsPost struct {
	Text    string  `json:"text"`
	Speaker string  `json:"speaker"`
	SDP     float32 `json:"sdp_ratio,omitempty"`
	NS      float32 `json:"noise_scale,omitempty"`
	NSW     float32 `json:"noise_scale_w,omitempty"`
	LS      float32 `json:"length_scale,omitempty"`
}

type bertVitsResp struct {
	Code   int    `json:"code"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

func (p *bertVitsPost) post() (*bertVitsResp, error) {
	postData, _ := json.Marshal(p)
	log.Debug("[BertVITS2] post: ", string(postData))
	resp, err := ihttp.New().WithUrl("http://127.0.0.1:9876").
		WithHeader("Content-Type", "application/json").
		WithBody(postData).
		Post().ToString()
	if err != nil {
		log.Error("[BertVITS2] ihttp error: ", err)
		return nil, err
	}
	log.Debug("[BertVITS2] resp: ", resp)
	r := &bertVitsResp{}
	json.Unmarshal([]byte(resp), r)
	return r, nil
}

func bertVits2TTS(intput string) (output string, err error) {
	post := &bertVitsPost{
		Text:    intput,
		Speaker: "suijiSUI",
	}
	resp, err := post.post()
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
	}
	if resp.Code != 0 {
		return "", errors.New("TTS FAILED")
	}
	if resp.Output == "" {
		return "", errors.New("OUTPUT IS EMPTY")
	}
	return resp.Output, nil
}

func wav2amr(wav []byte) (amr []byte, err error) {
	cmd := exec.Command("ffmpeg", "-f", "wav", "-i", "pipe:0", "-ar", "8000", "-ac", "1", "-f", "amr", "pipe:1")
	cmd.Stdin = strings.NewReader(string(wav))
	amr, err = cmd.Output()
	if err != nil {
		log.Error("[w2a] FFmpeg转换失败: ", err)
		return []byte{}, err
	}
	return
}

func checkBertVITS2(ctx gocqMessage) {
	match := ctx.regexpMustCompile(`(?s)(\[CQ:reply,id=(-?.*)].*)?让岁己(说|复述)\s*(.*)`)
	if len(match) > 0 {
		isInWhite := func() (is bool) {
			for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.group")); i++ { //群聊黑名单
				if ctx.group_id == v.GetInt(fmt.Sprint("bertVits.whiteList.group.", i)) {
					return true
				}
			}
			for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.private")); i++ { //私聊黑名单
				if ctx.user_id == v.GetInt(fmt.Sprint("bertVits.whiteList.private.", i)) {
					return true
				}
			}
			return false
		}()
		if !ctx.isSU() && !isInWhite {
			ctx.sendMsg("[BertVITS2] 岁己TTS需要主人权限捏")
			return
		}
		text := match[0][4]
		reply := match[0][2]
		replyId, _ := strconv.Atoi(reply)
		if reply != "" { //复述回复时无视内容
			text = trimOuterQuotes(ctx.getMsgFromId(replyId).message)
		}
		log.Debug("text: ", text)
		log.Debug("reply: ", reply)
		log.Debug("replyId: ", replyId)
		if len(strings.TrimSpace(text)) == 0 {
			ctx.sendMsgReply("[BertVITS2] 文本输入不可为空！")
			return
		}
		out, err := bertVits2TTS(text)
		if err != nil {
			ctx.sendMsgReply("[BertVITS2] 出现错误：", err.Error())
			return
		}
		wav, err := os.ReadFile(out)
		if err != nil {
			ctx.sendMsgReply("[BertVITS2] 出现错误：", err.Error())
			return
		}
		amr, err := wav2amr(wav)
		if err != nil {
			ctx.sendMsgReply("[BertVITS2] 出现错误：", err.Error())
			return
		}
		ctx.sendMsg("[CQ:record,file=base64://" + base64.StdEncoding.EncodeToString(amr) + "]")
	}
}

// 去除最外层一对互相匹配的引号
func trimOuterQuotes(s string) string {
	r := []rune(s)
	if len(r) < 2 {
		return s
	}

	f := r[0]
	l := r[(len(s) - 1)]

	if (f == '\'' && l == '\'') ||
		(f == '`' && l == '`') ||
		(f == '"' && l == '"') ||
		(f == '“' && l == '”') ||
		(f == '”' && l == '“') {
		r = r[1 : len(s)-1]
	}
	return string(r)
}
