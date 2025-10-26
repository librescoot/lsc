package monitor

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"librescoot/lsc/internal/redis"
)

// recordGPS records GPS coordinates and speed
func recordGPS(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "gps."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := RedisClient.HGetAll("gps:filtered")
			if err != nil || len(data) == 0 {
				continue
			}

			record := map[string]interface{}{
				"timestamp": time.Now().UnixMilli(),
			}

			// Add GPS fields
			for key, val := range data {
				// Convert numeric fields
				if key == "lat" || key == "lon" || key == "speed" || key == "heading" || key == "altitude" {
					if f, err := strconv.ParseFloat(val, 64); err == nil {
						record[key] = f
					}
				} else {
					record[key] = val
				}
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}
		}
	}
}

// recordBattery records battery metrics for all connected batteries
func recordBattery(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	// Create writers for each battery (0 and 1)
	writers := make(map[int]*MetricWriter)
	defer func() {
		for _, w := range writers {
			w.Close()
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check both battery:0 and battery:1
			for id := 0; id <= 1; id++ {
				key := "battery:" + strconv.Itoa(id)
				data, err := RedisClient.HGetAll(key)
				if err != nil || len(data) == 0 {
					continue
				}

				// Create writer on first successful read
				if writers[id] == nil {
					filename := "battery-" + strconv.Itoa(id) + "." + monitorFormat
					w, err := NewMetricWriter(filepath.Join(outputDir, filename), monitorFormat)
					if err != nil {
						continue
					}
					writers[id] = w
				}

				record := map[string]interface{}{
					"timestamp":   time.Now().UnixMilli(),
					"battery_id":  id,
				}

				// Add battery fields with type conversion
				for key, val := range data {
					switch key {
					case "soc", "voltage", "current", "temperature":
						if f, err := strconv.ParseFloat(val, 64); err == nil {
							record[key] = f
						}
					default:
						record[key] = val
					}
				}

				if err := writers[id].WriteJSON(record); err == nil {
					mu.Lock()
					*count++
					mu.Unlock()
				}
			}
		}
	}
}

// recordVehicle records vehicle state changes
func recordVehicle(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "vehicle."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := RedisClient.HGetAll("vehicle")
			if err != nil || len(data) == 0 {
				continue
			}

			record := map[string]interface{}{
				"timestamp": time.Now().UnixMilli(),
			}

			// Add all vehicle fields
			for key, val := range data {
				record[key] = val
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}
		}
	}
}

// recordMotor records motor/ECU metrics
func recordMotor(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "motor."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := RedisClient.HGetAll("engine-ecu")
			if err != nil || len(data) == 0 {
				continue
			}

			record := map[string]interface{}{
				"timestamp": time.Now().UnixMilli(),
			}

			// Add motor fields with type conversion
			for key, val := range data {
				switch key {
				case "rpm", "speed", "odometer", "temperature":
					if f, err := strconv.ParseFloat(val, 64); err == nil {
						record[key] = f
					}
				case "motor:voltage", "motor:current":
					if f, err := strconv.ParseFloat(val, 64); err == nil {
						// Clean up key name
						cleanKey := strings.ReplaceAll(key, ":", "_")
						record[cleanKey] = f
					}
				default:
					record[key] = val
				}
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}
		}
	}
}

// recordPower records power manager metrics
func recordPower(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "power."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := RedisClient.HGetAll("power-manager")
			if err != nil || len(data) == 0 {
				continue
			}

			record := map[string]interface{}{
				"timestamp": time.Now().UnixMilli(),
			}

			// Add power manager fields
			for key, val := range data {
				if key == "uptime" {
					if i, err := strconv.ParseInt(val, 10, 64); err == nil {
						record[key] = i
					}
				} else {
					record[key] = val
				}
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}
		}
	}
}

// recordModem records modem and internet connectivity metrics
func recordModem(ctx context.Context, wg *sync.WaitGroup, outputDir string, interval time.Duration, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "modem."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get modem data
			modemData, err := RedisClient.HGetAll("modem")
			if err != nil {
				modemData = make(map[string]string)
			}

			// Get internet data
			internetData, err := RedisClient.HGetAll("internet")
			if err != nil {
				internetData = make(map[string]string)
			}

			// Skip if both empty
			if len(modemData) == 0 && len(internetData) == 0 {
				continue
			}

			record := map[string]interface{}{
				"timestamp": time.Now().UnixMilli(),
			}

			// Add modem fields with prefix
			for key, val := range modemData {
				record["modem_"+key] = val
			}

			// Add internet fields with prefix
			for key, val := range internetData {
				record["internet_"+key] = val
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}
		}
	}
}

// recordEvents records fault events from the stream
func recordEvents(ctx context.Context, wg *sync.WaitGroup, outputDir string, count *int, mu *sync.Mutex) {
	defer wg.Done()

	writer, err := NewMetricWriter(filepath.Join(outputDir, "events."+monitorFormat), monitorFormat)
	if err != nil {
		return
	}
	defer writer.Close()

	lastID := "$" // Start from latest

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read with blocking
		streams, err := RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{"events:faults", lastID},
			Count:   10,
			Block:   1 * time.Second,
		})

		if err != nil {
			// Timeout is normal in block mode
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "nil") {
				continue
			}
			return
		}

		if len(streams) == 0 || len(streams[0].Messages) == 0 {
			continue
		}

		// Process events
		for _, msg := range streams[0].Messages {
			// Parse timestamp from message ID
			idParts := strings.Split(msg.ID, "-")
			var timestamp int64
			if len(idParts) > 0 {
				timestamp, _ = strconv.ParseInt(idParts[0], 10, 64)
			}

			record := map[string]interface{}{
				"timestamp": timestamp,
				"id":        msg.ID,
			}

			// Add event fields
			for key, val := range msg.Values {
				record[key] = val
			}

			if err := writer.WriteJSON(record); err == nil {
				mu.Lock()
				*count++
				mu.Unlock()
			}

			lastID = msg.ID
		}
	}
}
