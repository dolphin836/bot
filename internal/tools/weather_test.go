package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dolphin836/bot/internal/tools"
)

func TestWeatherToolName(t *testing.T) {
	w := tools.NewWeatherTool(tools.WeatherConfig{
		DefaultCity:      "大阪市",
		DefaultLatitude:  34.6937,
		DefaultLongitude: 135.5023,
	})
	if w.Name() != "get_weather" {
		t.Errorf("name = %q, want %q", w.Name(), "get_weather")
	}
}

func TestWeatherToolDescription(t *testing.T) {
	w := tools.NewWeatherTool(tools.WeatherConfig{
		DefaultCity: "大阪市",
	})
	desc := w.Description()
	if !strings.Contains(desc, "大阪市") {
		t.Errorf("description should contain default city, got %q", desc)
	}
}

func TestWeatherToolSchema(t *testing.T) {
	w := tools.NewWeatherTool(tools.WeatherConfig{})
	var schema map[string]interface{}
	if err := json.Unmarshal(w.InputSchema(), &schema); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}
}

func TestWeatherToolDefaultCity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	w := tools.NewWeatherTool(tools.WeatherConfig{
		DefaultCity:      "大阪市",
		DefaultLatitude:  34.6937,
		DefaultLongitude: 135.5023,
	})

	result, err := w.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "大阪市") {
		t.Errorf("result should contain default city, got: %s", result)
	}
	if !strings.Contains(result, "°C") {
		t.Errorf("result should contain temperature, got: %s", result)
	}
}

func TestWeatherToolWithCity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	w := tools.NewWeatherTool(tools.WeatherConfig{
		DefaultCity:      "大阪市",
		DefaultLatitude:  34.6937,
		DefaultLongitude: 135.5023,
	})

	result, err := w.Execute(context.Background(), json.RawMessage(`{"city":"Tokyo"}`))
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "°C") {
		t.Errorf("result should contain temperature, got: %s", result)
	}
}
