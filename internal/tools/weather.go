package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WMO weather code descriptions
var weatherCodes = map[int]string{
	0:  "晴天",
	1:  "大部晴朗",
	2:  "多云",
	3:  "阴天",
	45: "雾",
	48: "雾凇",
	51: "小毛毛雨",
	53: "毛毛雨",
	55: "大毛毛雨",
	61: "小雨",
	63: "中雨",
	65: "大雨",
	66: "小冻雨",
	67: "大冻雨",
	71: "小雪",
	73: "中雪",
	75: "大雪",
	77: "雪粒",
	80: "小阵雨",
	81: "中阵雨",
	82: "大阵雨",
	85: "小阵雪",
	86: "大阵雪",
	95: "雷暴",
	96: "雷暴伴小冰雹",
	99: "雷暴伴大冰雹",
}

type WeatherTool struct {
	defaultCity      string
	defaultLatitude  float64
	defaultLongitude float64
}

type WeatherConfig struct {
	DefaultCity      string
	DefaultLatitude  float64
	DefaultLongitude float64
}

func NewWeatherTool(cfg WeatherConfig) *WeatherTool {
	return &WeatherTool{
		defaultCity:      cfg.DefaultCity,
		defaultLatitude:  cfg.DefaultLatitude,
		defaultLongitude: cfg.DefaultLongitude,
	}
}

func (w *WeatherTool) Name() string { return "get_weather" }

func (w *WeatherTool) Description() string {
	return fmt.Sprintf(
		"Get current weather and forecast for a city. If no city is specified, uses the default city (%s). "+
			"Returns current temperature, conditions, humidity, wind, and a 3-day forecast.",
		w.defaultCity,
	)
}

func (w *WeatherTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"city": {
				"type": "string",
				"description": "City name (e.g. 'Tokyo', 'Osaka'). Leave empty to use default city."
			}
		}
	}`)
}

type weatherInput struct {
	City string `json:"city"`
}

type geoResult struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
}

type geoResponse struct {
	Results []geoResult `json:"results"`
}

type openMeteoResponse struct {
	Current struct {
		Temperature   float64 `json:"temperature_2m"`
		Humidity      int     `json:"relative_humidity_2m"`
		ApparentTemp  float64 `json:"apparent_temperature"`
		WeatherCode   int     `json:"weather_code"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		Precipitation float64 `json:"precipitation"`
	} `json:"current"`
	Daily struct {
		Time         []string  `json:"time"`
		TempMax      []float64 `json:"temperature_2m_max"`
		TempMin      []float64 `json:"temperature_2m_min"`
		WeatherCode  []int     `json:"weather_code"`
		PrecipSum    []float64 `json:"precipitation_sum"`
		PrecipProb   []int     `json:"precipitation_probability_max"`
	} `json:"daily"`
}

func (w *WeatherTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in weatherInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	city := strings.TrimSpace(in.City)
	lat, lon := w.defaultLatitude, w.defaultLongitude
	displayCity := w.defaultCity

	if city != "" {
		geoLat, geoLon, resolvedCity, err := geocode(ctx, city)
		if err != nil {
			return "", fmt.Errorf("geocode %q: %w", city, err)
		}
		lat, lon = geoLat, geoLon
		displayCity = resolvedCity
	}

	weather, err := fetchWeather(ctx, lat, lon)
	if err != nil {
		return "", fmt.Errorf("fetch weather: %w", err)
	}

	return formatWeather(displayCity, weather), nil
}

func geocode(ctx context.Context, city string) (float64, float64, string, error) {
	u := fmt.Sprintf(
		"https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=zh&format=json",
		url.QueryEscape(city),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, "", err
	}

	var geo geoResponse
	if err := json.Unmarshal(body, &geo); err != nil {
		return 0, 0, "", err
	}

	if len(geo.Results) == 0 {
		return 0, 0, "", fmt.Errorf("city %q not found", city)
	}

	r := geo.Results[0]
	name := r.Name
	if r.Country != "" {
		name = r.Name + ", " + r.Country
	}
	return r.Latitude, r.Longitude, name, nil
}

func fetchWeather(ctx context.Context, lat, lon float64) (*openMeteoResponse, error) {
	u := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&current=temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m,precipitation"+
			"&daily=temperature_2m_max,temperature_2m_min,weather_code,precipitation_sum,precipitation_probability_max"+
			"&timezone=auto&forecast_days=3",
		lat, lon,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned %d: %s", resp.StatusCode, string(body))
	}

	var result openMeteoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func weatherCodeDesc(code int) string {
	if desc, ok := weatherCodes[code]; ok {
		return desc
	}
	return fmt.Sprintf("未知(%d)", code)
}

func formatWeather(city string, w *openMeteoResponse) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📍 %s 天气\n\n", city))
	sb.WriteString(fmt.Sprintf("🌡 当前: %.1f°C (体感 %.1f°C)\n", w.Current.Temperature, w.Current.ApparentTemp))
	sb.WriteString(fmt.Sprintf("☁ 天气: %s\n", weatherCodeDesc(w.Current.WeatherCode)))
	sb.WriteString(fmt.Sprintf("💧 湿度: %d%%\n", w.Current.Humidity))
	sb.WriteString(fmt.Sprintf("💨 风速: %.1f km/h\n", w.Current.WindSpeed))
	if w.Current.Precipitation > 0 {
		sb.WriteString(fmt.Sprintf("🌧 降水: %.1f mm\n", w.Current.Precipitation))
	}

	if len(w.Daily.Time) > 0 {
		sb.WriteString("\n📅 未来三天:\n")
		for i, date := range w.Daily.Time {
			sb.WriteString(fmt.Sprintf("  %s: %s, %.0f~%.0f°C",
				date,
				weatherCodeDesc(w.Daily.WeatherCode[i]),
				w.Daily.TempMin[i],
				w.Daily.TempMax[i],
			))
			if w.Daily.PrecipProb[i] > 0 {
				sb.WriteString(fmt.Sprintf(", 降水概率 %d%%", w.Daily.PrecipProb[i]))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
