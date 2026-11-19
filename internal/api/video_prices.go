package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"videoflow-go/internal/costs"
)

// Live prices: each service's current price is read from the service's own price feed
// or pricing page, so comparing and picking the cheapest never relies on old numbers.

// priceSource is one place a price is published. parse returns what it found, keyed
// "family" for a token rate (dollars per 1,000 tokens) or "family:resolution" for
// dollars per second.
type priceSource struct {
	Providers []string // every service this source prices
	URL       string
	Parse     func(body string) map[string]float64
}

// videoPriceMaxAge is how old a price may be before it is read again.
const videoPriceMaxAge = 15 * time.Minute

var (
	tagPattern    = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	spacePattern  = regexp.MustCompile(`\s+`)
	argoPattern   = regexp.MustCompile(`(480p|720p|1080p) \$([0-9.]+) / second`)
	runwayPattern = regexp.MustCompile(`(480p|720p|1080p) [0-9.]+ credits per second \$([0-9.]+) per second`)
	kiePattern    = regexp.MustCompile(`Seedance 2\.0 costs [0-9.]+ credits \(\$([0-9.]+)\) per second at 480p, [0-9.]+ credits \(\$([0-9.]+)\) at 720p(?:, [0-9.]+ credits \(\$([0-9.]+)\) at 1080p)?`)
	kieFast       = regexp.MustCompile(`Seedance 2\.0 Fast costs [0-9.]+ credits \(\$([0-9.]+)\) per second at 480p and [0-9.]+ credits \(\$([0-9.]+)\) at 720p`)
	wavePerSecond = regexp.MustCompile(`(?m)^\| *(480p|720p|1080p) *\| *(?:~~\$[0-9.]+~~ *)?\$([0-9.]+) *\|\s*$`)
	wavePerFive   = regexp.MustCompile(`(?m)^\| *(480p|720p|1080p) *\| *5 s *\| *(?:~~\$[0-9.]+~~ *)?\$([0-9.]+) *\|`)
	replicateTier = regexp.MustCompile(`"value": "non_video_in"\}, \{[^}]*"value": "(480p|720p|1080p)"\}\][^\[]*"prices": \[\{[^}]*"price": "\$([0-9.]+)"`)
)

// pageText strips a web page down to its words.
func pageText(body string) string {
	return spacePattern.ReplaceAllString(html.UnescapeString(tagPattern.ReplaceAllString(body, " ")), " ")
}

// firstPerResolution keeps the first price a pattern finds for each resolution.
func firstPerResolution(pattern *regexp.Regexp, text, family string, divide float64) map[string]float64 {
	out := map[string]float64{}
	for _, m := range pattern.FindAllStringSubmatch(text, -1) {
		key := family + ":" + m[1]
		if _, seen := out[key]; seen {
			continue
		}
		if price, err := strconv.ParseFloat(m[2], 64); err == nil {
			out[key] = price / divide
		}
	}
	return out
}

func sectionAfter(text, start, end string) string {
	i := strings.Index(text, start)
	if i < 0 {
		return ""
	}
	text = text[i+len(start):]
	if j := strings.Index(text, end); j >= 0 {
		text = text[:j]
	}
	return text
}

func defaultPriceSources() []priceSource {
	argo := func(family string) func(string) map[string]float64 {
		return func(body string) map[string]float64 {
			return firstPerResolution(argoPattern, pageText(body), family, 1)
		}
	}
	wave := func(family string) func(string) map[string]float64 {
		return func(body string) map[string]float64 {
			if out := firstPerResolution(wavePerSecond, body, family, 1); len(out) > 0 {
				return out
			}
			return firstPerResolution(wavePerFive, body, family, 5)
		}
	}
	replicate := func(family string) func(string) map[string]float64 {
		return func(body string) map[string]float64 {
			return firstPerResolution(replicateTier, html.UnescapeString(body), family, 1)
		}
	}
	return []priceSource{
		{Providers: []string{"openrouter", "byteplus"}, URL: "https://openrouter.ai/api/v1/videos/models", Parse: func(body string) map[string]float64 {
			var feed struct {
				Data []struct {
					ID   string `json:"id"`
					SKUs struct {
						Tokens string `json:"video_tokens"`
					} `json:"pricing_skus"`
				} `json:"data"`
			}
			out := map[string]float64{}
			if json.Unmarshal([]byte(body), &feed) != nil {
				return out
			}
			for _, m := range feed.Data {
				if family := costs.FamilyOf("openrouter", m.ID); family != "" {
					if perToken, err := strconv.ParseFloat(m.SKUs.Tokens, 64); err == nil {
						out[family] = perToken * 1000
					}
				}
			}
			return out
		}},
		{Providers: []string{"loopling"}, URL: "https://loopling.ai/v1/models", Parse: func(body string) map[string]float64 {
			var feed struct {
				Data []struct {
					ID      string `json:"id"`
					Pricing struct {
						PerSecond map[string]float64 `json:"usd_payg_per_second"`
					} `json:"pricing"`
				} `json:"data"`
			}
			out := map[string]float64{}
			if json.Unmarshal([]byte(body), &feed) != nil {
				return out
			}
			for _, m := range feed.Data {
				if family := costs.FamilyOf("loopling", m.ID); family != "" {
					for resolution, price := range m.Pricing.PerSecond {
						out[family+":"+resolution] = price
					}
				}
			}
			return out
		}},
		{Providers: []string{"argolink"}, URL: "https://argolink.io/en/models/seedance-2.0", Parse: argo("seedance-2.0")},
		{Providers: []string{"argolink"}, URL: "https://argolink.io/en/models/seedance-2.0-fast", Parse: argo("seedance-2.0-fast")},
		{Providers: []string{"wavespeed"}, URL: "https://wavespeed.ai/models/bytedance/seedance-2.0/image-to-video/llms.txt", Parse: wave("seedance-2.0")},
		{Providers: []string{"wavespeed"}, URL: "https://wavespeed.ai/models/bytedance/seedance-2.0-fast/image-to-video/llms.txt", Parse: wave("seedance-2.0-fast")},
		{Providers: []string{"replicate"}, URL: "https://replicate.com/bytedance/seedance-2.0", Parse: replicate("seedance-2.0")},
		{Providers: []string{"replicate"}, URL: "https://replicate.com/bytedance/seedance-2.0-fast", Parse: replicate("seedance-2.0-fast")},
		{Providers: []string{"runway"}, URL: "https://dev.runwayml.com/learn/seedance-2-0-api", Parse: func(body string) map[string]float64 {
			text := pageText(body)
			out := firstPerResolution(runwayPattern, sectionAfter(text, "Seedance 2.0 Resolution Credits USD", "Minimum"), "seedance-2.0", 1)
			for key, price := range firstPerResolution(runwayPattern, sectionAfter(text, "Seedance 2.0 Fast Resolution Credits USD", "Minimum"), "seedance-2.0-fast", 1) {
				out[key] = price
			}
			return out
		}},
		{Providers: []string{"kie"}, URL: "https://kie.ai/seedance-2-0", Parse: func(body string) map[string]float64 {
			text, out := pageText(body), map[string]float64{}
			put := func(family string, m []string) {
				for i, resolution := range []string{"480p", "720p", "1080p"} {
					if i+1 < len(m) && m[i+1] != "" {
						if price, err := strconv.ParseFloat(m[i+1], 64); err == nil {
							out[family+":"+resolution] = price
						}
					}
				}
			}
			put("seedance-2.0", kiePattern.FindStringSubmatch(text))
			put("seedance-2.0-fast", kieFast.FindStringSubmatch(text))
			return out
		}},
	}
}

