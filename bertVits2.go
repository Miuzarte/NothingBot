package main

import (
	"NothinBot/EasyBot"
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

type bertVitsPost struct {
	Command string  `json:"command"`
	Text    string  `json:"text"`
	Speaker string  `json:"speaker"`
	Lang    string  `json:"language"`
	SDP     float32 `json:"sdp_ratio,omitempty"`
	NS      float32 `json:"noise_scale,omitempty"`
	NSW     float32 `json:"noise_scale_w,omitempty"`
	LS      float32 `json:"length_scale,omitempty"`
}

type bertVitsResp struct {
	Code   int    `json:"code"`
	Output string `json:"output"` // 音频base64, 直接发
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
		return nil, errors.New("BertVITS2后端未运行")
	}
	log.Debug("[BertVITS2] resp: ", resp)
	r := &bertVitsResp{}
	json.Unmarshal([]byte(resp), r)
	return r, nil
}

func bertVits2TTS(ctx *EasyBot.CQMessage, text, speaker, lang string) (outputB64 string, err error) {
	if text == "" {
		return "", errors.New("empty text")
	}
	if speaker == "" {
		speaker = "suijiSUI"
	}
	if lang == "" {
		lang = "ZH"
	}
	p := &bertVitsPost{
		Text:    text,
		Speaker: speaker,
		Lang:    lang,
	}
	resp, err := p.post()
	var state string
	switch {
	case err != nil:
		state = "后端未运行"
	case resp.Error != "":
		state = "Failed: " + resp.Error
		err = errors.New(state)
	case resp.Code != 0:
		state = "Code 0 failed"
		err = errors.New(state)
	case resp.Output == "":
		state = "Empty output"
		err = errors.New(state)
	default:
		state = "Success"
	}
	p.recordHist(ctx.GroupID, ctx.UserID, ctx.GetCardOrNickname(), state)
	if resp != nil {
		return resp.Output, err
	}
	return "", err
}

// 记录历史
func (p *bertVitsPost) recordHist(groupId, userId int, userName, state string) {
	nowTime := time.Now().Format(timeLayout.L24)
	content := toCsv(nowTime, groupId, userId, userName, state, p.Speaker, p.Lang, p.Text)
	if err := appendToFile(
		"tts_history.csv", content,
	); err != nil {
		log.Error("[BertVITS2] 历史写入失败")
		bot.Log2SU.Error("[BertVITS2] 历史记录写入失败\n", content)
	}
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

var (
	langMap = map[string]string{
		"中文": "ZH",
		"汉语": "ZH",
		"ZH": "ZH",
		"日文": "JP",
		"日语": "JP",
		"JP": "JP",
	}
)

func checkBertVITS2(ctx *EasyBot.CQMessage) {
	//后端控制
	matches := ctx.RegexpFindAllStringSubmatch(`^(unload|refresh|exit|卸载|清理|退出).*(模型|model)$`)
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
	matches = ctx.RegexpFindAllStringSubmatch(`(?s)让岁己(用(中文|汉语|ZH|日文|日语|JP))?(说|复述)\s*(.*)`)
	if len(matches) > 0 {
		// isInWhite := func() (is bool) {
		// 	var v *viper.Viper
		// 	for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.group")); i++ { //群聊白名单
		// 		if ctx.GroupID == v.GetInt(fmt.Sprint("bertVits.whiteList.group.", i)) {
		// 			return true
		// 		}
		// 	}
		// 	for i := 0; i < len(v.GetStringSlice("bertVits.whiteList.private")); i++ { //私聊白名单
		// 		if ctx.UserID == v.GetInt(fmt.Sprint("bertVits.whiteList.private.", i)) {
		// 			return true
		// 		}
		// 	}
		// 	return false
		// }()
		// if !ctx.IsSU() && !isInWhite {
		// 	ctx.SendMsg("[BertVITS2] Permission Denied")
		// 	return
		// }
		lang := "ZH"
		if l, ok := langMap[matches[0][2]]; ok {
			lang = l
		}
		text := trimOuterQuotes(matches[0][4])
		replyMsg, err := ctx.GetReplyedMsg()
		if replyMsg != nil && err == nil { //复述回复时无视内容
			text = trimOuterQuotes(replyMsg.RawMessage)
		}
		sendVitsMsg(ctx, text, lang)
	}
}

// 发送vits消息
func sendVitsMsg(ctx *EasyBot.CQMessage, text, lang string) {
	log.Debug("[BertVITS2] Vits text: ", text)
	if len(strings.TrimSpace(text)) == 0 {
		ctx.SendMsgReply("[BertVITS2] 文本输入不可为空！")
		return
	}
	outputB64, err := bertVits2TTS(ctx, text, "", lang)
	if err != nil {
		log.Error("[BertVITS2] 发生错误: ", err)
		ctx.SendMsgReply("[BertVITS2] 发生错误：", err.Error(), "\n可能是某人在臭打游戏没有运行后端捏")
		return
	}
	ctx.SendMsg(bot.Utils.Format.VocalBase64(outputB64, true))
}

// 去除最外层一对互相匹配的引号
func trimOuterQuotes(s string) string {
	runeArr := []rune(s)
	if len(runeArr) < 2 {
		return s
	}

	left := runeArr[0]
	right := runeArr[(len(runeArr) - 1)]

	if (left == '\'' && right == '\'') ||
		(left == '`' && right == '`') ||
		(left == '"' && right == '"') ||
		(left == '“' && right == '”') ||
		(left == '”' && right == '“') {
		runeArr = runeArr[1 : len(runeArr)-1]
	}
	return string(runeArr)
}
