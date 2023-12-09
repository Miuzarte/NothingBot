package main

import (
	"NothinBot/EasyBot"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"regexp"
	"time"

	b16384 "github.com/fumiama/go-base16384"
	"github.com/moxcomic/ihttp"
	log "github.com/sirupsen/logrus"
)

type BiliIdentity struct {
	Cookie       string `json:"cookie"`
	RefreshToken string `json:"refresh_token"`
}

const (
	biliIdentityPath = "bilibili.dat"
)

var (
	biliIdentity = BiliIdentity{}
)

func saveBiliIdentity(bi BiliIdentity) {
	s, err := json.Marshal(&bi)
	if err != nil {
		log.Error("[Bilibili] saveBiliIdentity marshal error: ", err)
		return
	}
	_ = os.Remove(biliIdentityPath)
	// err = os.WriteFile(biliIdentityPath, StringToBytes(base64.StdEncoding.EncodeToString(s)), 0o664)
	err = os.WriteFile(biliIdentityPath, b16384.Encode(s), 0o664)
	if err != nil {
		log.Error("[Bilibili] saveBiliIdentity write error: ", err)
		return
	}
}

func initLogin() {
	readBiliIdentity()
}

func readBiliIdentity() {
	raw, err := os.ReadFile(biliIdentityPath)
	if err != nil {
		log.Error("[Bilibili] readBiliIdentity read error: ", err)
		return
	}
	// s, _ := base64.StdEncoding.DecodeString(BytesToString(raw))
	s := b16384.Decode(raw)
	err = json.Unmarshal(s, &biliIdentity)
	if err != nil {
		log.Error("[Bilibili] readBiliIdentity unmarshal error: ", err)
		return
	}
	log.Trace("[Bilibili] biliIdentity: ", biliIdentity)
}

func checkBiliLogin(ctx *EasyBot.CQMessage) {
	if !ctx.IsPrivateSU() {
		return
	}
	matches := ctx.RegFindAllStringSubmatch(regexp.MustCompile(`(查看|保存|check|view|save)\s*(饼干|cookie)`))
	if len(matches) > 0 {
		switch matches[0][1] {
		case "查看", "check", "view":
			ctx.SendMsg(biliIdentity.Cookie, "\n\n", biliIdentity.RefreshToken)
		case "保存", "save":
			saveBiliIdentity(biliIdentity)
		}
	}
	matches = ctx.RegFindAllStringSubmatch(regexp.MustCompile("扫码登[录陆]"))
	if len(matches) == 0 {
		return
	}

	login, err := RequestLoginQR()
	if err != nil {
		ctx.SendMsg("获取二维码失败！\n", err)
		return
	}
	qrc, _ := NewQRcode().With(login.Url, 512)
	ctx.SendMsg(bot.Utils.Format.ImageBase64(qrc.ToBase64()))

	startTime := time.Now()
	errC := 0
	scanedSended := false
	for {
		scan, err := PollQRScan(login.QrcodeKey)
		if err != nil {
			errC++
			if errC >= 3 {
				ctx.SendMsg("轮询出现错误#", errC, "\n操作取消")
				return
			}
			ctx.SendMsg("轮询出现错误#", errC, "\n", err)
		}

		switch scan.Code {
		case 0: //已扫码并确认
			log.Info("[Bilibili] 已扫码并确认")
			ctx.SendMsg("确认成功")
			biliIdentity = BiliIdentity{
				Cookie:       scan.Cookie,
				RefreshToken: scan.RefreshToken,
			}
			saveBiliIdentity(biliIdentity)
			return
		case 86101: //未扫码
		case 86090: //已扫码未确认
			log.Info("[Bilibili] 已扫码未确认")
			if !scanedSended {
				ctx.SendMsg("已扫码")
				scanedSended = true
			}
		case 86038: //二维码已失效
			log.Info("[Bilibili] 二维码已失效")
			ctx.SendMsg("二维码已失效")
			return
		default:
			log.Error("[Bilibili] checkBiliLogin() unknown code: ", scan.Code)
			ctx.SendMsg("接口返回了未知状态码：", scan.Code, "\n操作取消")
			return
		}

		if time.Since(startTime) > 181*time.Second {
			ctx.SendMsg("操作超时")
			return
		}

		<-time.After(time.Second)
	}
}

const publicKeyPem = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----`

func getCorrespondPath() (encryptedHex string) {
	// 解析公钥
	block, _ := pem.Decode([]byte(publicKeyPem))
	if block == nil {
		log.Error("Failed to parse PEM block containing the public key")
		return
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		log.Error("Failed to parse DER encoded public key: ", err)
		return
	}
	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		log.Error("Failed to parse public key")
		return
	}

	// 获取当前时间戳并调用函数打印加密后的路径
	ts := time.Now().Unix()
	rng := rand.Reader
	label := []byte("OAEP Encrypted")
	message := []byte(fmt.Sprintf("refresh_%d", ts))

	// 使用公钥进行加密
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rng, pubKey, message, label)
	if err != nil {
		log.Error("Error encrypting: ", err)
		return
	}

	// 将加密后的结果转换为十六进制字符串
	encryptedHex = hex.EncodeToString(ciphertext)
	if err != nil {
		log.Error("Error encrypting:", err)
		return
	}
	return
}

// 检测cookie有效性
func validateCookie(cookie string) bool {
	g, err := ihttp.New().WithUrl("https://passport.bilibili.com/x/passport-login/web/cookie/info").
		WithHeaders(iheaders).WithCookie(cookie).
		Get().ToGson()
	if err != nil {
		log.Error("[bilibili] cookieChecker().ihttp请求错误: ", err)
	}
	switch g.Get("code").Int() {
	case 0:
		return true
	case -101:
		log.Error("[push] cookie已过期")
		bot.Log2SU.Error("[push] cookie已过期")
		return false
	default:
		log.Error("[push] 非正常cookie状态: ", g.JSON("", ""))
		bot.Log2SU.Error(fmt.Sprint("[push] 非正常cookie状态：", g.JSON("", "")))
		return false
	}
}

type LoginQRScan struct {
	Url          string `json:"url"`
	RefreshToken string `json:"refresh_token"`
	Timestamp    int    `json:"timestamp"`
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Cookie       string `json:"cookie"`
}

func PollQRScan(qrcodeKey string) (scanState *LoginQRScan, err error) {
	resp, headers, err := CallBiliApi(
		"https://passport.bilibili.com/x/passport-login/web/qrcode/poll", map[string]any{
			"qrcode_key": qrcodeKey,
		},
	)
	if err != nil {
		return nil, err
	}
	return &LoginQRScan{
		Url:          fmt.Sprint(resp.Data["url"]),
		RefreshToken: fmt.Sprint(resp.Data["refresh_token"]),
		Timestamp:    int(resp.Data["timestamp"].(float64)),
		Code:         int(resp.Data["code"].(float64)),
		Message:      fmt.Sprint(resp.Data["message"]),
		Cookie:       headers["Set-Cookie"],
	}, nil
}

type LoginQRCode struct {
	Url       string `json:"url"`
	QrcodeKey string `json:"qrcode_key"`
}

func RequestLoginQR() (*LoginQRCode, error) {
	resp, _, err := CallBiliApi("https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil)
	if err != nil {
		return nil, err
	}
	return &LoginQRCode{
		Url:       fmt.Sprint(resp.Data["url"]),
		QrcodeKey: fmt.Sprint(resp.Data["qrcode_key"]),
	}, nil
}
