package main

import (
	"fmt"

	"github.com/Miuzarte/EasyOnebot"
	"github.com/Miuzarte/EasyOnebot/message"
)

func ctxGetMsg(ctx *EasyOnebot.Ctx) (message.SegmentArray, error) {
	if ctx.ReplyId == 0 {
		return ctx.ParsedSegments, nil
	} else {
		replyMsg, err := ctx.GetReplyMsg()
		if err != nil {
			return nil, fmt.Errorf("获取消息失败：%w", err)
		}
		return replyMsg.Message, nil
	}
}

func ctxGetType(ctx *EasyOnebot.Ctx, typ ...message.SegType) (message.SegmentArray, error) {
	msg, err := ctxGetMsg(ctx)
	if err != nil {
		return nil, err
	}
	return msg.GetType(typ...), nil
}

func ctxGetImgSegs(ctx *EasyOnebot.Ctx) (message.SegmentArray, error) {
	imgSegs, err := ctxGetType(ctx, message.TYPE_IMAGE, message.TYPE_FILE)
	if err != nil {
		return nil, err
	}
	if len(imgSegs) == 0 {
		if ctx.ReplyId == 0 {
			return nil, fmt.Errorf("需要附带一张图片或回复一条图片/文件消息")
		} else {
			return nil, fmt.Errorf("回复的消息中不包含图片/文件")
		}
	}
	return imgSegs, nil
}

func ctxGetUrls(ctx *EasyOnebot.Ctx) (mediaUrls []string, segType message.SegType, err error) {
	mediaSegs, err := ctxGetType(ctx, message.TYPE_IMAGE, message.TYPE_VIDEO, message.TYPE_RECORD, message.TYPE_FILE)
	if err != nil {
		return nil, "", err
	}
	if len(mediaSegs) == 0 {
		if ctx.ReplyId == 0 {
			return nil, "", fmt.Errorf("需要附带图片或回复一条包含图片/视频/语音/文件的消息")
		} else {
			return nil, "", fmt.Errorf("回复的消息中不包含图片/视频/语音/文件内容")
		}
	}

	mediaUrls = make([]string, 0, len(mediaSegs))
	for _, mediaSeg := range mediaSegs {
		mediaUrl, ok := mediaSeg.Data["url"].(string)
		if ok {
			mediaUrls = append(mediaUrls, mediaUrl)
		}
	}
	if len(mediaUrls) == 0 {
		return nil, mediaSegs[0].Type, fmt.Errorf("无法获取图片/视频/语音/文件的URL，尝试重新发送后再次请求")
	}
	return mediaUrls, mediaSegs[0].Type, nil
}
