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
		RoomID:    roomID,
		ProtoVer:  2,
		Platform:  "web",
		ClientVer: "1.14.3",
		Type:      2,
		Key:       key,
	}
	m, err := json.Marshal(ent)
	if err != nil {
		panic(fmt.Sprintf("NewEnterPacket JsonMarshal failed: %v", err))
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
		panic(err)
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

func HeartBeatLoop(conn *websocket.Conn) {
	pkt := NewPacket(Plain, HeartBeat, nil).Build()
	for {
		<-time.After(time.Second * 30)
		if err := conn.WriteMessage(websocket.BinaryMessage, pkt); err != nil {
			log.Errorln("[danmaku] heartbeat error:", err)
		}
		if configChange { //配置文件更新 断开重新发起
			break
		}
	}
}

func NewPacketFromBytes(data []byte) *Packet {
	packLen := binary.BigEndian.Uint32(data[0:4])
	// 校验包长度
	if int(packLen) != len(data) {
		log.Errorln("[danmaku] error packet.")
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
			log.Errorln("[danmaku] zlib error:", err)
		}
		return Slice(z)
	case Brotli:
		b, err := brotliParser(p.Body)
		if err != nil {
			log.Errorln("[danmaku] brotli error:", err)
		}
		return Slice(b)
	default:
		log.Errorln("[danmaku] unknown protocolVersion.")
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

func RecvLoop(conn *websocket.Conn) {
	var pktJson gson.JSON
	for {
		msgType, data, err := conn.ReadMessage()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Errorln("[danmaku] get error message:", err)
			continue
		}
		if msgType != websocket.BinaryMessage {
			log.Errorln("[danmaku] packet not binary.")
			continue
		}
		for _, pkt := range NewPacketFromBytes(data).Parse() {
			log.Debugln("[danmaku] 接收数据:", string(pkt.Body))
			pktJson = gson.NewFrom(string(pkt.Body))
			go liveChecker(pktJson)
		}
		if configChange { //配置文件更新 断开重新发起
			break
		}
	}
}

func connectDanmu(uid int, roomID int) {
	roomInfo := GetRoomInfo(roomID)
	if roomInfo == nil {
		log.Errorln("[danmaku] room info is invalid.")
	}
	host := []string{"broadcastlv.chat.bilibili.com"}
	for _, h := range roomInfo.Get("data.host_list").([]any) {
		host = append(host, h.(map[string]any)["host"].(string))
	}
	token := roomInfo.GetString("data.token")
	if token == "" {
		log.Errorln("[danmaku] token is invalid.")
	}
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s/sub", host[0]), nil)
	if err != nil {
		log.Errorln("[danmaku] failed to establish websocket connection.")
	}
	err = SendEnterPacket(conn, uid, roomID, token)
	if err != nil {
		log.Errorln("[danmaku] can not enter room.")
	}
	go RecvLoop(conn)
	go HeartBeatLoop(conn)
}
