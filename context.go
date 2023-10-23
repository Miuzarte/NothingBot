package main

import (
	"NothinBot/EasyBot"
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

type botContext struct {
	timer         *time.Timer
	callbackStart func()
	callbackReach func(msg *EasyBot.CQMessage) (isDone bool)
	callbackEnd   func()
}

var (
	msgChans map[int64]chan *EasyBot.CQMessage
)

func newContext(timeout time.Duration) *botContext {
	timer := time.NewTimer(timeout)
	return &botContext{
		timer: timer,
	}
}

// 创建上下文监听, reach 时返回 true 则结束, 或者 WithCancel 后自行调用 CancelFunc
func newMsgContext(ctx context.Context, callbackStart func(), callbackReach func(msg *EasyBot.CQMessage) (isDone bool), callbackEnd func()) {
	now := time.Now().Unix()                 // 时间戳, 作为ID
	msgChan := make(chan *EasyBot.CQMessage) // 接收消息用
	msgChans[now] = msgChan
	go func() {
		defer func() {
			close(msgChan)
			msgChans[now] = nil
		}()

		log.Debug("[context] 创建了一条新的上下文: ", now)
		if callbackStart != nil {
			callbackStart()
		}

		for {
			select {
			case msg := <-msgChan:
				if callbackReach != nil {
					if isDone := callbackReach(msg); isDone {
						return
					}
				}

			case <-ctx.Done():
				log.Debug("[context] 上下文结束返回")
				if callbackEnd != nil {
					callbackEnd()
				}
				return

			}
		}
	}()
	<-ctx.Done()
}

// 将消息放入管道待取
func checkContextPutIn(ctx *EasyBot.CQMessage) {
	for _, msgChan := range msgChans {
		msgChan <- ctx
	}
}
