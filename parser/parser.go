package parser

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"supla_exporter/config"

	"github.com/PuerkitoBio/goquery"
)

type SuplaInfo struct {
	Name     string  `json:"name"`     // Device name from HTML
	State    string  `json:"state"`    // Device state (Zarejestrowany i gotowy)
	Firmware string  `json:"firmware"` // Firmware version (GG v24.09.24a)
	GUID     string  `json:"guid"`     // Device ID
	MAC      string  `json:"mac"`      // MAC address
	Mode     string  `json:"mode"`     // Operating mode (NORMAL)
	FreeMem  float64 `json:"free_mem"` // Memory in KB (28.34)
	Up       bool    `json:"up"`       // Device is up (true/false)
	URL      string  `json:"url"`      // Device URL
}

var deviceCount int64

// LogValue implements the slog.LogValuer interface
func (si *SuplaInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", si.Name),
		slog.String("url", si.URL),
		slog.String("firmware", si.Firmware),
		slog.Bool("up", si.Up),
	)
}

// FetchAndParseWithPool processes multiple devices using a worker pool
func FetchAndParseWithPool(devices []config.Device, numWorkers int) []*SuplaInfo {

	// Create channels for jobs and results
	jobs := make(chan config.Device, len(devices))
	results := make(chan *SuplaInfo, len(devices))

	// Start workers
	for w := 0; w < numWorkers; w++ {
		go worker(jobs, results)
	}

	// Send jobs to workers
	for _, device := range devices {
		jobs <- device
	}
	close(jobs)

	// Collect results
	var infos []*SuplaInfo
	for i := 0; i < len(devices); i++ {
		info := <-results
		infos = append(infos, info)
	}

	return infos
}

// worker processes jobs from jobs channel and sends results to results channel
func worker(jobs <-chan config.Device, results chan<- *SuplaInfo) {
	for device := range jobs {
		info, err := FetchAndParse(device)
		if err != nil {
			// In case of error, send a basic error info
			results <- &SuplaInfo{
				URL:   device.URL,
				Up:    false,
				State: fmt.Sprintf("Error: %v", err),
			}
			continue
		}
		results <- info
	}
}

// FetchAndParse gets content from URL with basic auth and parses it
func FetchAndParse(device config.Device) (*SuplaInfo, error) {

	// Increment device count (regardless of a status)
	atomic.AddInt64(&deviceCount, 1)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: config.GetTimeout(),
	}

	// Create request
	req, err := http.NewRequest("GET", device.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add basic auth
	req.SetBasicAuth(device.Username, device.Password)

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		// Connection/timeout error
		return &SuplaInfo{
			URL: device.URL,
			Up:  false,
		}, nil
	}
	defer resp.Body.Close()

	// Check status code
	switch resp.StatusCode {
	case http.StatusOK: // 200
		// All good, continue with parsing

	case http.StatusUnauthorized: // 401
		return &SuplaInfo{
			URL:   device.URL,
			Up:    false,
			State: "Unauthorized - check credentials",
		}, nil

	case http.StatusForbidden: // 403
		return &SuplaInfo{
			URL:   device.URL,
			Up:    false,
			State: "Forbidden - access denied",
		}, nil

	case http.StatusNotFound: // 404
		return &SuplaInfo{
			URL:   device.URL,
			Up:    false,
			State: "Not found - check URL",
		}, nil

	case http.StatusInternalServerError: // 500
		return &SuplaInfo{
			URL:   device.URL,
			Up:    false,
			State: "Internal server error",
		}, nil

	default:
		return &SuplaInfo{
			Name:  device.URL,
			URL:   device.URL,
			Up:    false,
			State: fmt.Sprintf("HTTP error %d", resp.StatusCode),
		}, nil
	}
	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// If it's a chunked encoding error, try to read the body differently
		if strings.Contains(err.Error(), "malformed chunked encoding") {
			slog.Warn("Encountered malformed chunked encoding, attempting alternative read method",
				"url", device.URL)

			// Discard the current body
			io.Copy(io.Discard, resp.Body)

			// Make a new request without accepting chunked responses
			req.Header.Set("Accept-Encoding", "identity")
			resp, err = client.Do(req)
			if err != nil {
				return &SuplaInfo{
					URL:   device.URL,
					Up:    false,
					State: "Timeout",
					Mode:  "ERROR",
				}, nil
			}
			defer resp.Body.Close()

			// Try to read the body again
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				return &SuplaInfo{
					URL:   device.URL,
					Up:    false,
					State: "Timeout",
					Mode:  "ERROR",
				}, nil
			}
		} else {
			return &SuplaInfo{
				URL:   device.URL,
				Up:    false,
				State: "Timeout",
				Mode:  "ERROR",
			}, nil
		}
	}

	// Parse HTML and set device as up
	info, err := ParseHTML(string(body))
	if err != nil {
		return &SuplaInfo{
			URL:   device.URL,
			Up:    false,
			State: fmt.Sprintf("Error parsing HTML: %v", err),
		}, nil
	}
	info.URL = device.URL
	info.Up = true

	// Convert debug output to use slog
	// debugJSON, _ := json.MarshalIndent(info, "", " ")
	slog.Debug("Parsed device info",
		"device", info)
	return info, nil
}

