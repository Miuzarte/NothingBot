package main

import (
	"NothinBot/EasyBot"
	"fmt"
	"sync"
	"time"
)

const (
	_  = iota
	B  = iota
	KB = B * 1024
	MB = KB * 1024
	GB = MB * 1024
	TB = GB * 1024
)

type groupFileParse struct {
	files []*EasyBot.CQNoticeGroupUpload

	isWating bool
	timer    *time.Timer
	again    chan struct{}
}

var (
	gfpMutex sync.Mutex
	gfpTable map[int]map[int]*groupFileParse //group:user:gfp
)

func GroupUploadParse(gu *EasyBot.CQNoticeGroupUpload) {
	gfpMutex.Lock()

	if gfpTable == nil {
		gfpTable = make(map[int]map[int]*groupFileParse)
	}
	if gfpTable[gu.GroupID] == nil {
		gfpTable[gu.GroupID] = make(map[int]*groupFileParse)
	}
	if gfpTable[gu.GroupID][gu.UserID] == nil {
		gfpTable[gu.GroupID][gu.UserID] = &groupFileParse{}
	}

	gfp := gfpTable[gu.GroupID][gu.UserID]
	gfpMutex.Unlock()

	gfp.files = append(gfp.files, gu)

	waitToSendGroupUpload(gu.GroupID, gu.UserID)
}

func waitToSendGroupUpload(groupId int, userId int) {
	gfpMutex.Lock()
	gfp := gfpTable[groupId][userId]
	gfpMutex.Unlock()
	if gfp.isWating {
		gfp.again <- struct{}{}
		return
	}

	gfp.isWating = true
	gfp.timer = time.NewTimer(time.Second * 3)
	gfp.again = make(chan struct{}, 1)
	defer func() {
		gfpMutex.Lock()
		gfpTable[groupId][userId] = nil
		gfpMutex.Unlock()
	}()

	for {
		select {
		case <-gfp.timer.C:
			go func() {
				userName := bot.GetCardName(groupId, userId)
				forwardMsg := EasyBot.NewForwardMsg(EasyBot.NewCustomForwardNode(
					fmt.Sprintf("%s（%d）", userName, userId), bot.GetSelfID(),
					fmt.Sprintf("上传了 %d 个文件\nNothingbot_FileParse", len(gfp.files)),
					0, 0))
				for _, file := range gfp.files {
					forwardMsg = EasyBot.AppendForwardMsg(forwardMsg, EasyBot.NewCustomForwardNode(
						file.File.Name+"（"+formatFileSize(file.File.Size)+"）", file.UserID,
						file.File.Url,
						0, 0))
				}
				bot.SendGroupForwardMsg(groupId, forwardMsg)
			}()
			return
		case _, ok := <-gfp.again:
			if !ok {
				return
			}
			gfp.timer.Reset(time.Second * 3)
			continue
		}
	}
}

func formatFileSize(size int) string {
	switch {
	case size > GB:
		return fmt.Sprintf("%sGB", formatNumber(float64(size)/float64(GB), 2, true))
	case size > MB:
		return fmt.Sprintf("%sMB", formatNumber(float64(size)/float64(MB), 2, true))
	case size > KB:
		return fmt.Sprintf("%sKB", formatNumber(float64(size)/float64(KB), 2, true))
	default:
		return fmt.Sprintf("%dB", size)
	}
}
