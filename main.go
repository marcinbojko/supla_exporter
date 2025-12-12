package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"supla_exporter/config"
	"supla_exporter/metrics"
	"supla_exporter/parser"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func updateMetrics(devices []config.Device, cfg *config.Config) {
	// Reset device count before updating
	parser.GetAndResetDeviceCount() // This resets the count to 0

	// Use worker pool instead of sequential processing
	numWorkers := cfg.Global.Workers
	results := parser.FetchAndParseWithPool(devices, numWorkers)

	// Process results and update metrics
	for _, info := range results {
		if info != nil {
			metrics.UpdateMetrics(info)
		}
	}
	// for _, device := range devices {
	// 	info, err := parser.FetchAndParse(device)
	// 	if err != nil {
	// 		slog.Error("Error fetching data from device",
	// 			"url", device.URL,
	// 			"error", err,
	// 		)
	// 		continue
	// 	}
	// 	metrics.UpdateMetrics(info)
	// }

	// Get and log the device count
	deviceCount := parser.GetAndResetDeviceCount()
	slog.Debug("Metrics update completed", "devices_processed", deviceCount)
}

func getLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	// Define config paths to try
	configPaths := []string{
		*configFile,          // CLI argument (first priority)
		"config/config.yaml", // Default path
		"./config.yaml",      // Current directory
		"../config.yaml",     // Parent directory
	}

	var cfg *config.Config
	var err error
	// Load configuration
	for _, path := range configPaths {
		// Check if file exists
		if _, err := os.Stat(path); err != nil {
			slog.Debug("Config file not accessible",
				"path", path,
				"error", err,
			)
			continue
		}
		cfg, err = config.LoadConfig(path)
		if err != nil {
			slog.Error("Error loading config",
				"error", err,
				"tried_paths", configPaths,
			)
			os.Exit(1)
		}
		config.Set(cfg)
	}

	// Set log level based on configuration
	logLevel := getLogLevel(cfg.Global.LogLevel)
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	slog.Info("Log level set", "level", cfg.Global.LogLevel)
	slog.Debug("This is a debug message to verify log level")

	// Initial metrics update
	updateMetrics(cfg.Devices, cfg)

	// Start periodic updates with configured interval
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.Global.Interval) * time.Second)
		for range ticker.C {
			updateMetrics(cfg.Devices, cfg)
		}
	}()

	// Start metrics server on configured port
	addr := fmt.Sprintf(":%d", cfg.Global.Port)
	slog.Info("Starting metrics server",
		"address", addr,
	)
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("Failed to start metrics server",
			"address", addr,
			"error", err,
		)
		os.Exit(1)
	}
}
