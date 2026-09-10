package main

import (
	"testing"
	"time"

	"github.com/Miuzarte/EasyOnebot/message"
)

// {
//     "user_id": 0,
//     "messages": [
//         {
//             "type": "node",
//             "data": {
//                 "user_id": "string",
//                 "nickname": "string",
//                 "content": [
//                     {
//                         "type": "at",
//                         "data": {
//                             "user_id": "string",
//                             "name": "string"
//                         }
//                     }
//                 ]
//             }
//         }
//     ]
// }

func TestForwardMsg(t *testing.T) {
	InitBot()
	onebot.Run()
	onebot.Call().Lgr.SendGroupForwardMsg(
		612645549, message.SegmentArray{
			{
				Type: "node",
				Data: map[string]any{
					"user_id":  "982809597",
					"nickname": "謬紗特",
					"content":  "233",
				},
			},
			{
				Type: "node",
				Data: map[string]any{
					"user_id":  "2393827810",
					"nickname": "也是謬紗特",
					"content":  "233333",
				},
			},
			{
				Type: "node",
				Data: map[string]any{
					"user_id":  "0",
					"nickname": "",
					"content":  "猜猜这是谁",
				},
			},
		},
	)
	<-time.After(3 * time.Second)
	onebot.Stop()
}
