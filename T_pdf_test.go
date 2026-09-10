package main

import (
	"bytes"
	"image"
	"os"
	"testing"
)

var imgPaths = []string{
	`a:\Miuzarte\Pictures\98412643_p0_1x1.jpg`,
	`a:\Miuzarte\Pictures\头像.jpg`,
	`a:\Miuzarte\Pictures\REALMS.jpg`,
}

func imgGetBounds(img []byte) (bounds image.Rectangle, err error) {
	var i image.Image
	i, _, err = image.Decode(bytes.NewReader(img))
	if err != nil {
		return
	}
	bounds = i.Bounds()
	return
}

func TestPdfPageCount(t *testing.T) {
	pdf := NewImagePdf()
	for _, imgPath := range imgPaths {
		img, err := os.ReadFile(imgPath)
		if err != nil {
			t.Fatal(err)
		}
		bounds, err := imgGetBounds(img)
		if err != nil {
			t.Fatal(err)
		}
		pdf.AddImage(img, bounds, "")
		t.Log(pdf.PageCount())
		t.Log(pdf.PageNo())
	}
	pdf.OutputFileAndClose(`.\TestPdfPageCount.pdf`)
}
