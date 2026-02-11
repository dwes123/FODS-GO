package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// TEST 1: Check the "Team" Post Type again
	checkEndpoint("Teams", "https://frontofficedynastysports.com/wp-json/wp/v2/team?per_page=5")

	fmt.Println("\n------------------------------------------------\n")

	// TEST 2: Check the "Users" (Owners)
	checkEndpoint("Users", "https://frontofficedynastysports.com/wp-json/wp/v2/users?per_page=5")
}

func checkEndpoint(label, url string) {
	fmt.Printf("🕵️  Checking %s Endpoint...\n    %s\n", label, url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("❌ Network Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("❌ API Blocked: Status %d (Unauthorized or Missing)\n", resp.StatusCode)
		return
	}

	body, _ := io.ReadAll(resp.Body)

	// Try parsing as a generic list of maps so we don't crash on different shapes
	var results []map[string]interface{}
	if err := json.Unmarshal(body, &results); err != nil {
		fmt.Println("⚠️  Could not parse JSON (Data might be empty or wrong format).")
		return
	}

	if len(results) == 0 {
		fmt.Println("❌ Result: Empty List (No items found).")
		return
	}

	fmt.Printf("✅ Result: Found %d items! Here is the first one:\n", len(results))
	first := results[0]

	// Try to find the name
	if title, ok := first["title"].(map[string]interface{}); ok {
		fmt.Printf("   Name: %v\n", title["rendered"])
	} else if name, ok := first["name"]; ok {
		fmt.Printf("   Name: %v\n", name)
	}

	// Dump keys to see if we spot "owner" or "league"
	fmt.Println("   Fields Available:")
	for k := range first {
		fmt.Printf("    - %s\n", k)
	}
}
