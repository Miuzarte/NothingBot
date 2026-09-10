package main

import (
	"time"

	"github.com/Miuzarte/EasyOnebot/message"
)

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
				log.Errorf("[PushMsg] failed to send group msg to %d: %v", group, erro)
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
				log.Errorf("[PushMsg] failed to send private msg to %d: %v", user, erro)
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
			_, erro := onebot.Call().Lgr.SendGroupForwardMsg(group, msg)
			if erro != nil {
				err = erro
				log.Errorf("[PushMsg] failed to send group forward msg to %d: %v", group, erro)
			}
		}(group)
	}
	for _, user := range users {
		go func(user int) {
			_, erro := onebot.Call().Lgr.SendPrivateForwardMsg(user, msg)
			if erro != nil {
				err = erro
				log.Errorf("[PushMsg] failed to send private forward msg to %d: %v", user, erro)
			}
		}(user)
	}
	return
}
