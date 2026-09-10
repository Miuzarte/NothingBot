package utils

import (
	"encoding/base64"
	"io"
	"strings"
)

const base64Prefix = "base64://"

// ToBase64 将数据转换为 base64 编码的字符串，前缀为 "base64://"
func ToBase64(data []byte) string {
	b64Data := make([]byte, len(base64Prefix)+base64.StdEncoding.EncodedLen(len(data)))
	copy(b64Data[:len(base64Prefix)], base64Prefix)
	base64.StdEncoding.Encode(b64Data[len(base64Prefix):], data)
	return ToString(b64Data)
}

// Base64Builder 尽可能减少内存复制,
// size 可以为 0
func Base64Builder(size int, prefix bool) (vw *base64Builder) {
	vw = &base64Builder{}
	if prefix {
		vw.sb.Grow(len(base64Prefix) + base64.StdEncoding.EncodedLen(size))
		vw.sb.WriteString(base64Prefix)
	} else {
		vw.sb.Grow(base64.StdEncoding.EncodedLen(size))
	}
	vw.WriteCloser = base64.NewEncoder(base64.RawStdEncoding, &vw.sb)
	return vw
}

// base64Builder 尽可能减少内存复制,
// when finished writing,
// the caller must Close the returned encoder
// to flush any partially written blocks.
type base64Builder struct {
	io.WriteCloser
	sb strings.Builder
}

func (vw *base64Builder) Cap() int {
	return vw.sb.Cap()
}

func (vw *base64Builder) Grow(n int) {
	vw.sb.Grow(n)
}

func (vw *base64Builder) Len() int {
	return vw.sb.Len()
}

func (vw *base64Builder) Reset() {
	vw.sb.Reset()
}

func (vw *base64Builder) String() string {
	return vw.sb.String()
}
