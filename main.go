package main

import (
	_ "example/corpus"
	_ "example/hello"
	_ "example/matcher"
	_ "example/message"
	_ "example/repeat"
	_ "example/rule"

	log "github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"
)

func init() {
	log.SetFormatter(&easy.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LogFormat:       "[zero][%time%][%lvl%]: %msg% \n",
	})
	log.SetLevel(log.DebugLevel)
}

func main() {
	zero.RunAndBlock(zero.Config{
		NickName:      []string{"bot"},
		CommandPrefix: "/",
		SuperUsers:    []int64{982809597},
		Driver: []zero.Driver{
			driver.NewWebSocketClient("ws://127.0.0.1:9820/", ""),
		},
	}, nil)
}
