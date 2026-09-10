package main

import (
	"regexp"
	"strings"
	"testing"

	"NothingBot_v4/slicesyntax"
)

func TestDeepSeekRegexp(t *testing.T) {
	reg := regexp.MustCompile(`(?si)^(dsr|ds)\s*(.*)`)
	r1 := reg.FindAllString("dsr你好", -1)
	r2 := reg.FindAllStringSubmatch("dsr你好", -1)
	t.Logf("%+v\n", r1)
	t.Logf("%+v\n", r2)
}

func TestBiliSearchRegexp(t *testing.T) {
	reg := regexp.MustCompile(BILI_SEARCH_REGEXP)
	m0 := reg.FindAllStringSubmatch("B搜视频 岁己SUI", -1)
	m1 := reg.FindAllStringSubmatch("B搜视频: 岁己SUI", -1)
	m2 := reg.FindAllStringSubmatch("B搜视频：岁己SUI", -1)
	m3 := reg.FindAllStringSubmatch("B搜岁己SUI", -1)
	for i, mmm := range [][][]string{m0, m1, m2, m3} {
		for j, mm := range mmm {
			for k, m := range mm {
				t.Logf("m%d[%d][%d]: %s\n", i, j, k, m)
			}
		}
	}
}

func TestGroupAtRegexp(t *testing.T) {
	for i, s := range []string{"谁at我", "谁艾特 [CQ:at,qq=2393827810,name=@rurudoBOT] ", "谁at了我"} {
		for j, m := range groupAtReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestGroupRecallRegexp(t *testing.T) {
	for i, s := range []string{"我撤回了什么", " [CQ:at,qq=2393827810,name=@rurudoBOT] 撤回了什么"} {
		for j, m := range groupRecallReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestPixivParseRegexp(t *testing.T) {
	for i, s := range []string{"pixiv.net/i/126799508 [2:4]", "pixiv.net/artworks/126799508", "kkp126799508 [6:8]", "看看批 126799508"} {
		for j, m := range pixivParseReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestHelpRegexp(t *testing.T) {
	for i, s := range []string{"/help", "/help biliparse"} {
		for j, m := range helpReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestEHentaiRegexp(t *testing.T) {
	// https://e-hentai.org/g/3138775/30b0285f9b
	// https://e-hentai.org/s/859299c9ef/3138775-7
	// https://e-hentai.org/s/0b2127ea05/3138775-8
	for i, match := range eHentaiGalleryUrlReg.FindStringSubmatch("https://e-hentai.org/g/3138775/30b0285f9b") {
		t.Logf("%d: %s\n", i, match)
	}
	for i, match := range eHentaiGalleryUrlReg.FindStringSubmatch("https://e-hentai.org/g/3138775/30b0285f9b[:80]") {
		t.Logf("%d: %s\n", i, match)
	}
	for i, matches := range eHentaiPageUrlReg.FindAllStringSubmatch("https://e-hentai.org/s/859299c9ef/3138775-7\nhttps://e-hentai.org/s/0b2127ea05/3138775-8", -1) {
		for j, match := range matches {
			t.Logf("%d.%d: %s\n", i, j, match)
		}
	}
	for _, s := range strings.SplitN("3138775-8", "-", 1) {
		t.Log(s)
	}
	for _, s := range strings.SplitN("3138775-8", "-", 2) {
		t.Log(s)
	}
}

func TestForwardSliceReg(t *testing.T) {
	for i, s := range []string{"[1][2]", "[:3]", "[4:6]", "[:][:]"} {
		for j, m := range forwardSliceReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestEHSearchReg(t *testing.T) {
	for i, s := range []string{"e搜Niyaniya Kyouju ha Tsukamari m"} {
		for j, m := range eHentaiSearchReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}

func TestEHGalleryUrlReg(t *testing.T) {
	for i, s := range []string{"https://e-hentai.org/g/3122492/32eb69dafd/[-1][2:6][8:10] --download"} {
		for j, m := range eHentaiGalleryUrlReg.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
				if k == 2 {
					t.Log(slicesyntax.ParseMulti(n))
				}
			}
		}
	}
}

func TestRegAdminRepeat(t *testing.T) {
	for i, s := range []string{"message.Image https://github.com/SocialSisterYi/bilibili-API-collect/blob/master/assets/img/logo.png?raw=true"} {
		for j, m := range regAdminRepeat.FindAllStringSubmatch(s, -1) {
			for k, n := range m {
				t.Logf("%d.%d.%d: %s\n", i, j, k, n)
			}
		}
	}
}
