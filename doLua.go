package main

import (
	"NothinBot/EasyBot"
	"bytes"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	lua "github.com/yuin/gopher-lua"
)

var (
	// Bot用的VM, 不需要重定向print
	globalLuaVM = lua.NewState(lua.Options{
		MinimizeStackMemory: true,
	})
)

var (
	doLuaEnable  = false
	doLuaTimeout = time.Second * 10
	doLuaVM      *lua.LState
	printBuffer  = new(bytes.Buffer)
)

func resetDoLuaVM() {
	log.Debug("[doLua] reset doLuaVM")
	printBuffer.Reset()
	if doLuaVM != nil {
		doLuaVM.Close()
	}
	doLuaVM = lua.NewState(lua.Options{
		MinimizeStackMemory: true,
	})
	// alias
	doLuaVM.SetGlobal("stdPrint", doLuaVM.GetGlobal("print"))
	// print重定向
	doLuaVM.SetGlobal("print", doLuaVM.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		for i := 1; i <= top; i++ {
			if i > 1 {
				printBuffer.WriteString("\t")
			}
			printBuffer.WriteString(L.Get(i).String())
		}
		// printBuffer.WriteString("\n")
		return 0
	}))
	// add := func(a, b int) int { return a + b }
	// doLuaVM.Register("add", add)
}

func doLuaWithTimeout(l *lua.LState, source string, timeout time.Duration) (result string, err error) {
	resetDoLuaVM()
	resultChan := make(chan string)
	errChan := make(chan error)
	go func() {
		err = doLuaVM.DoString(source)
		if err == nil {
			resultChan <- printBuffer.String()
		} else {
			errChan <- err
		}
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err = <-errChan:
		return "", err
	case <-time.After(timeout):
		log.Warn("[doLua] excute timeout (", timeout, ")")
		l.Close()
		return "", errors.New("excute timeout")
	}
}

func checkDoLua(ctx *EasyBot.CQMessage) {
	//开关控制
	matches := ctx.RegexpFindAllStringSubmatch(`(开启|启用|关闭|禁用)do[Ll]ua`)
	if len(matches) > 0 && ctx.IsPrivateSU() {
		switch matches[0][1] {
		case "开启", "启用":
			doLuaEnable = true
			ctx.SendMsg("doLua已启用")
		case "关闭", "禁用":
			doLuaEnable = false
			ctx.SendMsg("doLua已禁用")
		}
		return
	}
	if !doLuaEnable {
		return
	}

	symbolMatches := ctx.RegexpFindAllStringSubmatch("do[Ll]ua\n")
	if len(symbolMatches) > 0 {
		luaScript := ctx.RegexpReplaceAll("do[Ll]ua\n", "")
		log.Debug("execute lua:\n", luaScript)
		result, err := doLuaWithTimeout(doLuaVM, luaScript, doLuaTimeout)
		if err == nil {
			ctx.SendMsgReply(result)
		} else {
			ctx.SendMsgReply("执行出现错误: ", err.Error())
		}
	}
}
