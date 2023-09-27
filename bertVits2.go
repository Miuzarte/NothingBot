package main

import (
	"NothinBot/EasyBot"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type bertVitsPost struct {
	Command string  `json:"command"`
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
		return nil, errors.New("[BertVITS2] BertVITS2后端未运行")
	}
	log.Debug("[BertVITS2] resp: ", resp)
	r := &bertVitsResp{}
	json.Unmarshal([]byte(resp), r)
	return r, nil
}

func bertVits2TTS(intput string) (output string, err error) {
	speaker := "suijiSUI"
	p := &bertVitsPost{
		Text:    intput,
		Speaker: speaker,
	}
	nowTime := time.Now().Format(timeLayout.L24)
	recordHist := func(stat string) { // 记录历史
		if err = appendToFile("./tts_history.txt",
			fmt.Sprintf("%s  (%s)\n%s: %s\n\n",
				nowTime, stat,
				speaker, intput)); err != nil {
			log.Warn("[BertVITS2] 历史写入失败")
		}
	}
	resp, err := p.post()
	if err != nil {
		s := "后端未运行"
		recordHist("Failed: " + s)
		return "", errors.New(s)
	}
	if resp.Error != "" {
		recordHist("Failed: " + resp.Error)
		return "", errors.New(resp.Error)
	}
	if resp.Code != 0 {
		s := "TTS FAILED"
		recordHist("Failed: " + s)
		return "", errors.New(s)
	}
	if resp.Output == "" {
		s := "OUTPUT IS EMPTY"
		recordHist("Failed: " + s)
		return "", errors.New(s)
	}

	recordHist("Success")
	return resp.Output, nil
}

// 追加文本
func appendToFile(filePath, content string) error {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	_, err = writer.WriteString(content)
	if err != nil {
		return err
	}
	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}

func checkBertVITS2(ctx *EasyBot.CQMessage) {
	//后端控制
	matches := ctx.RegexpMustCompile(`^(unload|refresh|exit|卸载|清理|退出).*(模型|model)$`)
	if len(matches) > 0 && ctx.IsPrivateSU() {
		p := &bertVitsPost{}
		switch matches[0][1] {
		case "unload", "refresh", "卸载", "清理":
			p.Command = "/refresh"
		case "exit", "退出":
			p.Command = "/exit"
		}
		_, err := p.post()
		if err == nil {
			ctx.SendMsg("已执行")
		} else {
			ctx.SendMsg(err)
		}
		return
	}
	matches = ctx.RegexpMustCompile(`(?s)让岁己(说|复述)\s*(.*)`)
	if len(matches) > 0 {
		isInWhite := func() (is bool) {

			var v *viper.Viper

			for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.group")); i++ { //群聊黑名单
				if ctx.GroupID == v.GetInt(fmt.Sprint("bertVits.whiteList.group.", i)) {
					return true
				}
			}
			for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.private")); i++ { //私聊黑名单
				if ctx.UserID == v.GetInt(fmt.Sprint("bertVits.whiteList.private.", i)) {
					return true
				}
			}
			return false
		}()
		if !ctx.IsSU() && !isInWhite {
			ctx.SendMsg("[BertVITS2] Permission Denied")
			return
		}
		text := trimOuterQuotes(matches[0][2])
		replyMsg, err := ctx.GetReplyedMsg()
		if replyMsg != nil && err == nil { //复述回复时无视内容
			text = trimOuterQuotes(replyMsg.RawMessage)
		}
		sendVitsMsg(ctx, text)
	}
}

func sendVitsMsg(ctx *EasyBot.CQMessage, text string) {
	log.Debug("text: ", text)
	if len(strings.TrimSpace(text)) == 0 {
		ctx.SendMsgReply("[BertVITS2] 文本输入不可为空！")
		return
	}
	output, err := bertVits2TTS(text)
	if err != nil {
		log.Error("[BertVITS2] 出现错误(1): ", err)
		ctx.SendMsgReply("[BertVITS2] 出现错误(1)：", err.Error())
		return
	}
	wavData, err := os.ReadFile(output)
	if err != nil {
		log.Error("[BertVITS2] 出现错误(2): ", err)
		ctx.SendMsgReply("[BertVITS2] 出现错误(2)：", err.Error())
		return
	}
	ctx.SendMsg(bot.Utils.Format.Vocal(wavData, false))
}

// 去除最外层一对互相匹配的引号
func trimOuterQuotes(s string) string {
	r := []rune(s)
	if len(r) < 2 {
		return s
	}

	f := r[0]
	l := r[(len(r) - 1)]

	if (f == '\'' && l == '\'') ||
		(f == '`' && l == '`') ||
		(f == '"' && l == '"') ||
		(f == '“' && l == '”') ||
		(f == '”' && l == '“') {
		r = r[1 : len(r)-1]
	}
	return string(r)
}
