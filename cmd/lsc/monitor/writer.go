package monitor

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// MetricWriter handles writing metrics to JSONL or CSV files with buffering
type MetricWriter struct {
	file       *os.File
	bufWriter  *bufio.Writer
	csvWriter  *csv.Writer
	format     string
	mu         sync.Mutex
	headerDone bool
}

// NewMetricWriter creates a new metric writer
func NewMetricWriter(filepath string, format string) (*MetricWriter, error) {
	file, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}

	writer := &MetricWriter{
		file:      file,
		bufWriter: bufio.NewWriter(file),
		format:    format,
	}

	if format == "csv" {
		writer.csvWriter = csv.NewWriter(writer.bufWriter)
	}

	return writer, nil
}

// WriteJSON writes a JSON object as a single line (JSONL format)
func (w *MetricWriter) WriteJSON(data map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.format == "jsonl" {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		_, err = w.bufWriter.Write(jsonBytes)
		if err != nil {
			return err
		}
		_, err = w.bufWriter.WriteString("\n")
		return err
	} else if w.format == "csv" {
		// Write CSV header on first write
		if !w.headerDone {
			header := make([]string, 0, len(data))
			for key := range data {
				header = append(header, key)
			}
			if err := w.csvWriter.Write(header); err != nil {
				return err
			}
			w.headerDone = true
		}

		// Write CSV row
		row := make([]string, 0, len(data))
		for _, val := range data {
			row = append(row, fmt.Sprintf("%v", val))
		}
		return w.csvWriter.Write(row)
	}

	return fmt.Errorf("unknown format: %s", w.format)
}

// Flush flushes buffered data to disk
func (w *MetricWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.csvWriter != nil {
		w.csvWriter.Flush()
		if err := w.csvWriter.Error(); err != nil {
			return err
		}
	}

	return w.bufWriter.Flush()
}

// Close flushes and closes the file
func (w *MetricWriter) Close() error {
	if err := w.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}
