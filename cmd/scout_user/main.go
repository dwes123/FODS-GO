package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// Fetch one user (owner)
	url := "https://frontofficedynastysports.com/wp-json/wp/v2/users?per_page=1"
	fmt.Printf("🕵️  Investigating User Structure...\n    %s\n\n", url)

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var users []map[string]interface{}
	if err := json.Unmarshal(body, &users); err != nil {
		panic(err)
	}

	if len(users) == 0 {
		fmt.Println("No users found.")
		return
	}

	u := users[0]
	name := u["name"]

	fmt.Printf("👤 USER: %v\n", name)
	fmt.Println("------------------------------------------------")

	// Check for ACF Data (This is where the teams might be hiding!)
	if acf, ok := u["acf"].(map[string]interface{}); ok {
		fmt.Println("✅ HIDDEN FIELDS FOUND (ACF):")
		for key, val := range acf {
			fmt.Printf("   • %-20s : %v\n", key, val)
		}
	} else {
		fmt.Println("❌ No ACF data found on this user.")
	}
}
