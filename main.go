package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
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

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

func Search(text string) string {
	parameter := map[string]string{
		"q": text,
	}

	serpAPIKey := os.Getenv("SERP_API_KEY")
	if serpAPIKey == "" {
		log.Fatal("SERP_API_KEY is not set in the environment")
	}

	search := serpapi.NewGoogleSearch(parameter, serpAPIKey)
	data, err := search.GetJSON()
	if err != nil {
		panic(err)
	}

	results := data["organic_results"].([]interface{})
	firstResult := results[0].(map[string]interface{})
	title := firstResult["title"].(string)
	link := firstResult["link"].(string)
	snippet := firstResult["snippet"].(string)
	return fmt.Sprintf("Title: %s\nLink: %s\nSnippet: %s\n", title, link, snippet)
}

func Browse(url string) string {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var title, description string
	var divNodes, pNodes, spanNodes, linkNodes []*cdp.Node

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Title(&title),
		chromedp.AttributeValue(`meta[name="description"]`, "content", &description, nil),
		chromedp.Nodes(`div`, &divNodes, chromedp.ByQueryAll, chromedp.AtLeast(0)),
		chromedp.Nodes(`p`, &pNodes, chromedp.ByQueryAll, chromedp.AtLeast(0)),
		chromedp.Nodes(`span`, &spanNodes, chromedp.ByQueryAll, chromedp.AtLeast(0)),
		chromedp.Nodes(`a`, &linkNodes, chromedp.ByQueryAll, chromedp.AtLeast(0)),
	)
	if err != nil {
		log.Fatal(err)
	}

	divTexts, _ := extractTexts(ctx, "div")
	pTexts, _ := extractTexts(ctx, "p")
	spanTexts, _ := extractTexts(ctx, "span")
	links := extractHrefs(linkNodes)

	return fmt.Sprintf("Title: %s\nDescription: %s\n\nDiv Texts:\n%s\n\nP Texts:\n%s\n\nSpan Texts:\n%s\n\nLinks:\n%s",
		title, description,
		strings.Join(divTexts, "\n"),
		strings.Join(pTexts, "\n"),
		strings.Join(spanTexts, "\n"),
		strings.Join(links, "\n"),
	)
}

func extractTexts(ctx context.Context, selector string) ([]string, error) {
	var texts []string
	var nodes []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Nodes(selector, &nodes, chromedp.ByQueryAll, chromedp.AtLeast(0)))
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		var text string
		err = chromedp.Run(ctx, chromedp.Text(n.FullXPath(), &text, chromedp.NodeVisible, chromedp.BySearch))
		if err == nil {
			texts = append(texts, text)
		}
	}
	return texts, nil
}

func extractHrefs(nodes []*cdp.Node) []string {
	var hrefs []string
	for _, n := range nodes {
		if href := n.AttributeValue("href"); href != "" {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs
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
		err = fmt.Errorf("missing required fields in output. Output %s", text)
	}
	return
}

func createPrompt(userInput string, knowledge []string) string {
	knowledge_str := strings.Join(knowledge, "\n")
	return fmt.Sprintf(`Task: You are a functional agent which analyzes the input and decides the next step as a Function and input. Do not write anything else or you will fail. Strictly follow the given output.
Input: %s
Active Knowledge: 
%s
Functions:
- Function: Search
  Description: This search is useful to get reliable quick data.
  Input: Search  Input Text
- Function: Browse
  Description: This browse is useful when users want to get content of a page.
  Input: websiste url
- Function: Finish
  Description: This is useful when the agent decides to finish this task.
  Input: Summarize findings and provide reasoning for the conclusion of the task.
Output should only include one function as the next step using the format:
Function: Function name
Input: Function Input as text
Reasoning: Reason to choose the Function and Input
`, userInput, knowledge_str)
}

func callAPI(prompt string) (*GenerateResponse, error) {
	requestBody := &GenerateRequest{
		Model:  "zephyr:latest",
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", ollamaEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var generateResp GenerateResponse
	err = json.Unmarshal(body, &generateResp)
	if err != nil {
		return nil, err
	}

	return &generateResp, nil
}

func main() {
	loadEnv()

	userInput := "I live in Amsterdam. How is the weather today?"
	knowledge := []string{}
	var functionName, input, reasoning string
	var generateResp *GenerateResponse
	var err error

	for functionName != "Final" {
		prompt := createPrompt(userInput, knowledge)

		generateResp, err = callAPI(prompt)
		if err != nil {
			fmt.Println("Error calling API:", err)
			return
		}

		functionName, input, reasoning, err = parseOutput(generateResp.Response)
		if err != nil {
			fmt.Println("Error parsing output:", err)
			return
		}

		fmt.Printf("Function Name: %s\nInput: %s\nReasoning: %s\n------\n", functionName, input, reasoning)

		if functionName == "Search" {
			result := Search(input)
			fmt.Printf("Result:\n%s\n", result)
			knowledge = append(knowledge, result)
		} else if functionName == "Browse" {
			result := Browse(input)
			fmt.Printf("Result:\n%s\n", result)
			knowledge = append(knowledge, result[:1000])
		}
	}

	fmt.Println("Process completed.")
}
