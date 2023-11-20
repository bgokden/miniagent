package agent

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
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/joho/godotenv"

	serpapi "github.com/serpapi/google-search-results-golang"
)

const MODEL_NAME = "zephyr"

type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Raw    bool   `json:"raw"`
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
		return err.Error()
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

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.UserDataDir("./tmp/user"),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocatorCancel()
	ctx, cancel := chromedp.NewContext(allocatorCtx)
	defer cancel()

	var nodes []*cdp.Node

	var sb strings.Builder
	var b1 []byte
	err := chromedp.Run(ctx,
		chromedp.Emulate(device.IPhone13),
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Nodes(`html`, &nodes, chromedp.ByQuery),
		chromedp.ActionFunc(func(c context.Context) error {
			return dom.RequestChildNodes(nodes[0].NodeID).WithDepth(-1).Do(c)
		}),
		chromedp.Sleep(1*time.Second),
		chromedp.CaptureScreenshot(&b1),
		chromedp.ActionFunc(func(c context.Context) error {
			printNodes(&sb, nodes, "", "  ", 20)
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			// for i, cookie := range cookies {
			// 	log.Printf("chrome cookie %d: %+v", i, cookie.Name)
			// }
			log.Println(len(cookies))
			return nil
		}),
	)
	if err != nil {

		log.Println(err.Error())
		return err.Error()
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
	// text = restructureOutput(text)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Function:") {
			functionName = strings.TrimSpace(strings.TrimPrefix(line, "Function:"))
		} else if strings.HasPrefix(line, "Input:") {
			input = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "Input:")), "\"")
		} else if strings.HasPrefix(line, "Reasoning:") {
			reasoning = strings.TrimSpace(strings.TrimPrefix(line, "Reasoning:"))
		}
	}
	if functionName == "" {
		err = fmt.Errorf("missing required fields in output. Output <<<<<<%s>>>>>", text)
	}
	return
}

// func createPrompt(userInput string, knowledge []string) string {
// 	knowledge_str := strings.Join(knowledge, "\n")
// 	return fmt.Sprintf(`Task: You are a functional agent which analyzes the input and decides the next step as a Function and input. Do not write anything else or you will fail. Strictly follow the given output.
// Input: %s
// Active Knowledge:
// %s
// Functions:
// - Function: Search
//   Description: This search is useful to get reliable quick data.
//   Input: Search  Input
// - Function: Browse
//   Description: This browse is useful when users want to get content of a page.
//   Input: websiste url
// - Function: CurrentTime
//   Description: Get Current Time
// - Function: Finish
//   Description: This is useful when the agent decides to finish this task.
//   Input: Result of the task.
// Output should only include one function as the next step using the format:
// Function: Function name
// Input: Function Input as text
// Reasoning: Reason to choose the Function and Input
// `, userInput, knowledge_str)
// }

func PullModel() error {
	ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT") // Replace with your actual endpoint
	pullEndpoint := fmt.Sprintf("%s/api/pull", ollamaEndpoint)
	log.Printf("URL: %v", pullEndpoint)
	modelName := MODEL_NAME
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

	if resp.StatusCode < 200 && resp.StatusCode > 299 {
		return errors.New(fmt.Sprintf("API Pull Error code: %d", resp.StatusCode))
	}

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
		Model:  MODEL_NAME,
		Prompt: prompt,
		Raw:    true,
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

	if resp.StatusCode < 200 && resp.StatusCode > 299 {
		return nil, errors.New(fmt.Sprintf("API Error code: %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var generateResp GenerateResponse
	err = json.Unmarshal(body, &generateResp)
	if err != nil {
		log.Printf("error decoding response: %v", err)
		if e, ok := err.(*json.SyntaxError); ok {
			log.Printf("syntax error at byte offset %d", e.Offset)
		}
		log.Printf("Response: %q", body)
		if string(body) == "error code: 524" {
			return callAPI(prompt)
		}
		return nil, err
	}

	return &generateResp, nil
}

// splitIntoChunks splits a text into chunks of a specified length.
func splitIntoChunks(text string, chunkSize int) []string {
	var chunks []string
	for len(text) > 0 {
		if len(text) < chunkSize {
			chunks = append(chunks, text)
			break
		}

		chunk := text[:chunkSize]
		chunks = append(chunks, chunk)
		text = text[chunkSize:]
	}

	return chunks
}

func inferPrompt(input string) string {
	systemText := "Analyze the user's original intent and reformulate it into a well-structured, single-paragraph input. This input should clearly outline the task requirements and specify the criteria for successful completion by an AI system, based on the following provided text:"
	prompt := fmt.Sprintf("<|system|>%s</s><|user|>%s</s><|assistant|>", systemText, input)
	response, err := callAPI(prompt)
	if err != nil {
		return input
	}
	return response.Response
}

func restructureOutput(input string) string {
	systemText := "Restructure output as follows:\nFunction: name of the function\nInput: Funtion Input\nReasoning: Why this function is selected\nCritism: A critic of this action\n"
	prompt := fmt.Sprintf("<|system|>%s</s><|user|>%s</s><|assistant|>", systemText, input)
	response, err := callAPI(prompt)
	if err != nil {
		return input
	}
	return response.Response
}

func summarizeWebPage(topic, input string) string {
	texts := splitIntoChunks(input, 1000)
	previous := ""
	isRelated := false
	for i, text := range texts {
		prompt := fmt.Sprintf(`"For the topic '%s', please prograsively extract essential information from the given web page and previous extract, 
		concentrating on the main body text, headings, and significant hyperlinks. 
		Summarize the central themes or key information related to the specified topic, 
		ensuring the summary is succinct and directly relevant. 
		Exclude any details about website technologies or unrelated content. 
		Additionally, identify if the content is relevant to the given topic. 
		Provide a list of the most pertinent links for additional reading or context, disregarding cookie and consent notices. 
		The output should be formatted as follows:

		Content: [Concise summary focusing on the topic]
		IsRelated: [Yes/No, based on relevance to the topic]
		Links: [Relevant links for further information]
		- Link 1
		- Link 2

		Previous Extract:
		%s

		WebPage:
		%s"`, topic, previous, text)
		response, err := callAPI(prompt)
		if err != nil {
			if len(previous) > 0 {
				return previous
			}
			return err.Error()
		}
		previous = response.Response
		isRelated = findIsRelatedStatus(previous)
		if !isRelated {
			return previous
		}
		// log.Printf("Iteration %d Summary: %s", i, previous)
		if i > 5 {
			break
		}
	}
	return previous
}

func findIsRelatedStatus(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, "IsRelated: Yes") {
			return true
		}
		if strings.Contains(line, "IsRelated: No") {
			return false
		}
	}
	return true
}

// GetCurrentTimeString returns the current time as a string.
func GetCurrentTimeString() string {
	currentTime := time.Now()
	return fmt.Sprintf("Current time is %s\n", currentTime.Format("2006-01-02 15:04:05"))
}