// videoPriceSources is replaced in tests so they never touch the network.
var videoPriceSources = defaultPriceSources()

var videoPriceRefresh sync.Mutex

func fetchPricePage(ctx context.Context, link string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; price-check)")
	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil || res.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return string(body), nil
}

// refreshVideoPrices reads every service's current price. It does nothing when the
// prices on hand are newer than videoPriceMaxAge, unless force is set. A source that
// fails keeps its last known price and is marked so the screen can say so.
func (s *Server) refreshVideoPrices(ctx context.Context, force bool) {
	videoPriceRefresh.Lock()
	defer videoPriceRefresh.Unlock()
	if at, err := s.database.GetAppSetting("video_prices_checked"); err == nil && !force {
		if t, e := time.Parse(time.RFC3339, at); e == nil && time.Since(t) < videoPriceMaxAge {
			return
		}
	}
	type result struct {
		source priceSource
		found  map[string]float64
	}
	results := make(chan result, len(videoPriceSources))
	for _, source := range videoPriceSources {
		go func(source priceSource) {
			found := map[string]float64{}
			if body, err := fetchPricePage(ctx, source.URL); err == nil {
				for key, price := range source.Parse(body) {
					// A price far outside what video costs means the page changed shape; ignore it.
					if price > 0.00001 && price < 5 {
						found[key] = price
					}
				}
			}
			results <- result{source, found}
		}(source)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ok, failed := map[string]bool{}, map[string]bool{}
	for range videoPriceSources {
		r := <-results
		for _, provider := range r.source.Providers {
			if len(r.found) == 0 {
				failed[provider] = true
				continue
			}
			ok[provider] = true
			for key, price := range r.found {
				setting := "video_price_"
				if !strings.Contains(key, ":") {
					setting = "video_rate_"
				}
				_ = s.database.SetAppSetting(setting+provider+":"+key, strconv.FormatFloat(price, 'f', -1, 64))
			}
		}
	}
	// fal.ai publishes its rate through an API that needs the saved key.
	if fal := costs.HostByName("fal"); fal != nil && s.routes().VideoConnected("fal") {
		var list struct {
			Prices []struct {
				Endpoint  string  `json:"endpoint_id"`
				UnitPrice float64 `json:"unit_price"`
				Unit      string  `json:"unit"`
			} `json:"prices"`
		}
		ids := []string{}
		for _, model := range fal.Models {
			ids = append(ids, model)
		}
		if s.falPlatform(ctx, "/models/pricing?endpoint_id="+url.QueryEscape(strings.Join(ids, ",")), &list) == nil {
			for _, p := range list.Prices {
				if family := costs.FamilyOf("fal", p.Endpoint); family != "" && p.UnitPrice > 0 && strings.Contains(p.Unit, "1000 tokens") {
					_ = s.database.SetAppSetting("video_rate_fal:"+family, strconv.FormatFloat(p.UnitPrice, 'f', -1, 64))
					ok["fal"] = true
				}
			}
		}
		if !ok["fal"] {
			failed["fal"] = true
		}
	}
	for provider := range ok {
		_ = s.database.SetAppSetting("video_checked_"+provider, now)
		_ = s.database.SetAppSetting("video_problem_"+provider, "")
	}
	for provider := range failed {
		if !ok[provider] {
			_ = s.database.SetAppSetting("video_problem_"+provider, "Could not read this service's price just now, so the last known price is shown.")
		}
	}
	_ = s.database.SetAppSetting("video_prices_checked", now)
}
