package prompt

import (
	"sort"
	"strings"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

var (
	tk   *tokenizer.Tokenizer
	once sync.Once
)

func getTokenizer() (*tokenizer.Tokenizer, error) {
	var err error
	once.Do(func() {
		// Initialize tokenizer here
		configFile, e := tokenizer.CachedPath("HuggingFaceH4/zephyr-7b-beta", "tokenizer.json")
		if e != nil {
			err = e
			return
		}
		tk, e = pretrained.FromFile(configFile)
		if e != nil {
			err = e
			return
		}
	})
	return tk, err
}

func getLength(s string, tk *tokenizer.Tokenizer) int {
	en, err := tk.EncodeSingle(s)
	if err != nil {
		return 0
	}
	return en.Len()
}

// FunctionNode represents a node in the tree
type FunctionNode struct {
	ID           string
	Priority     int
	GeneratePart func(string, int) (string, error)
	Children     []*FunctionNode
}

// NewFunctionNode creates a new FunctionNode
func NewFunctionNode(id string, priority int, genFunc func(string, int) (string, error), children ...*FunctionNode) *FunctionNode {
	return &FunctionNode{
		ID:           id,
		Priority:     priority,
		GeneratePart: genFunc,
		Children:     children,
	}
}

// ByPriority implements sort.Interface for []*FunctionNode based on the Priority field
type ByPriority []*FunctionNode

func (a ByPriority) Len() int           { return len(a) }
func (a ByPriority) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPriority) Less(i, j int) bool { return a[i].Priority < a[j].Priority }

// FunctionNodeOutput stores the node's output and the order for output assembly
type FunctionNodeOutput struct {
	node   *FunctionNode
	output string
	order  int // This defines the order in which the output should be assembled
}

// GeneratePrompt assembles the prompt with separate processing and output orders
func GeneratePrompt(root *FunctionNode, input string, maxLength int) (string, error) {
	tk, err := getTokenizer()
	if err != nil {
		return "", err
	}
	var queue []FunctionNodeOutput
	var outputParts []FunctionNodeOutput // This will store the outputs in the order they should be assembled

	queue = append(queue, FunctionNodeOutput{node: root, output: "", order: root.Priority})

	count := 0

	for len(queue) > 0 {
		if count >= maxLength {
			break
		}
		current := queue[0]
		queue = queue[1:]

		// fmt.Printf("Processing Node: %s\n", current.node.ID)

		var part string
		var err error

		if current.node.GeneratePart != nil {
			part, err = current.node.GeneratePart(input, maxLength-count)
			if err != nil {
				return "", err
			}
			// fmt.Printf("Generated Part: %s\n", part)
			current.output = part
			outputParts = append(outputParts, current)
			count += getLength(part, tk)
		}

		for _, child := range current.node.Children {
			queue = append(queue, FunctionNodeOutput{node: child, output: "", order: child.Priority})
		}
	}

	// Sort outputParts based on the order field
	sort.Slice(outputParts, func(i, j int) bool {
		return outputParts[i].order < outputParts[j].order
	})

	// Assemble the final output based on the sorted parts
	var finalOutput strings.Builder
	for _, part := range outputParts {
		if finalOutput.Len() > 0 && part.output != "" {
			finalOutput.WriteString("")
		}
		finalOutput.WriteString(part.output)
	}

	return finalOutput.String(), nil
}

// func main() {
// 	// Example prompt part generators and priorities
// 	systemDescription := func(input string, maxLength int) (string, error) {
// 		return "You are an AI Assistant.\nThis is a friendly conversation between Human and AI.\nYour primary role is to answer questions and provide assistance.\n", nil
// 	}
// 	longTermMemory := func(input string, maxLength int) (string, error) {
// 		return "Long-Term Memory returned based on input.\n", nil
// 	}
// 	shortTermMemory := func(input string, maxLength int) (string, error) {
// 		return "Conversation:\nHuman: Hey\nAI:How can I help you?\n", nil
// 	}
// 	functions := func(input string, maxLength int) (string, error) {
// 		return "Functions:\n-Search\n-Browse\n-Final\n", nil
// 	}
// 	askingDescription := func(input string, maxLength int) (string, error) {
// 		return fmt.Sprintf("Human: %s\nAI:", input), nil
// 	}

// 	// Building the tree of FunctionNodes with priorities
// 	root := NewFunctionNode("root", 0, nil, // Root node is empty and does not generate any part
// 		NewFunctionNode("system", 1, systemDescription),
// 		NewFunctionNode("memories", 2, nil, // Memories node does not generate its own part
// 			NewFunctionNode("ltm_optional", 1, nil, NewFunctionNode("ltm", 1, longTermMemory)),
// 			NewFunctionNode("stm", 2, shortTermMemory),
// 		),
// 		NewFunctionNode("functions_optional", 3, nil, NewFunctionNode("functions", 1, functions)),
// 		NewFunctionNode("asking", 4, askingDescription),
// 	)

// 	// Generate the prompt
// 	input := "How does AI work?"
// 	maxLength := 50
// 	prompt, err := GeneratePrompt(root, input, maxLength)
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 	} else {
// 		fmt.Println("Generated Prompt:")
// 		fmt.Println(prompt)
// 	}
// }
