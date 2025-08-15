package main

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestCheckUrl(t *testing.T) {
	address := "extim.su"
	timeout := time.Duration(time.Second * 4)
	_, err := net.DialTimeout("tcp", address+":80", timeout)
	if err != nil {
		fmt.Printf("[-] %v ", address)
		return
	} else {
		fmt.Printf("[+] %v ", address)
		return
	}
	panic("FAILED")
}

// TestCalculateGroupHealth tests the group health calculation functionality
func TestCalculateGroupHealth(t *testing.T) {
	tests := []struct {
		name           string
		groupName      string
		urlsWithGroups []URLWithGroup
		checkResults   map[string]bool
		expected       *GroupStatus
	}{
		{
			name:      "All URLs healthy",
			groupName: "test-group",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "test-group"},
				{URL: "url2.com", Group: "test-group"},
				{URL: "url3.com", Group: "test-group"},
			},
			checkResults: map[string]bool{
				"url1.com": true,
				"url2.com": true,
				"url3.com": true,
			},
			expected: &GroupStatus{
				GroupName:     "test-group",
				IsHealthy:     true,
				TotalURLs:     3,
				HealthyURLs:   3,
				UnhealthyURLs: 0,
				URLs:          []string{"url1.com", "url2.com", "url3.com"},
			},
		},
		{
			name:      "Some URLs unhealthy",
			groupName: "test-group",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "test-group"},
				{URL: "url2.com", Group: "test-group"},
				{URL: "url3.com", Group: "test-group"},
			},
			checkResults: map[string]bool{
				"url1.com": true,
				"url2.com": false,
				"url3.com": true,
			},
			expected: &GroupStatus{
				GroupName:     "test-group",
				IsHealthy:     false,
				TotalURLs:     3,
				HealthyURLs:   2,
				UnhealthyURLs: 1,
				URLs:          []string{"url1.com", "url2.com", "url3.com"},
			},
		},
		{
			name:      "All URLs unhealthy",
			groupName: "test-group",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "test-group"},
				{URL: "url2.com", Group: "test-group"},
			},
			checkResults: map[string]bool{
				"url1.com": false,
				"url2.com": false,
			},
			expected: &GroupStatus{
				GroupName:     "test-group",
				IsHealthy:     false,
				TotalURLs:     2,
				HealthyURLs:   0,
				UnhealthyURLs: 2,
				URLs:          []string{"url1.com", "url2.com"},
			},
		},
		{
			name:      "Empty group",
			groupName: "empty-group",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "other-group"},
				{URL: "url2.com", Group: "other-group"},
			},
			checkResults: map[string]bool{
				"url1.com": true,
				"url2.com": true,
			},
			expected: &GroupStatus{
				GroupName:     "empty-group",
				IsHealthy:     false,
				TotalURLs:     0,
				HealthyURLs:   0,
				UnhealthyURLs: 0,
				URLs:          []string{},
			},
		},
		{
			name:      "Mixed groups",
			groupName: "group1",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "group1"},
				{URL: "url2.com", Group: "group1"},
				{URL: "url3.com", Group: "group2"},
				{URL: "url4.com", Group: "group2"},
			},
			checkResults: map[string]bool{
				"url1.com": true,
				"url2.com": true,
				"url3.com": false,
				"url4.com": true,
			},
			expected: &GroupStatus{
				GroupName:     "group1",
				IsHealthy:     true,
				TotalURLs:     2,
				HealthyURLs:   2,
				UnhealthyURLs: 0,
				URLs:          []string{"url1.com", "url2.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateGroupHealth(tt.groupName, tt.urlsWithGroups, tt.checkResults)

			if result.GroupName != tt.expected.GroupName {
				t.Errorf("GroupName = %v, want %v", result.GroupName, tt.expected.GroupName)
			}
			if result.IsHealthy != tt.expected.IsHealthy {
				t.Errorf("IsHealthy = %v, want %v", result.IsHealthy, tt.expected.IsHealthy)
			}
			if result.TotalURLs != tt.expected.TotalURLs {
				t.Errorf("TotalURLs = %v, want %v", result.TotalURLs, tt.expected.TotalURLs)
			}
			if result.HealthyURLs != tt.expected.HealthyURLs {
				t.Errorf("HealthyURLs = %v, want %v", result.HealthyURLs, tt.expected.HealthyURLs)
			}
			if result.UnhealthyURLs != tt.expected.UnhealthyURLs {
				t.Errorf("UnhealthyURLs = %v, want %v", result.UnhealthyURLs, tt.expected.UnhealthyURLs)
			}
			if len(result.URLs) != len(tt.expected.URLs) {
				t.Errorf("URLs length = %v, want %v", len(result.URLs), len(tt.expected.URLs))
			}
		})
	}
}

