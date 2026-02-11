package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	wpUser := "djwes487@gmail.com"
	wpPass := "ab4H TPEh vyrc 9lOL T91Z Zt5L"
	auth := base64.StdEncoding.EncodeToString([]byte(wpUser + ":" + wpPass))

	url := "https://frontofficedynastysports.com/wp-json/wp/v2/settings"
	fmt.Printf("Fetching: %s\n", url)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Basic "+auth)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Response Status: %d\n", resp.StatusCode)
	fmt.Println(string(body))
}
