package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadBody(t *testing.T) {
	// Test loading from string
	body, err := loadBody("test body")
	if err != nil {
		t.Fatalf("loadBody from string failed: %v", err)
	}
	if string(body) != "test body" {
		t.Errorf("expected 'test body', got %q", string(body))
	}

	// Test loading from file
	tmpFile := filepath.Join(t.TempDir(), "test-body.txt")
	content := "file body content"
	err = os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	body, err = loadBody("@" + tmpFile)
	if err != nil {
		t.Fatalf("loadBody from file failed: %v", err)
	}
	if string(body) != content {
		t.Errorf("expected %q, got %q", content, string(body))
	}

	// Test with empty string
	body, err = loadBody("")
	if err != nil || body != nil {
		t.Errorf("loadBody with empty string should return nil, nil")
	}

	// Test with non-existent file
	_, err = loadBody("@nonexistent.file")
	if err == nil {
		t.Error("loadBody with non-existent file should return error")
	}
}

func TestHeaderSlice(t *testing.T) {
	var h headerSlice

	// Test String()
	if h.String() != "" {
		t.Errorf("expected empty string, got %q", h.String())
	}

	// Test Set()
	err := h.Set("Content-Type: application/json")
	if err != nil {
		t.Fatalf("headerSlice.Set failed: %v", err)
	}

	err = h.Set("Authorization: Bearer token")
	if err != nil {
		t.Fatalf("headerSlice.Set failed: %v", err)
	}

	// Check result
	expected := []string{"Content-Type: application/json", "Authorization: Bearer token"}
	if !reflect.DeepEqual([]string(h), expected) {
		t.Errorf("expected %v, got %v", expected, h)
	}

	// Test String() with values
	expectedStr := "Content-Type: application/json; Authorization: Bearer token"
	if h.String() != expectedStr {
		t.Errorf("expected %q, got %q", expectedStr, h.String())
	}
}

func TestBytesReader(t *testing.T) {
	input := []byte("test data")
	reader := bytes.NewReader(input)

	// Test reading entire content
	buf := make([]byte, len(input))
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("bytesReader.Read failed: %v", err)
	}
	if n != len(input) {
		t.Errorf("expected to read %d bytes, got %d", len(input), n)
	}
	if string(buf) != "test data" {
		t.Errorf("expected %q, got %q", input, buf)
	}

	// Test reading past end
	n, err = reader.Read(buf)
	if err == nil || err.Error() != "EOF" {
		t.Errorf("expected EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected to read 0 bytes, got %d", n)
	}

	// Test partial read
	reader = bytes.NewReader(input)
	smallBuf := make([]byte, 4)
	n, err = reader.Read(smallBuf)
	if err != nil {
		t.Fatalf("bytesReader.Read failed: %v", err)
	}
	if n != 4 {
		t.Errorf("expected to read 4 bytes, got %d", n)
	}
	if string(smallBuf) != "test" {
		t.Errorf("expected %q, got %q", "test", smallBuf)
	}
}

func TestResultSetAddAndGet(t *testing.T) {
	rs := &resultSet{}

	testRecords := []record{
		{latency: 100 * time.Millisecond, status: 200, size: 100, failed: false},
		{latency: 200 * time.Millisecond, status: 404, size: 50, failed: false},
		{latency: 0, status: 0, size: 0, failed: true, errMsg: "timeout"},
	}

	for _, rec := range testRecords {
		rs.add(rec)
	}

	if len(rs.records) != len(testRecords) {
		t.Errorf("expected %d records, got %d", len(testRecords), len(rs.records))
	}

	for i, expected := range testRecords {
		if rs.records[i].status != expected.status ||
			rs.records[i].failed != expected.failed ||
			rs.records[i].latency != expected.latency {
			t.Errorf("record %d mismatch: got %+v, want %+v", i, rs.records[i], expected)
		}
	}
}

func TestResultSetSummarize(t *testing.T) {
	// This is primarily a visual output function, so we'll verify it doesn't crash with test data
	// and basic output validation by capturing stdout

	// Setup: capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	rs := &resultSet{
		start: time.Now().Add(-1 * time.Second),
		end:   time.Now(),
		records: []record{
			{latency: 100 * time.Millisecond, status: 200, size: 100, failed: false},
			{latency: 150 * time.Millisecond, status: 200, size: 150, failed: false},
			{latency: 200 * time.Millisecond, status: 200, size: 200, failed: false},
			{latency: 0, status: 0, size: 0, failed: true, errMsg: "timeout"},
			{latency: 300 * time.Millisecond, status: 404, size: 50, failed: false},
		},
	}

	rs.summarize()

	// Close pipe and restore stdout
	_ = w.Close()
	os.Stdout = oldStdout

	// Read captured output
	output, _ := io.ReadAll(r)
	outputStr := string(output)

	// Verify basic output components
	if !strings.Contains(outputStr, "Summary:") {
		t.Error("Missing 'Summary:' in output")
	}

	if !strings.Contains(outputStr, "Latency distribution:") {
		t.Error("Missing 'Latency distribution:' in output")
	}

	if !strings.Contains(outputStr, "Status code distribution:") {
		t.Error("Missing 'Status code distribution:' in output")
	}

	if !strings.Contains(outputStr, "[200]") || !strings.Contains(outputStr, "[404]") {
		t.Error("Missing status code counts in output")
	}
}
