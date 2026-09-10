package main

import (
	"testing"
	"time"

	"github.com/Miuzarte/EasyOnebot/message"
)

func TestMarkdownSeg(t *testing.T) {
	const MARKDOWN = `# This is a Markdown Title`

	runBot()

	// resp, err := onebot.Call().Std.SendGroupMsg(706579049, message.Markdown(MARKDOWN))
	resp, err := onebot.Call().Std.SendPrivateMsg(982809597, message.Markdown(MARKDOWN))
	if err != nil {
		t.Error(err)
	} else {
		t.Log(resp.MessageId)
	}

	<-time.After(time.Second * 10)
	onebot.Stop()
}
