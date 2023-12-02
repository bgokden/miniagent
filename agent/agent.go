package agent

import (
	"fmt"
	"strings"

	"github.com/bgokden/miniagent/messages"
	"github.com/bgokden/miniagent/prompt"
)

type FunctionInfo struct {
	FunctionName        string
	FunctionDescription string
	FunctionInput       string
	FunctionRef         func(string) string
}

type Agent struct {
	PromptTree     *prompt.FunctionNode
	MaxLength      int
	MessageHistory *messages.MessageHistory
	Functions      []FunctionInfo
}

type AgentOption func(*Agent)

func WithPromptTree(promptTree *prompt.FunctionNode) AgentOption {
	return func(a *Agent) {
		a.PromptTree = promptTree
	}
}

func WithMaxLength(maxLength int) AgentOption {
	return func(a *Agent) {
		a.MaxLength = maxLength
	}
}

func NewAgent(options ...AgentOption) *Agent {
	agent := &Agent{
		MaxLength:      8000, // Default MaxLength
		PromptTree:     DefaultBuildTree(),
		MessageHistory: messages.NewMessageHistory(),
		Functions: []FunctionInfo{
			{"Search", "This search is useful to get reliable quick data.", "Text to be searched", Search},
			{"Browse", "This browse is useful when users want to get content of a page.", "website url", Browse},
			{"CurrentTime", "This function is useful when you need the current time", "N/A", CurrentTime},
			{"Finish", "This is useful when the agent decides to finish this task and generate output.", "The output that you want to print as the result of your task as detailed as possible.", Finish},
		},
	}
	for _, option := range options {
		option(agent)
	}
	agent.PromptTree = BuildTree(agent)
	return agent
}

func findFunctionByName(functions []FunctionInfo, name string) (FunctionInfo, bool) {
	lowerName := strings.ToLower(name)
	for _, fn := range functions {
		if strings.ToLower(fn.FunctionName) == lowerName {
			return fn, true
		}
	}
	return FunctionInfo{}, false
}

func (a *Agent) GeneratePrompt(input ...string) (string, string, error) {
	var userInput string
	if len(input) > 0 {
		userInput = input[0]
	} else {
		userInput = "default input"
	}

	// Call the external GeneratePrompt function with the agent's PromptTree, userInput, and MaxLength
	system, err := prompt.GeneratePrompt(a.PromptTree, userInput, a.MaxLength)
	if err != nil {
		return "", "", err
	}
	return system, userInput, err
}

// DefaultBuildTree builds the default tree of FunctionNodes
func DefaultBuildTree() *prompt.FunctionNode {
	systemDescription := func(input string, maxLength int) (string, error) {
		return "You are an AI Assistant.\nThis is a friendly conversation between Human and AI.\nYour primary role is to answer questions and provide assistance.\n", nil
	}
	longTermMemory := func(input string, maxLength int) (string, error) {
		return "Long-Term Memory returned based on input.\n", nil
	}
	shortTermMemory := func(input string, maxLength int) (string, error) {
		return "Conversation:\nHuman: Hey\nAI:How can I help you?\n", nil
	}
	functions := func(input string, maxLength int) (string, error) {
		return "Functions:\n-Search\n-Browse\n-Final\n", nil
	}
	// askingDescription := func(input string, maxLength int) (string, error) {
	// 	return fmt.Sprintf("Human: %s\nAI:", input), nil
	// }

	root := prompt.NewFunctionNode("root", 0, nil,
		prompt.NewFunctionNode("system", 1, systemDescription),
		prompt.NewFunctionNode("memories", 2, nil,
			prompt.NewFunctionNode("ltm_optional", 1, nil, prompt.NewFunctionNode("ltm", 1, longTermMemory)),
			prompt.NewFunctionNode("stm", 2, shortTermMemory),
		),
		prompt.NewFunctionNode("functions_optional", 3, nil, prompt.NewFunctionNode("functions", 1, functions)),
		// prompt.NewFunctionNode("asking", 4, askingDescription),
	)

	return root
}

func FunctionsAsString(functions []FunctionInfo) string {
	var result strings.Builder
	result.WriteString("------\n")
	result.WriteString("Functions:\n")
	for _, fn := range functions {
		result.WriteString(fmt.Sprintf("- %s:\n", fn.FunctionDescription))
		result.WriteString(fmt.Sprintf("Function: %s\n", fn.FunctionName))
		// result.WriteString(fmt.Sprintf("Description: %s\n", fn.FunctionDescription))
		result.WriteString(fmt.Sprintf("Input: %s\n", fn.FunctionInput))
	}
	result.WriteString("------\n")
	return result.String()
}

