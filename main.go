package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	serpapi "github.com/serpapi/google-search-results-golang"
)

const ollamaEndpoint = "http://localhost:11434/api/generate"

type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type GenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func Search(text string) string {
	parameter := map[string]string{
		"q": text,
	}

	search := serpapi.NewGoogleSearch(parameter, os.Getenv("SERP_API_KEY"))
	data, err := search.GetJSON()
	if err != nil {
		panic(err)
	}
	// decode data and display
	results := data["organic_results"].([]interface{})
	firstResult := results[0].(map[string]interface{})
	title := firstResult["title"].(string)
	link := firstResult["link"].(string)
	snippet := firstResult["snippet"].(string)
	return fmt.Sprintf("Title: %s\nLink: %s\nSnippet: %s\n", title, link, snippet)
}

func parseOutput(text string) (functionName, input, reasoning string, err error) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Function:") {
			functionName = strings.TrimSpace(strings.TrimPrefix(line, "Function:"))
		} else if strings.HasPrefix(line, "Input:") {
			input = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "Input:")), "\"")
		} else if strings.HasPrefix(line, "Reasoning:") {
			reasoning = strings.TrimSpace(strings.TrimPrefix(line, "Reasoning:"))
		}
	}
	if functionName == "" || input == "" || reasoning == "" {
		err = fmt.Errorf("missing required fields in output")
	}
	return
}

func main() {
	userInput := "I live in Amsterdam. How is the weather today?"
	prompt := fmt.Sprintf(`Task: You are a functional agent which analyzes the input and decides the next step as a Function and input. Do not write anything else or you will fail. Strictly follow the given output.
Input: %s

Active Knowledge: 
%s

Functions:
- Function: Search
  Input: Search  Input Text
- Function: Browse
  Input: websiste url
- Function: Finish
  Input: Summarize findings and provide reasoning for the conclusion of the task.
Output should only include one function as the next step using the format:
Function: Function name
Input: Function Input as text
Reasoning: Reason to choose the Function and Input
`, userInput, "") //"Up-to-date data: Amsterdam weather today is rainy and windy. 5 degrees at night.")
	requestBody := &GenerateRequest{
		Model:  "zephyr:latest",
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", ollamaEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var generateResp GenerateResponse
	err = json.Unmarshal(body, &generateResp)
	if err != nil {
		panic(err)
	}

	// fmt.Println(generateResp.Response)
	// fmt.Println("Done:", generateResp.Done)
	functionName, input, reasoning, err := parseOutput(generateResp.Response)
	if err != nil {
		fmt.Println("Error parsing output:", err)
		return
	}

	fmt.Printf("Function Name: %s\nInput: %s\nReasoning: %s\n", functionName, input, reasoning)
	if functionName == "Search" {
		result := Search(input)
		fmt.Printf("Result:\n%s", result)
	}
}
