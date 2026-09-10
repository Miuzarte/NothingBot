package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	env "NothingBot_v4/environment"
	"NothingBot_v4/utils"
)

func writePageNum(w io.Writer, start, end int) {
	if start == end {
		fmt.Fprintf(w, "%d", start)
	} else {
		fmt.Fprintf(w, "%d - %d", start, end)
	}
}

type MangaConfig struct {
	EHentaiEnabled   bool
	NHentaiEnabled   bool
	JmComicEnabled   bool
	PicaComicEnabled bool

	SaltLength          int
	MaxForwardImages    int
	ForwardMsgBatchSize int
	MaxPdfImages        int

	EHentaiCookie struct {
		IpbMemberId string
		IpbPassHash string
		Igneous     string
		Sk          string // 不给的话搜索结果只有英文
	}
	// NHentaiCookie map[string]string
	// JmComicCookie map[string]string
	PicaComicCookie struct {
		Token    string
		Account  string
		Password string
	}

	NHentaiApiKey string

	Threads int

	EHentaiCacheDir   string
	NHentaiCacheDir   string
	JmComicCacheDir   string
	PicaComicCacheDir string

	EHentaiCoverShowRating float64

	UseEnvProxy              bool
	EHentaiUseDomainFronting bool
	EHentaiGalleryCache      bool
	EHentaiUseEhTagDB        bool

	List
}

const mangaMId ModuleId = "manga"

var mangaConfig = MangaConfig{}

var moduleManga = Module{
	ModuleMeta: ModuleMeta{
		Name:   mangaMId.WithSuffix("init"),
		Desc:   "initialize manga config",
		Hidden: true,
	},
	// [TODO] 管理父模块子模块读写锁关系
	// before [moduleEHentai],
	// [moduleNHentai],
	// [moduleJmComic],
	// [modulePicaComic]
	Priority: 0,
	Disable:  env.Testing,
}

func init() {
	NoBuildPrintFile("M_MangaParse_utils.go")

	moduleManga.Init = initManga
	moduleManga.ReInit = initManga
	modules.Add(&moduleManga)
}

func initManga() {
	err := config.DecodeModule(mangaMId, &mangaConfig)
	if err != nil {
		log.Error(err)
		return
	}
}

// 避免同时解析
type Locker[T comparable] struct {
	Locks utils.Set[T]
	Mu    sync.Mutex
}

func (l *Locker[T]) TryLock(list []T) (failed []T) {
	if l.Locks == nil {
		l.Locks = utils.Set[T]{}
	}
	l.Mu.Lock()
	defer l.Mu.Unlock()

	// try to get the locks
	for _, item := range list {
		if l.Locks.Ok(item) {
			failed = append(failed, item)
		}
	}
	if len(failed) != 0 {
		return
	}

	// hold the locks
	l.Locks.Add(list...)

	return nil
}

func (l *Locker[T]) Unlock(list []T) {
	if l.Locks == nil {
		l.Locks = utils.Set[T]{}
	}
	l.Mu.Lock()
	defer l.Mu.Unlock()
	l.Locks.Delete(list...)
}

func nHUrlDeconstruct(u string) (gId, pNum int, err error) {
	// '1' index
	// https://nhentai.net/g/{gId}/{pNum}
	// https://nhentai.net/g/540994/1
	u = strings.TrimSuffix(u, "/")
	splits := strings.Split(u, "/")
	for i, s := range splits {
		if s == "g" {
			// i   g
			// i+1 540994
			// i+2 1
			switch {
			case len(splits) >= i+3:
				pNum, err = strconv.Atoi(splits[i+2])
				if err != nil {
					err = fmt.Errorf("failed to deconstruct NHentai url %q: %w", u, err)
					return
				}

				fallthrough

			case len(splits) >= i+2:
				gId, err = strconv.Atoi(splits[i+1])
				if err != nil {
					err = fmt.Errorf("failed to deconstruct NHentai url %q: %w", u, err)
					return
				}

			default:
				break
			}
			return
		}
	}
	return 0, 0, fmt.Errorf("failed to deconstruct NHentai url %q", u)
}