func ParseHTML(content string) (*SuplaInfo, error) {

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %w", err)
	}

	info := &SuplaInfo{}

	// Get device name
	info.Name = doc.Find("h1").Text()

	doc.Find("span").Each(func(i int, s *goquery.Selection) {
		text := s.Text()

		if strings.Contains(text, "LAST STATE:") {
			// Define sections in order
			sections := []string{"LAST STATE:", "Firmware:", "GUID:", "MAC:"}

			// Split text into sections
			for i, section := range sections {
				if i < len(sections)-1 {
					// Get text between current section and next section
					nextSection := sections[i+1]
					value := strings.Split(strings.Split(text, section)[1], nextSection)[0]
					value = strings.TrimSpace(value)

					switch section {
					case "LAST STATE:":
						info.State = value
					case "Firmware:":
						info.Firmware = value
						re := regexp.MustCompile(`v\d`)
						if loc := re.FindStringIndex(info.Firmware); loc != nil {
							info.Firmware = info.Firmware[loc[0]:]
						}
					case "GUID:":
						info.GUID = value
					}
				} else {
					// Last section (MAC)
					if parts := strings.Split(text, section); len(parts) > 1 {
						macPart := strings.TrimSpace(parts[1])
						// Use regex to extract MAC address
						macRegex := regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`)
						if match := macRegex.FindString(macPart); match != "" {
							info.MAC = match
						}
					}
				}
			}
		}

		if strings.Contains(text, "Free Mem:") {
			// Get free memory value
			parts := strings.Split(text, "Free Mem:")[1] // Get everything after "Free Mem:"
			memAndMode := strings.Split(parts, "Mode:")  // Split into [" 28.34kB ", " NORMAL"]

			// Handle free memory
			memStr := regexp.MustCompile(`(?i)kb?$`).ReplaceAllString(strings.TrimSpace(memAndMode[0]), "")
			if freeMem, err := strconv.ParseFloat(memStr, 64); err == nil {
				info.FreeMem = freeMem
			}

			// Handle mode
			if len(memAndMode) > 1 {
				info.Mode = strings.TrimSpace(memAndMode[1])
			}
		}
	})

	return info, nil
}

// GetAndResetDeviceCount returns the current count of devices and resets it to 0
func GetAndResetDeviceCount() int64 {
	return atomic.SwapInt64(&deviceCount, 0)
}

// GetDeviceCount returns the current count of devices without resetting it
func GetDeviceCount() int64 {
	count := atomic.LoadInt64(&deviceCount)
	slog.Debug("Device count retrieved", "device_count", count)
	return count
}
