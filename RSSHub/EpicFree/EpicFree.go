package EpicFree

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	ROUTER  = `/epicgames/freegames/zh-CN/CN`
	CRONTAB = `0 0 * * *`
	// DURATION = 1 * time.Hour
)

type RssHubResource struct {
	Router  string
	Crontab string
}

func (r *RssHubResource) GetRouter() string {
	return r.Router
}

func (r *RssHubResource) GetCrontab() string {
	return r.Crontab
}

var Resource = &RssHubResource{
	Router:  ROUTER,
	Crontab: CRONTAB,
}

// internal structs matching RSS structure
type rss struct {
	Channel channel `xml:"channel"`
}

type channel struct {
	Items []rawItem `xml:"item"`
}

type rawItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	Guid        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
}

// FeedItem is the normalized result after parsing the Epic free games RSS feed.
type FeedItem struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ImageURL    string    `json:"image_url"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Author      string    `json:"author"`
	StoreURL    string    `json:"store_url"`
	RawPubDate  string    `json:"raw_pub_date"`
	RawDeadline string    `json:"raw_deadline"`
	RawHTML     string    `json:"raw_html"`
}

func (fi *FeedItem) String() string {
	sTime := fi.StartTime.In(time.Local).Format("01/02 15:04")
	eTime := fi.EndTime.In(time.Local).Format("01/02 15:04")
	return fmt.Sprintf("[CQ:image,file=%s]%s\n%s\n%s\n%s - %s\n%s",
		fi.ImageURL, fi.Title, fi.Author, fi.Description, sTime, eTime, fi.StoreURL)
}

// ParseFeed will be implemented to parse the XML into []FeedItem.
func ParseFeed(data []byte) ([]FeedItem, error) {
	rssFeed, err := unmarshalRSS(data)
	if err != nil {
		return nil, fmt.Errorf("xml unmarshal: %w", err)
	}
	items := make([]FeedItem, 0, len(rssFeed.Channel.Items))
	for _, it := range rssFeed.Channel.Items {
		intro := extractIntro(it.Description)
		img := extractImage(it.Description)
		endRaw, endTime := extractDeadline(it.Description)
		pubRaw := strings.TrimSpace(it.PubDate)
		startTime := parseTimeBest(pubRaw)
		storeURL := strings.TrimSpace(it.Link)
		if storeURL == "" {
			// some RSS might have newlines/indentation; collapse whitespace
			storeURL = strings.TrimSpace(strings.ReplaceAll(it.Link, "\n", ""))
		}
		items = append(items, FeedItem{
			Title:       strings.TrimSpace(it.Title),
			Description: intro,
			ImageURL:    img,
			StartTime:   startTime,
			EndTime:     endTime,
			Author:      strings.TrimSpace(it.Author),
			StoreURL:    storeURL,
			RawPubDate:  pubRaw,
			RawDeadline: endRaw,
			RawHTML:     it.Description,
		})
	}
	return items, nil
}

// For time parsing: keep a list of layouts we may need (RFC1123 etc.)
var timeLayouts = []string{
	time.RFC1123Z,                   // Thu, 25 Sep 2025 15:00:00 GMT
	time.RFC1123,                    // fallback
	time.RFC3339Nano,                // 2025-10-02T15:00:00.000Z
	time.RFC3339,                    // 2025-10-02T15:00:00Z
	"2006-01-02T15:04:05.000Z07:00", // explicit milli layout
}

// unmarshal wrapper (may be used by ParseFeed later)
func unmarshalRSS(data []byte) (rss, error) {
	var r rss
	err := xml.Unmarshal(data, &r)
	return r, err
}

// extractIntro grabs the first <p>...</p> text content (stripped) as intro.
var pTagRegexp = regexp.MustCompile(`(?is)<p>(.*?)</p>`)

func extractIntro(html string) string {
	matches := pTagRegexp.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return ""
	}
	// first paragraph that is non-empty and not containing 'Free Now'
	for _, m := range matches {
		txt := stripTags(m[1])
		txt = strings.TrimSpace(txt)
		if txt == "" {
			continue
		}
		if strings.Contains(strings.ToLower(txt), "free now") {
			continue
		}
		return txt
	}
	return stripTags(matches[0][1])
}

// extractImage finds first <img ... src="..."> URL.
var imgTagRegexp = regexp.MustCompile(`(?is)<img[^>]+src=["']([^"'>]+)["']`)

func extractImage(html string) string {
	m := imgTagRegexp.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractDeadline finds a line like 'Free Now to 2025-10-02T15:00:00.000Z'
var deadlineRegexp = regexp.MustCompile(`(?i)Free\s+Now\s+to\s+([0-9TZ:.-]+Z?)`)

func extractDeadline(html string) (raw string, t time.Time) {
	m := deadlineRegexp.FindStringSubmatch(html)
	if len(m) > 1 {
		raw = m[1]
		t = parseTimeBest(raw)
	}
	return
}

// parseTimeBest attempts multiple layouts.
func parseTimeBest(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if tm, err := time.Parse(layout, s); err == nil {
			return tm.UTC()
		}
	}
	// attempt to normalize GMT without offset
	if strings.HasSuffix(s, " GMT") {
		if tm, err := time.Parse(time.RFC1123, s); err == nil {
			return tm.UTC()
		}
	}
	return time.Time{}
}

// stripTags removes HTML tags (naive) from a fragment.
var tagRegexp = regexp.MustCompile(`(?is)<[^>]+>`)

func stripTags(s string) string {
	s = tagRegexp.ReplaceAllString(s, "")
	// collapse whitespace
	parts := strings.Fields(s)
	return strings.TrimSpace(strings.Join(parts, " "))
}

// helper to debug (not exported)
func debugDump(items []FeedItem) string {
	var b bytes.Buffer
	for i, it := range items {
		fmt.Fprintf(&b, "#%d %s (%s -> %s)\n", i+1, it.Title, it.StartTime, it.EndTime)
	}
	return b.String()
}
