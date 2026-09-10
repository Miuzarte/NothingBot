package main

import (
	"time"

	"NothingBot_v4/logger"
	"github.com/Miuzarte/EasyOnebot/message"
)

// 本文件的日志 scope
var logPush = logger.New("PushMsg")

func PushMsg(msg any, users, groups []int) (err error) {
	for _, group := range groups {
		go func(group int) {
			var tries int
			var erro error
			for tries < 3 {
				_, erro = onebot.Call().Std.SendGroupMsg(group, msg)
				if erro == nil {
					break
				}
				tries++
				logPush.Error().
					Err(erro).
					Int("group", group).
					Msg("failed to send group msg")
				time.Sleep(time.Duration(tries*10) * time.Second)
			}
			if erro != nil {
				err = erro
			}
		}(group)
	}
	for _, user := range users {
		go func(user int) {
			var tries int
			var erro error
			for tries < 3 {
				_, erro = onebot.Call().Std.SendPrivateMsg(user, msg)
				if erro == nil {
					break
				}
				tries++
				logPush.Error().
					Err(erro).
					Int("user", user).
					Msg("failed to send private msg")
				time.Sleep(time.Duration(tries*10) * time.Second)
			}
			if erro != nil {
				err = erro
			}
		}(user)
	}
	return
}

func PushForwardMsg(msg message.SegmentArray, users, groups []int) (err error) {
	for _, group := range groups {
		go func(group int) {
			_, erro := onebot.Call().Nc.SendGroupForwardMsg(group, msg)
			if erro != nil {
				err = erro
				logPush.Error().
					Err(erro).
					Int("group", group).
					Msg("failed to send group forward msg")
			}
		}(group)
	}
	for _, user := range users {
		go func(user int) {
			_, erro := onebot.Call().Nc.SendPrivateForwardMsg(user, msg)
			if erro != nil {
				err = erro
				logPush.Error().
					Err(erro).
					Int("user", user).
					Msg("failed to send private forward msg")
			}
		}(user)
	}
	return
}
