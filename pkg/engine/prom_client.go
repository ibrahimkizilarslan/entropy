package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ibrahimkizilarslan/entropy/pkg/config"
)

type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// EvaluateSteadyState checks the steady state probes against their sources.
func EvaluateSteadyState(probes []config.SteadyStateProbe) error {
	for _, probe := range probes {
		if probe.Source == "prometheus" {
			val, err := queryPrometheus(probe.Query)
			if err != nil {
				return fmt.Errorf("prometheus query failed for metric '%s': %w", probe.Metric, err)
			}

			if err := compareThreshold(val, probe.Threshold); err != nil {
				return fmt.Errorf("steady state violated for metric '%s': %w", probe.Metric, err)
			}
		} else {
			return fmt.Errorf("unsupported steady state source: %s", probe.Source)
		}
	}
	return nil
}

func queryPrometheus(query string) (float64, error) {
	promAddr := os.Getenv("PROMETHEUS_ADDR")
	if promAddr == "" {
		promAddr = "http://localhost:9090"
	}

	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", promAddr, url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned status: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var promResp PrometheusResponse
	if err := json.Unmarshal(body, &promResp); err != nil {
		return 0, err
	}

	if promResp.Status != "success" || len(promResp.Data.Result) == 0 {
		return 0, fmt.Errorf("no data returned from prometheus")
	}

	// Prometheus returns [timestamp, "value"]
	valStr, ok := promResp.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("invalid value format from prometheus")
	}

	return strconv.ParseFloat(valStr, 64)
}

func compareThreshold(value float64, threshold string) error {
	threshold = strings.TrimSpace(threshold)
	if threshold == "" {
		return nil
	}

	parts := strings.Fields(threshold)
	if len(parts) < 2 {
		// Fallback to simple comparison if only number is provided (assumes "<")
		target, err := strconv.ParseFloat(threshold, 64)
		if err != nil {
			return fmt.Errorf("invalid threshold format: %s", threshold)
		}
		if value >= target {
			return fmt.Errorf("value %f is not < %f", value, target)
		}
		return nil
	}

	operator := parts[0]
	target, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return fmt.Errorf("invalid threshold value: %s", parts[1])
	}

	switch operator {
	case "<":
		if value >= target {
			return fmt.Errorf("value %f is not < %f", value, target)
		}
	case "<=":
		if value > target {
			return fmt.Errorf("value %f is not <= %f", value, target)
		}
	case ">":
		if value <= target {
			return fmt.Errorf("value %f is not > %f", value, target)
		}
	case ">=":
		if value < target {
			return fmt.Errorf("value %f is not >= %f", value, target)
		}
	case "==":
		if value != target {
			return fmt.Errorf("value %f is not == %f", value, target)
		}
	default:
		return fmt.Errorf("unsupported operator: %s", operator)
	}

	return nil
}
