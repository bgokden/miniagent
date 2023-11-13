package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/joho/godotenv"
	serpapi "github.com/serpapi/google-search-results-golang"
)

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

// safeString safely extracts a string from a map, returning an empty string if not found or if not a string.
func safeString(m map[string]interface{}, key string) string {
	value, ok := m[key]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
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
	var allResults strings.Builder

	for _, r := range results {
		result, ok := r.(map[string]interface{})
		if !ok {
			// Handle the case where the type assertion fails
			continue
		}
		title := safeString(result, "title")
		link := safeString(result, "link")
		snippet := safeString(result, "snippet")

		allResults.WriteString(fmt.Sprintf("Title: %s\nLink: %s\nSnippet: %s\n\n", title, link, snippet))
	}

	return allResults.String()

}

func Browse(url string) string {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var nodes []*cdp.Node

	var sb strings.Builder
	var b1 []byte
	err := chromedp.Run(ctx,
		chromedp.Emulate(device.IPhone7landscape),
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Nodes(`html`, &nodes, chromedp.ByQuery),
		chromedp.ActionFunc(func(c context.Context) error {
			return dom.RequestChildNodes(nodes[0].NodeID).WithDepth(-1).Do(c)
		}),
		chromedp.Sleep(10*time.Second),
		chromedp.CaptureScreenshot(&b1),
		chromedp.ActionFunc(func(c context.Context) error {
			printNodes(&sb, nodes, "", "  ", 20)
			return nil
		}),
	)
	if err != nil {
		log.Println(err.Error())
		// return err.Error()
	}
	if err := os.WriteFile("./tmp/screenshot1.png", b1, 0o644); err != nil {
		log.Println(err.Error())
	}
	return sb.String()
}

func printNodes(w *strings.Builder, nodes []*cdp.Node, padding, indent string, depth int) {
	if depth >= 0 {
		depth = depth - 1
	} else {
		return
	}
	names := map[string]bool{
		"html":        true,
		"body":        true,
		"head":        true,
		"header":      true,
		"description": true,
		"title":       true,
		"main":        true,
		"div":         true,
		"a":           true,
		"p":           true,
		"h1":          true,
		"h2":          true,
		"h3":          true,
		"h4":          true,
		"h5":          true,
		"span":        true,
		"#text":       true,
		"button":      true,
		// "footer":      true,
		"img":     true,
		"section": true,
		"aside":   true,
		"meta":    true,
		// "iframe":      true,
		"ul": true,
		"li": true,
	}
	attr := map[string]bool{
		"id":      true,
		"href":    true,
		"alt":     true,
		"name":    true,
		"content": true,
	}
	for _, node := range nodes {
		nodeName := strings.ToLower(node.NodeName)
		if !names[nodeName] {
			// log.Printf("NodeName was %s\n", nodeName)
			continue
		}
		switch {
		case node.NodeName == "#text":
			fmt.Fprintf(w, "%s#text: %q\n", padding, node.NodeValue)
		default:
			fmt.Fprintf(w, "%s%s:\n", padding, strings.ToLower(node.NodeName))
			if n := len(node.Attributes); n > 0 {
				fmt.Fprintf(w, "%sattributes:\n", padding+indent)
				for i := 0; i < n; i += 2 {
					if attr[node.Attributes[i]] {
						fmt.Fprintf(w, "%s%s: %q\n", padding+indent+indent, node.Attributes[i], node.Attributes[i+1])
					}
				}
			}
		}
		if node.ChildNodeCount > 0 {
			fmt.Fprintf(w, "%schildren:\n", padding+indent)
			printNodes(w, node.Children, padding+indent+indent, indent, depth)
		}
	}
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
	if functionName == "" || input == "" {
		err = fmt.Errorf("missing required fields in output. Output %s", text)
	}
	return
}

// - Function: Browse
//   Description: This browse is useful when users want to get content of a page.
//   Input: websiste url

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
- Function: Finish
  Description: This is useful when the agent decides to finish this task.
  Input: Summarize findings based on your knowledge.
Output should only include one function as the next step using the format:
Function: Function name
Input: Function Input as text
Reasoning: Reason to choose the Function and Input
`, userInput, knowledge_str)
}

func pullModel() error {
	ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT") // Replace with your actual endpoint
	pullEndpoint := fmt.Sprintf("%s/api/pull", ollamaEndpoint)
	modelName := "zephyr:latest"
	stream := false
	// Define the request body
	requestBody := struct {
		Name   string `json:"name"`
		Stream bool   `json:"stream"`
	}{
		Name:   modelName,
		Stream: stream,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", pullEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var response struct {
		Status string `json:"status"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}

	log.Println(response)

	if response.Status != "success" {
		return errors.New("request did not return success status")
	}

	return nil
}

func callAPI(prompt string) (*GenerateResponse, error) {
	ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT")
	generateEndpoint := fmt.Sprintf("%s/api/generate", ollamaEndpoint)

	requestBody := &GenerateRequest{
		Model:  "zephyr:latest",
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", generateEndpoint, bytes.NewBuffer(jsonData))
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

func summarizeWebPage(topic, input string) string {
	prompt := fmt.Sprintf(`Based on topic "%s", Extract and summarize the key content from following web page as input, 
	focusing on the main text, headings, and significant links. 
	Provide a concise summary highlighting the central themes or information presented on the page, 
	along with the most relevant links for further reading or context.
	Ignore cookie and consent forms. 
	Output should be in the following format:
	Content: Content of the web page excluding technologies used.
	IsRelated: Yes or No based on if it is related to the topic.
	Links: List of useful links.
	- Link 1
	- Link 2
	\n %s`, topic, input)
	response, err := callAPI(prompt)
	if err != nil {
		return err.Error()
	}
	return response.Response
}

func main() {

	loadEnv()

	err_pull := pullModel()
	if err_pull != nil {
		log.Printf("Error pulling model: %s", err_pull.Error())
	}

	userInput := "Please tell me the age of girlfriend of Leonardo Di Caprio currently."

	// result := Browse("https://weather.com/weather/today/l/a0a48c0f8630d7e60cc5d03bf2dc2d039cad87e8dfdb8fc476a43473a6ff7e17")
	// // fmt.Println(result)
	// result = summarizeWebPage(userInput, result)
	// fmt.Println("------")
	// fmt.Println(result)
	// fmt.Println("------")
	// os.Exit(0)

	knowledge := []string{}
	var functionName, input, reasoning string
	var generateResp *GenerateResponse
	var err error

	for functionName != "Finish" {
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
			result = summarizeWebPage(userInput, result)
			if len(result) > 1000 {
				result = result[:1000]
			}
			result = fmt.Sprintf("URL: %s \nSummary: %s \n Website might be blocking due to cookie consent\n", input, result)
			fmt.Printf("Result:\n%s\n", result)
			knowledge = append(knowledge, result)
		} else if functionName == "Finish" {
			fmt.Println(input)
		}
	}

	fmt.Println("Process completed.")
}
