package main

import (
	"NothinBot/EasyBot"
	"bytes"
	"encoding/base64"
	"image/png"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	log "github.com/sirupsen/logrus"
)

func checkQRCode(ctx *EasyBot.CQMessage) {
	matches := ctx.RegexpFindAllStringSubmatch(`(?s)制作二维码\s*(.*)`)
	if len(matches) > 0 {
		s := trimOuterQuotes(matches[0][1])
		replyMsg, err := ctx.GetReplyedMsg()
		if replyMsg != nil && err == nil { //复述回复时无视内容
			s = trimOuterQuotes(replyMsg.RawMessage)
		}
		qr, _ := NewQRcode().With(s, 512)
		ctx.SendMsgReply(bot.Utils.Format.ImageBase64(qr.ToBase64()))
	}
}

type QRcode []byte

func NewQRcode() *QRcode {
	return &QRcode{}
}

func (qrc QRcode) With(content string, size int) (QRcode, error) {
	if size == 0 {
		size = 256
	}

	code, err := qr.Encode(content, qr.L, qr.Auto)
	if err != nil {
		log.Println("[QRcode] err1:", err)
		return qrc, err
	}
	code, err = barcode.Scale(code, size, size)
	if err != nil {
		log.Println("[QRcode] err2:", err)
		return qrc, err
	}
	buf := new(bytes.Buffer)
	err = png.Encode(buf, code)
	if err != nil {
		log.Println("[QRcode] err3:", err)
		return qrc, err
	}
	return buf.Bytes(), nil
}

func (qrc QRcode) ToBase64() string {
	return base64.StdEncoding.EncodeToString(qrc)
}
