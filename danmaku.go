package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/ysmood/gson"
)

type danmaku struct {
	conn           *websocket.Conn
	connected      bool
	uid            int
	roomid         int
	onDanmakuRecv_ func(recv gson.JSON, uid int, roomid int)
}

func NewDanmaku(uid int, roomid int) (d *danmaku) {
	return &danmaku{
		uid:    uid,
		roomid: roomid,
	}
}

func (d *danmaku) OnDanmakuRecv(f func(recv gson.JSON, uid int, roomid int)) *danmaku {
	d.onDanmakuRecv_ = f
	return d
}

func (d *danmaku) Start() {
	go func() {
		for {
			if !d.connected {
				d.connect()
			}
			time.Sleep(time.Second)
		}
	}()
	go func() {
		for {
			if d.connected {
				go d.recvLoop()
				break
			}
			time.Sleep(time.Second)
		}
	}()
}

func (d *danmaku) Stop() {
	d.conn.Close()
}

func (d *danmaku) connect() {
	roomInfo := GetRoomInfo(d.roomid)
	if roomInfo == nil {
		log.Error("[danmaku] roomid: ", d.roomid, " room info is invalid.")
	}
	host := []string{"broadcastlv.chat.bilibili.com"}
	for _, h := range roomInfo.Get("data.host_list").([]any) {
		host = append(host, h.(map[string]any)["host"].(string))
	}
	token := roomInfo.GetString("data.token")
	if token == "" {
		log.Error("[danmaku] roomid: ", d.roomid, " token is invalid.")
	}
	reqHeader := &http.Header{}
	reqHeader.Set("User-Agent", iheaders["User-Agent"])
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/sub", host[0]), *reqHeader)
	if err != nil {
		log.Error("[danmaku] roomid: ", d.roomid, " failed to establish websocket connection: ", err)
	}
	err = SendEnterPacket(conn, d.uid, d.roomid, token)
	if err != nil {
		log.Error("[danmaku] roomid: ", d.roomid, " can not enter: ", err)
	}
	d.conn = conn
	d.connected = true
}

func (d *danmaku) recvLoop() {
	var pktJson gson.JSON
	go d.heartBeatLoop()
	for {
		if !d.connected {
			time.Sleep(time.Second)
			continue
		}
		msgType, data, err := d.conn.ReadMessage()
		if err == io.EOF {
			log.Error("[danmaku] roomid: ", d.roomid, " disconnected: ", err)
			d.connected = false
			time.Sleep(time.Second * 10)
			continue
		}
		if err != nil {
			log.Error("[danmaku] roomid: ", d.roomid, " get error message: ", err)
			d.connected = false
			time.Sleep(time.Second * 10)
			continue
		}
		if msgType != websocket.BinaryMessage {
			log.Error("[danmaku] roomid: ", d.roomid, " packet not binary.")
			d.connected = false
			time.Sleep(time.Second * 10)
			continue
		}
		for _, pkt := range NewPacketFromBytes(data).Parse() {
			pktJson = gson.NewFrom(string(pkt.Body))
			log.Trace("[danmaku] roomid: ", d.roomid, " 接收数据包: ", string(pkt.Body))
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
				case "FULL_SCREEN_SPECIAL_EFFECT":
				case "GUARD_BUY":
				case "GUARD_HONOR_THOUSAND":
				case "INTERACT_WORD":
				case "LIKE_INFO_V3_CLICK":
				case "LIKE_INFO_V3_UPDATE":
				case "LIVE_PANEL_CHANGE":
				//case "LIVE":
				case "MESSAGEBOX_USER_GAIN_MEDAL":
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
				case "ROOM_SKIN_MSG":
				case "SEND_GIFT":
				case "SPREAD_SHOW_FEET_V2":
				case "STOP_LIVE_ROOM_LIST":
				case "SUPER_CHAT_MESSAGE":
				case "TRADING_SCORE":
				case "USER_TOAST_MSG":
				case "WATCHED_CHANGE":
				case "WIDGET_BANNER":
				case "WIDGET_GIFT_STAR_PROCESS":
				default:
					log.Debug("[danmaku] 直播间 ", d.roomid, " 接收数据: {\"cmd\": ", cmd, "}")
				}
			case !pktJson.Get("code").Nil():
				code := pktJson.Get("code").Str()
				log.Info("[danmaku] roomid: ", d.roomid, " 接收数据: {\"code\": ", code, "}")
			default:
				if len(string(pkt.Body)) > 4 { //过滤奇怪的数据包导致控制台发声
					log.Debug("[danmaku] roomid: ", d.roomid, " 原始数据: ", string(pkt.Body))
				}
			}
			if d.onDanmakuRecv_ != nil {
				d.onDanmakuRecv_(pktJson, d.uid, d.roomid)
			}
		}
	}
}