func BuildTree(agent *Agent) *prompt.FunctionNode {
	systemDescription := func(input string, maxLength int) (string, error) {
		content := "You are an AI Assistant.\n" +
			"This is a friendly conversation between Human and AI.\n" +
			"Your primary role is to answer questions and provide assistance.\n" +
			"Output should only include one function as the next step using the format:\n" +
			"OUTPUT_FORMAT:\n" +
			"Function: name of the function\n" +
			"Input: Function Input as text\n" +
			"Reasoning: Reason to choose the Function and Input\n" +
			"Critism: Self critic of the current action\n" +
			"END_OF_OUTPUT_FORMAT:\n" +
			"Please only use the given functions in your responses.\n" +
			"Please write only one function in one response.\n"
		return content, nil
	}
	longTermMemory := func(input string, maxLength int) (string, error) {
		return "", nil
	}
	shortTermMemory := func(input string, maxLength int) (string, error) {
		return fmt.Sprintf("Conversation:\n%s\n", agent.MessageHistory.GetAllMessagesAsString()), nil
	}
	functions := func(input string, maxLength int) (string, error) {
		content := FunctionsAsString(agent.Functions)
		return content, nil
	}
	// askingDescription := func(input string, maxLength int) (string, error) {
	// 	return fmt.Sprintf("%s\n", input), nil
	// }

	root := prompt.NewFunctionNode("root", 0, nil,
		prompt.NewFunctionNode("system", 1, systemDescription),
		prompt.NewFunctionNode("memories", 2, nil,
			prompt.NewFunctionNode("ltm_optional", 1, nil, prompt.NewFunctionNode("ltm", 1, longTermMemory)),
			prompt.NewFunctionNode("stm", 2, shortTermMemory),
		),
		prompt.NewFunctionNode("functions_optional", 3, nil, prompt.NewFunctionNode("functions", 1, functions)),
		// prompt.NewFunctionNode("asking", 4, askingDescription),
	)

	return root
}

func (a *Agent) Run(input ...string) (string, error) {
	var userInput string
	// Determine the userInput based on the optional input argument
	if len(input) > 0 {
		userInput = input[0]
	} else {
		userInput = "default input" // Set your default input here
	}

	a.MessageHistory.AddMessage(messages.HumanMessage, "Human", "", userInput)

	userInputInferred := inferPrompt(userInput)

	var functionName, functionInput, lastGoodInput string
	var generateResp *GenerateResponse

	for functionName != "Finish" {
		system, prompt, err := a.GeneratePrompt(userInputInferred)
		if err != nil {
			fmt.Println("Error generating prompt:", err)
			return "", err
		}

		// log.Println(prompt)

		generateResp, err = callAPI(system, prompt)
		if err != nil {
			fmt.Println("Error calling API:", err)
			return "", err
		}

		functionName, functionInput, _, err = parseOutput(generateResp.Response)
		if err != nil {
			// fmt.Println("Error parsing output:", err)
			// fmt.Println(generateResp.Response)
			a.MessageHistory.AddMessage(messages.FunctionResult, "System", "", "Error parsing output.")
			break
			// continue
		}

		fmt.Printf("Running function: %s\n", functionName)

		// fmt.Printf("Function: %s\nInput: %s\nReasoning: %s\n------\n", functionName, functionInput, reasoning)
		result := ""
		if len(functionInput) > 0 {
			lastGoodInput = functionInput
		}
		if fn, found := findFunctionByName(a.Functions, functionName); found {
			result = fn.FunctionRef(functionInput)
			fmt.Println("Result:", result)
		} else {
			fmt.Println("Function not found:", functionName)
			result = Search(functionInput) // Defauls function call
		}

		// if functionName == "Search" {
		// 	result = Search(functionInput)
		// 	// fmt.Printf("Result:\n%s\n", result)
		// } else if functionName == "CurrentTime" {
		// 	result = GetCurrentTimeString()
		// 	// fmt.Printf("Result:\n%s\n", result)
		// } else if functionName == "Browse" {
		// 	result = Browse(functionInput)
		// 	result = summarizeWebPage(userInput, result)
		// 	result = fmt.Sprintf("URL: %s \nSummary: %s \n\n", functionInput, result)
		// 	// fmt.Printf("Result:\n%s\n", result)
		// }
		// else if functionName == "Finish" {
		// 	// fmt.Println(input)
		// }
		if len(result) > 0 {
			a.MessageHistory.AddMessage(messages.FunctionResult, "System", functionName, result)
			lastGoodInput = result
		}
	}

	fmt.Println("Process completed.")
	a.MessageHistory.AddMessage(messages.AIMessage, "AI", "", lastGoodInput)
	return lastGoodInput, nil
}