// TestGetAllGroups tests the getAllGroups function
func TestGetAllGroups(t *testing.T) {
	tests := []struct {
		name           string
		urlsWithGroups []URLWithGroup
		expected       []string
	}{
		{
			name: "Multiple groups",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: "group1"},
				{URL: "url2.com", Group: "group1"},
				{URL: "url3.com", Group: "group2"},
				{URL: "url4.com", Group: "group2"},
				{URL: "url5.com", Group: "group3"},
			},
			expected: []string{"group1", "group2", "group3"},
		},
		{
			name: "Empty groups included",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: ""},
				{URL: "url2.com", Group: "group1"},
				{URL: "url3.com", Group: ""},
			},
			expected: []string{"", "group1"},
		},
		{
			name: "No groups",
			urlsWithGroups: []URLWithGroup{
				{URL: "url1.com", Group: ""},
				{URL: "url2.com", Group: ""},
			},
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAllGroups(tt.urlsWithGroups)

			if len(result) != len(tt.expected) {
				t.Errorf("Result length = %v, want %v", len(result), len(tt.expected))
			}

			// Check that all expected groups are present
			for _, expectedGroup := range tt.expected {
				found := false
				for _, resultGroup := range result {
					if resultGroup == expectedGroup {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected group %v not found in result", expectedGroup)
				}
			}
		})
	}
}

// TestImportFromFileWithGroups tests the file parsing functionality
func TestImportFromFileWithGroups(t *testing.T) {
	// Create a temporary test file
	testContent := `# Test file with group configuration
[group:web-servers]
google.com
github.com

[group:api-services]
api.github.com

# URLs without a group
example.com
test.com`

	// Write test content to a temporary file
	testFile := "test_groups_temp.txt"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	// Test the parsing
	result, err := importFromFileWithGroups(testFile)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// Verify the results
	expected := []URLWithGroup{
		{URL: "google.com", Group: "web-servers"},
		{URL: "github.com", Group: "web-servers"},
		{URL: "api.github.com", Group: "api-services"},
		{URL: "example.com", Group: "api-services"},
		{URL: "test.com", Group: "api-services"},
	}

	if len(result) != len(expected) {
		t.Errorf("Result length = %v, want %v", len(result), len(expected))
	}

	for i, expectedItem := range expected {
		if i >= len(result) {
			t.Errorf("Missing result item at index %d", i)
			continue
		}
		if result[i].URL != expectedItem.URL {
			t.Errorf("URL at index %d = %v, want %v", i, result[i].URL, expectedItem.URL)
		}
		if result[i].Group != expectedItem.Group {
			t.Errorf("Group at index %d = %v, want %v", i, result[i].Group, expectedItem.Group)
		}
	}
}

// TestNestedJSONStructure tests the nested JSON output functionality
func TestNestedJSONStructure(t *testing.T) {
	// Create test data
	urlsWithGroups := []URLWithGroup{
		{URL: "url1.com", Group: "group1"},
		{URL: "url2.com", Group: "group1"},
		{URL: "url3.com", Group: "group2"},
		{URL: "url4.com", Group: ""}, // ungrouped
	}

	checkResults := map[string]bool{
		"url1.com": true,
		"url2.com": false,
		"url3.com": true,
		"url4.com": true,
	}

	urlResults := map[string]*SearchResult{
		"url1.com": {Address: "url1.com", Port: "80", State: "Success", ResponseTime: 0.1, Group: "group1"},
		"url2.com": {Address: "url2.com", Port: "80", State: "Failed", ResponseTime: 0.2, Group: "group1"},
		"url3.com": {Address: "url3.com", Port: "80", State: "Success", ResponseTime: 0.3, Group: "group2"},
		"url4.com": {Address: "url4.com", Port: "80", State: "Success", ResponseTime: 0.4, Group: ""},
	}

	// Test the nested JSON output function
	outputNestedJSON(urlsWithGroups, checkResults, urlResults)

	// Note: This test primarily verifies that the function doesn't panic
	// In a real scenario, you might want to capture the output and verify the JSON structure
}

// TestCircuitBreaker tests the circuit breaker implementation
func TestCircuitBreaker(t *testing.T) {
	// Create a circuit breaker with threshold 3 and timeout 1 second
	cb := NewCircuitBreaker(3, 1*time.Second)

	// Initially should be closed
	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected initial state to be closed, got %v", cb.GetState())
	}

	// Record 2 failures - should still be closed
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to be closed after 2 failures, got %v", cb.GetState())
	}

	if cb.GetFailureCount() != 2 {
		t.Errorf("Expected failure count to be 2, got %d", cb.GetFailureCount())
	}

	// Record 3rd failure - should open
	cb.RecordFailure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open after 3 failures, got %v", cb.GetState())
	}

	// Should be open
	if !cb.IsOpen() {
		t.Errorf("Expected circuit to be open")
	}

	// Wait for timeout and check if it goes to half-open
	time.Sleep(1100 * time.Millisecond) // Wait slightly more than 1 second

	if cb.GetState() != CircuitHalfOpen {
		t.Errorf("Expected state to be half-open after timeout, got %v", cb.GetState())
	}

	// Should not be open in half-open state
	if cb.IsOpen() {
		t.Errorf("Expected circuit to not be open in half-open state")
	}

	// Record success - should go back to closed
	cb.RecordSuccess()

	if cb.GetState() != CircuitClosed {
		t.Errorf("Expected state to be closed after success, got %v", cb.GetState())
	}

	if cb.GetFailureCount() != 0 {
		t.Errorf("Expected failure count to be 0 after success, got %d", cb.GetFailureCount())
	}

	// Test half-open failure
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(1100 * time.Millisecond) // Wait for timeout

	if cb.GetState() != CircuitHalfOpen {
		t.Errorf("Expected state to be half-open after timeout, got %v", cb.GetState())
	}

	// Record failure in half-open state - should go back to open
	cb.RecordFailure()

	if cb.GetState() != CircuitOpen {
		t.Errorf("Expected state to be open after failure in half-open state, got %v", cb.GetState())
	}
}
