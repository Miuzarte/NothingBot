package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"io"
	"os"
	"testing"
	"time"

	"github.com/KononK/resize"

	"golang.org/x/image/webp"
)

func TestScale(t *testing.T) {
	f, err := os.Open(`a:\Miuzarte\Pictures\REALMS.jpg`)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	f.Close()
	t.Log(len(b))
	for i := range resize.InterpolationFunction(6) {
		tn := time.Now()
		scaled, err := scaleDownImageJpeg(b, 2560)
		t.Logf("Interpolation %d took %s", i, time.Since(tn))
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		t.Log(len(scaled))
		f, err = os.Create(fmt.Sprintf(`a:\Miuzarte\Pictures\REALMS_scaled_%d.jpg`, i))
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		_, err = f.Write(scaled)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		f.Close()
	}
}

func TestWebpFailed(t *testing.T) {
	f, err := os.Open(`a:\Miuzarte\Downloads\24.webp`)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	f.Close()
	t.Log(len(b))
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	t.Log(format)
	t.Log(img.Bounds().Dx(), img.Bounds().Dy())
}

func TestWebp(t *testing.T) {
	f, err := os.Open(`a:\Miuzarte\Downloads\24.webp`)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	f.Close()
	t.Log(len(b))
	img, err := webp.Decode(bytes.NewReader(b))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	t.Log(img.Bounds().Dx(), img.Bounds().Dy())
}

func TestJpeg(t *testing.T) {
	f, err := os.Open(`B:\Programming\Go\NothingBot_v4\test.jpg`)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	f.Close()
	t.Log(len(b))
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	t.Log(img.Bounds().Dx(), img.Bounds().Dy())
}
