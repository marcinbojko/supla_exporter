package metrics

import (
	"log/slog"
	"supla_exporter/parser"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	ErrorValue   = "ERROR"
	UnknownValue = "UNKNOWN"
)

var (
	// Base device info - identity and static info
	SuplaDeviceInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_info",
			Help: "Base device information indicating device presence",
		},
		[]string{"url", "name"},
	)

	// Separate state metric
	SuplaDeviceState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_state",
			Help: "Device operational state",
		},
		[]string{"url", "state"},
	)

	// Network information
	SuplaDeviceNetwork = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_network",
			Help: "Device network information",
		},
		[]string{"url", "mac"},
	)

	// Firmware information
	SuplaDeviceFirmware = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_firmware",
			Help: "Device firmware version",
		},
		[]string{"url", "firmware", "name"},
	)

	// Memory metric (renamed to be clearer)
	SuplaDeviceMemory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_memory_free_bytes",
			Help: "Free memory in bytes",
		},
		[]string{"url", "name"},
	)

	// Availability status (unchanged)
	SuplaDeviceUp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "supla_device_up",
			Help: "Device availability status (1=up, 0=down)",
		},
		[]string{"url"},
	)

	// Total count (unchanged)
	SuplaDeviceCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "supla_device_count",
			Help: "Total number of Supla devices",
		},
	)
)

func UpdateMetrics(info *parser.SuplaInfo) {
	// Convert bool to float64 (1.0 = up, 0.0 = down)
	upValue := 0.0
	if info.Up {
		upValue = 1.0
	}

	// Update device up/down status metric (always emitted, never deleted)
	SuplaDeviceUp.WithLabelValues(info.URL).Set(upValue)

	if upValue == 0 {
		slog.Debug("Device not present: ", "url", info.URL)
		return
	}

	// Clean existing metrics for this device (only when we have fresh data to replace them)
	SuplaDeviceInfo.DeletePartialMatch(prometheus.Labels{"url": info.URL})
	SuplaDeviceState.DeletePartialMatch(prometheus.Labels{"url": info.URL})
	SuplaDeviceNetwork.DeletePartialMatch(prometheus.Labels{"url": info.URL})
	SuplaDeviceFirmware.DeletePartialMatch(prometheus.Labels{"url": info.URL})
	SuplaDeviceMemory.DeletePartialMatch(prometheus.Labels{"url": info.URL})

	// Base device info
	SuplaDeviceInfo.WithLabelValues(
		info.URL,
		getOrDefault(info.Name, UnknownValue),
	).Set(1)

	// Device state
	SuplaDeviceState.WithLabelValues(
		info.URL,
		getOrDefault(info.State, UnknownValue),
	).Set(1)

	// Network info
	SuplaDeviceNetwork.WithLabelValues(
		info.URL,
		getOrDefault(info.MAC, UnknownValue),
	).Set(1)

	// Firmware info
	SuplaDeviceFirmware.WithLabelValues(
		info.URL,
		getOrDefault(info.Firmware, UnknownValue),
		getOrDefault(info.Name, UnknownValue),
	).Set(1)

	// Memory (converted to bytes)
	if info.FreeMem > 0 {
		SuplaDeviceMemory.WithLabelValues(
			info.URL,
			getOrDefault(info.Name, UnknownValue),
		).Set(info.FreeMem) // Converting KB to bytes
	}

	// Update device count
	SuplaDeviceCount.Set(float64(parser.GetDeviceCount()))
}

// getOrDefault returns the value if it's not empty, otherwise returns the default value
func getOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
