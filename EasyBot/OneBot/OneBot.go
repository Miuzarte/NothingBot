package OneBot

import "strconv"

type Message struct {
	Time        int    `json:"time"`
	SelfId      int64  `json:"self_id"`
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	SubType     string `json:"sub_type"`
	MessageId   int    `json:"message_id"`
	GroupId     int    `json:"group_id"`
	PeerId      int64  `json:"peer_id"`
	UserId      int    `json:"user_id"`
	Message     []struct {
		Type string `json:"type"`
		Data struct {
			Qq     string `json:"qq,omitempty"`
			Text   string `json:"text,omitempty"`
			File   string `json:"file,omitempty"`
			Url    string `json:"url,omitempty"`  // image / file
			Data   string `json:"data,omitempty"` // card message data (marshaled json)
			Sub    string `json:"sub,omitempty"`
			Biz    int    `json:"biz,omitempty"`
			Size   int    `json:"size,omitempty"`
			Expire int    `json:"expire,omitempty"`
			Name   string `json:"name,omitempty"`
			Id     string `json:"id,omitempty"`
		} `json:"data"`
	} `json:"message"`
	RawMessage string `json:"raw_message"`
	Font       int    `json:"font"`
	Sender     struct {
		UserId   int    `json:"user_id"`
		Nickname string `json:"nickname"`
		Card     string `json:"card"`
		Role     string `json:"role"`
		Title    string `json:"title"`
		Level    string `json:"level"`
	} `json:"sender"`
}

func handleMsg(msg *Message) {
	allTheText := ""
	var atList []int
	var imgList []string
	var isCardMsg bool
	for _, s := range msg.Message {
		switch s.Type {
		case "at":
			qq, _ := strconv.Atoi(s.Data.Qq)
			atList = append(atList, qq)
		case "text":
			text := s.Data.Text
			allTheText += text
		case "image":
			file := s.Data.File
			url := s.Data.Url
			_ = file
			imgList = append(imgList, url)
		case "json":
			rawJson := s.Data.Data
			_ = rawJson
			isCardMsg = true
			_ = isCardMsg
		case "file":
			sub := s.Data.Sub
			biz := s.Data.Biz
			size := s.Data.Size
			expire := s.Data.Expire
			name := s.Data.Name
			id := s.Data.Id
			_, _, _, _, _, _ = sub, biz, size, expire, name, id
		}
	}
}

type Poke struct {
	Time       int    `json:"time"`
	SelfId     int64  `json:"self_id"`
	PostType   string `json:"post_type"`
	NoticeType string `json:"notice_type"`
	SubType    string `json:"sub_type"`
	OperatorId int    `json:"operator_id"`
	UserId     int    `json:"user_id"`
	TargetId   int64  `json:"target_id"`

	GroupId  int `json:"group_id"`  // 群聊戳一戳
	SenderId int `json:"sender_id"` // 私聊戳一戳
}

type GroupUpload struct {
	Time       int    `json:"time"`
	SelfId     int64  `json:"self_id"`
	PostType   string `json:"post_type"`
	NoticeType string `json:"notice_type"`
	GroupId    int    `json:"group_id"`
	OperatorId int    `json:"operator_id"`
	UserId     int    `json:"user_id"`
	File       struct {
		Id    string `json:"id"`
		Name  string `json:"name"`
		Size  int    `json:"size"`
		Busid int    `json:"busid"`
		Url   string `json:"url"`
	} `json:"file"`
}

type PrivateUpload struct {
	Time        int    `json:"time"`
	SelfId      int64  `json:"self_id"`
	PostType    string `json:"post_type"`
	NoticeType  string `json:"notice_type"`
	OperatorId  int    `json:"operator_id"`
	UserId      int    `json:"user_id"`
	SenderId    int    `json:"sender_id"`
	PrivateFile struct {
		Id     string `json:"id"`
		Name   string `json:"name"`
		Size   int    `json:"size"`
		SubId  string `json:"sub_id"`
		Url    string `json:"url"`
		Expire int    `json:"expire"`
	} `json:"private_file"`
}

type Heartbeat struct {
	Time          int    `json:"time"`
	SelfId        int    `json:"self_id"`
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
	Status        struct {
		Self struct {
			Platform string `json:"platform"`
			UserId   int    `json:"user_id"`
		} `json:"self"`
		Online   bool   `json:"online"`
		Good     bool   `json:"good"`
		QqStatus string `json:"qq.status"`
	} `json:"status"`
	Interval int `json:"interval"`
}

type Lifecycle struct {
	Time          int    `json:"time"`
	SelfId        int    `json:"self_id"`
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
	Status        struct {
		Self struct {
			Platform string `json:"platform"`
			UserId   int    `json:"user_id"`
		} `json:"self"`
		Online   bool   `json:"online"`
		Good     bool   `json:"good"`
		QqStatus string `json:"qq.status"`
	} `json:"status"`
	Interval int `json:"interval"`
}
