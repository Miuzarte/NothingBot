package qrcode

import (
	"bytes"
	"image/color"
	"image/png"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

var colorScheme = barcode.ColorScheme{
	Model:      color.RGBAModel,
	Background: color.RGBA{255, 255, 255, 255},
	Foreground: color.RGBA{0xA6, 0x62, 0x61, 255},
}

func NewWithColor(content string, size int, color barcode.ColorScheme) ([]byte, error) {
	if size == 0 {
		size = 256
	}
	code, err := qr.EncodeWithColor(content, qr.L, qr.Auto, color)
	if err != nil {
		return nil, err
	}
	code, err = barcode.Scale(code, size, size)
	if err != nil {
		return nil, err
	}
	buf := bytes.Buffer{}
	err = png.Encode(&buf, code)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func New(content string, size int) ([]byte, error) {
	return NewWithColor(content, size, colorScheme)
}
