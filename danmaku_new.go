package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

type danmaku struct {
	conn           *websocket.Conn
	connected      bool
	uid            int
	roomid         int
	onDanmakuRecv_ func(recv gson.JSON)
}

func NewDanmaku(uid int, roomid int) (d danmaku) {
	d = danmaku{
		uid:    uid,
		roomid: roomid,
	}
	return d
}

func (d danmaku) OnDanmakuRecv(f func(recv gson.JSON)) danmaku {
	d.onDanmakuRecv_ = f
	return d
}

func (d danmaku) Start() {
	go func() {
		d.connect()
		d.recvLoop()
	}()
}

func (d danmaku) Stop() {
	d.conn.Close()
}

func (d danmaku) connect() {
	roomInfo := GetRoomInfo(d.roomid)
	if roomInfo == nil {
		log.Error("[danmaku] room info is invalid.")
	}
	host := []string{"broadcastlv.chat.bilibili.com"}
	for _, h := range roomInfo.Get("data.host_list").([]any) {
		host = append(host, h.(map[string]any)["host"].(string))
	}
	token := roomInfo.GetString("data.token")
	if token == "" {
		log.Error("[danmaku] token is invalid.")
	}
	reqHeader := &http.Header{}
	reqHeader.Set("User-Agent", iheaders["User-Agent"])
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/sub", host[0]), *reqHeader)
	if err != nil {
		log.Error("[danmaku] failed to establish websocket connection: ", err)
	}
	err = SendEnterPacket(conn, d.uid, d.roomid, token)
	if err != nil {
		log.Error("[danmaku] can not enter room: ", err)
	}
	d.connected = true
	d.conn = conn
}

func (d danmaku) recvLoop() {
	var pktJson gson.JSON
	go d.HeartBeatLoop()
	for {
		msgType, data, err := d.conn.ReadMessage()
		if err == io.EOF {
			log.Error("[danmaku] disconnected: ", err)
			d.connected = false
			break
		}
		if err != nil {
			log.Error("[danmaku] get error message: ", err)
			d.connected = false
			break
		}
		if msgType != websocket.BinaryMessage {
			log.Error("[danmaku] packet not binary.")
			time.Sleep(time.Second * 5)
			continue
		}
		for _, pkt := range NewPacketFromBytes(data).Parse() {
			pktJson = gson.NewFrom(string(pkt.Body))
			log.Trace("[danmaku] 接收数据包: ", string(pkt.Body))
			switch {
			case !pktJson.Get("cmd").Nil():
				cmd := pktJson.Get("cmd").Str()
				switch cmd {
				case "AREA_RANK_CHANGED":
				case "COMBO_SEND":
				case "COMBO_END":
				case "COMMON_NOTICE_DANMAKU":
				case "DANMU_AGGREGATION":
				case "DANMU_MSG":
				case "ENTRY_EFFECT":
				case "ENTRY_EFFECT_MUST_RECEIVE":
				case "GUARD_HONOR_THOUSAND":
				case "INTERACT_WORD":
				case "LIKE_INFO_V3_CLICK":
				case "LIKE_INFO_V3_UPDATE":
				//case "LIVE":
				case "NOTICE_MSG":
				case "ONLINE_RANK_COUNT":
				case "ONLINE_RANK_V2":
				case "ONLINE_RANK_TOP3":
				case "PK_BATTLE_END":
				case "PK_BATTLE_FINAL_PROCESS":
				case "PK_BATTLE_PRE":
				case "PK_BATTLE_PRE_NEW":
				case "PK_BATTLE_START":
				case "PK_BATTLE_START_NEW":
				case "PK_BATTLE_PROCESS":
				case "PK_BATTLE_PROCESS_NEW":
				case "PK_BATTLE_SETTLE":
				case "PK_BATTLE_SETTLE_USER":
				case "PK_BATTLE_SETTLE_V2":
				case "POPULAR_RANK_CHANGED":
				case "POPULARITY_RED_POCKET_WINNER_LIST":
				//case "PREPARING":
				//case "ROOM_CHANGE":
				case "ROOM_REAL_TIME_MESSAGE_UPDATE":
				case "SEND_GIFT":
				case "SPREAD_SHOW_FEET_V2":
				case "STOP_LIVE_ROOM_LIST":
				case "SUPER_CHAT_MESSAGE":
				case "WATCHED_CHANGE":
				case "WIDGET_BANNER":
				case "WIDGET_GIFT_STAR_PROCESS":
				default:
					log.Debug("[danmaku] 直播间 ", d.roomid, " 接收数据: {\"cmd\": ", cmd, "}")
				}
			case !pktJson.Get("code").Nil():
				code := pktJson.Get("code").Str()
				log.Info("[danmaku] 直播间 ", d.roomid, " 接收数据: {\"code\": ", code, "}")
			default:
				if len(string(pkt.Body)) > 4 { //过滤奇怪的数据包导致控制台发声
					log.Debug("[danmaku] 直播间 ", d.roomid, " 原始数据: ", string(pkt.Body))
				}
			}
			d.onDanmakuRecv_(pktJson)
		}
	}
}

func (d danmaku) HeartBeatLoop() {
	pkt := NewPacket(Plain, HeartBeat, nil).Build()
	boolChange := make(chan struct{})
	go func() {
		for {
			time.Sleep(time.Second)
			if !d.connected {
				boolChange <- struct{}{}
				break
			}
		}
	}()
	for {
		select {
		case <-time.After(time.Second * 30):
			if err := d.conn.WriteMessage(websocket.BinaryMessage, pkt); err != nil {
				log.Error("[danmaku] heartbeat error: ", err)
				d.connected = false
				break
			}
		case <-boolChange:
			log.Info("[danmaku] disconnected by connect()")
			break
		}
	}
}

func yyyyy() {
	danmaku := NewDanmaku(111, 222).
		OnDanmakuRecv(func(recv gson.JSON) {
			fmt.Println(recv.JSON("", ""))
		})
	danmaku.Start()
}
