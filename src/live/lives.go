//go:generate mockgen -package mock -destination mock/mock.go github.com/hr3lxphr6j/bililive-go/src/live Live
package live

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/bluele/gcache"
)

var (
	m                               = make(map[string]Builder)
	InitializingLiveBuilderInstance InitializingLiveBuilder
)

func Register(domain string, b Builder) {
	m[domain] = b
}

func getBuilder(domain string) (Builder, bool) {
	builder, ok := m[domain]
	return builder, ok
}

type Builder interface {
	Build(*url.URL, ...Option) (Live, error)
}

type InitializingLiveBuilder interface {
	Build(Live, *url.URL, ...Option) (Live, error)
}

type InitializingFinishedParam struct {
	InitializingLive Live
	Live             Live
	Info             *Info
}

type Options struct {
	Cookies *cookiejar.Jar
	Quality int
}

func NewOptions(opts ...Option) (*Options, error) {
	cookieJar, err := cookiejar.New(&cookiejar.Options{})
	if err != nil {
		return nil, err
	}
	options := &Options{Cookies: cookieJar, Quality: 0}
	for _, opt := range opts {
		opt(options)
	}
	return options, nil
}

func MustNewOptions(opts ...Option) *Options {
	options, err := NewOptions(opts...)
	if err != nil {
		panic(err)
	}
	return options
}

type Option func(*Options)

func WithKVStringCookies(u *url.URL, cookies string) Option {
	return func(opts *Options) {
		cookiesList := make([]*http.Cookie, 0)
		for _, pairStr := range strings.Split(cookies, ";") {
			pairs := strings.SplitN(pairStr, "=", 2)
			if len(pairs) != 2 {
				continue
			}
			cookiesList = append(cookiesList, &http.Cookie{
				Name:  strings.TrimSpace(pairs[0]),
				Value: strings.TrimSpace(pairs[1]),
			})
		}
		opts.Cookies.SetCookies(u, cookiesList)
	}
}

func WithQuality(quality int) Option {
	return func(opts *Options) {
		opts.Quality = quality
	}
}

type ID string

type StreamUrlInfo struct {
	Url         *url.URL
	Name        string
	Description string
	Resolution  int
	Vbitrate    int
}

type Live interface {
	SetLiveIdByString(string)
	GetLiveId() ID
	GetRawUrl() string
	GetInfo() (*Info, error)
	GetStreamUrls() ([]*url.URL, error)
	GetPlatformCNName() string
	GetLastStartTime() time.Time
	SetLastStartTime(time.Time)
}

type WrappedLive struct {
	Live
	cache gcache.Cache
}

func newWrappedLive(live Live, cache gcache.Cache) Live {
	return &WrappedLive{
		Live:  live,
		cache: cache,
	}
}

func (w *WrappedLive) GetInfo() (*Info, error) {
	i, err := w.Live.GetInfo()
	if err != nil {
		if info, err2 := w.cache.Get(w); err2 == nil {
			info.(*Info).RoomName = err.Error()
		}
		return nil, err
	}
	if w.cache != nil {
		w.cache.Set(w, i)
	}
	return i, nil
}

func New(url *url.URL, cache gcache.Cache, opts ...Option) (live Live, err error) {
	builder, ok := getBuilder(url.Host)
	if !ok {
		return nil, errors.New("not support this url")
	}
	live, err = builder.Build(url, opts...)
	if err != nil {
		return
	}
	live = newWrappedLive(live, cache)
	for i := 0; i < 3; i++ {
		var info *Info
		if info, err = live.GetInfo(); err == nil {
			if info.CustomLiveId != "" {
				live.SetLiveIdByString(info.CustomLiveId)
			}
			return
		}
		time.Sleep(1 * time.Second)
	}

	// when room initializaion is failed
	live, err = InitializingLiveBuilderInstance.Build(live, url, opts...)
	live = newWrappedLive(live, cache)
	live.GetInfo() // dummy call to initialize cache inside wrappedLive
	return
}