func (d *danmaku) heartBeatLoop() {
	pkt := NewPacket(Plain, HeartBeat, nil).Build()
	boolChange := make(chan struct{})
	go func() {
		for {
			if !d.connected {
				boolChange <- struct{}{}
				break
			}
			time.Sleep(time.Second)
		}
	}()
	for {
		select {
		case <-time.After(time.Second * 30):
			if err := d.conn.WriteMessage(websocket.BinaryMessage, pkt); err != nil {
				log.Warn("[danmaku] heartbeat error: ", err)
				d.connected = false
				break
			}
		case <-boolChange:
			break
		}
	}
}

const (
	Plain = iota
	Popularity
	Zlib
	Brotli
)

const (
	_ = iota
	_
	HeartBeat
	HeartBeatResponse
	_
	Notification
	_
	RoomEnter
	RoomEnterResponse
)

type Packet struct {
	PacketLength    int
	HeaderLength    int
	ProtocolVersion uint16
	Operation       uint32
	SequenceID      int
	Body            []byte
}

func NewPacket(protocolVersion uint16, operation uint32, body []byte) *Packet {
	return &Packet{
		ProtocolVersion: protocolVersion,
		Operation:       operation,
		Body:            body,
	}
}

func NewPlainPacket(operation int, body []byte) *Packet {
	return NewPacket(Plain, uint32(operation), body)
}

func (p *Packet) Build() []byte {
	rawBuf := []byte{0, 0, 0, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	binary.BigEndian.PutUint16(rawBuf[6:], p.ProtocolVersion)
	binary.BigEndian.PutUint32(rawBuf[8:], p.Operation)
	rawBuf = append(rawBuf, p.Body...)
	binary.BigEndian.PutUint32(rawBuf, uint32(len(rawBuf)))
	return rawBuf
}

type Enter struct {
	UID       int    `json:"uid"`
	Bvuid     string `json:"bvuid"`
	RoomID    int    `json:"roomid"`
	ProtoVer  int    `json:"protover"`
	Platform  string `json:"platform"`
	ClientVer string `json:"clientver"`
	Type      int    `json:"type"`
	Key       string `json:"key"`
}

func NewEnterPacket(uid int, roomID int, key string) []byte {
	ent := &Enter{
		UID:       uid,
		Bvuid:     cookieBuvid,
		RoomID:    roomID,
		ProtoVer:  3,
		Platform:  "web",
		ClientVer: "1.14.3",
		Type:      2,
		Key:       key,
	}
	m, err := json.Marshal(ent)
	if err != nil {
		log.Panic("[danmaku] NewEnterPacket JsonMarshal failed: ", err)
	}
	pkt := NewPlainPacket(RoomEnter, m)
	return pkt.Build()
}

func ParseJson(reader io.ReadCloser) *viper.Viper {
	v := viper.New()
	v.SetConfigType("json")
	err := v.ReadConfig(reader)
	if err != nil {
		return nil
	}
	defer reader.Close()
	return v
}

func GetRoomInfo(roomid int) *viper.Viper {
	resp, err := http.Get(fmt.Sprintf("https://api.live.bilibili.com/xlive/web-room/v1/index/getDanmuInfo?id=%d&type=0", roomid))
	if err != nil {
		log.Error("[danmaku] GerRoomInfo().http.Get发生错误: ", err)
		return nil
	}
	return ParseJson(resp.Body)
}

func SendEnterPacket(conn *websocket.Conn, uid, roomID int, token string) error {
	pkt := NewEnterPacket(uid, roomID, token)
	if err := conn.WriteMessage(websocket.BinaryMessage, pkt); err != nil {
		return err
	}
	return nil
}

func NewPacketFromBytes(data []byte) *Packet {
	packLen := binary.BigEndian.Uint32(data[0:4])
	// 校验包长度
	if int(packLen) != len(data) {
		log.Error("[danmaku] error packet.")
	}
	pv := binary.BigEndian.Uint16(data[6:8])
	op := binary.BigEndian.Uint32(data[8:12])
	body := data[16:packLen]
	packet := NewPacket(pv, op, body)
	return packet
}

func (p *Packet) Parse() []*Packet {
	switch p.ProtocolVersion {
	case Popularity:
		fallthrough
	case Plain:
		return []*Packet{p}
	case Zlib:
		z, err := zlibParser(p.Body)
		if err != nil {
			log.Error("[danmaku] zlib error: ", err)
		}
		return Slice(z)
	case Brotli:
		b, err := brotliParser(p.Body)
		if err != nil {
			log.Error("[danmaku] brotli error: ", err)
		}
		return Slice(b)
	default:
		log.Error("[danmaku] unknown protocolVersion.")
	}
	return nil
}

func Slice(data []byte) []*Packet {
	var packets []*Packet
	total := len(data)
	cursor := 0
	for cursor < total {
		packLen := int(binary.BigEndian.Uint32(data[cursor : cursor+4]))
		packets = append(packets, NewPacketFromBytes(data[cursor:cursor+packLen]))
		cursor += packLen
	}
	return packets
}

func zlibParser(b []byte) ([]byte, error) {
	var rdBuf []byte
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	rdBuf, _ = io.ReadAll(zr)
	return rdBuf, nil
}

func brotliParser(b []byte) ([]byte, error) {
	zr := brotli.NewReader(bytes.NewReader(b))
	rdBuf, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	return rdBuf, nil
}
