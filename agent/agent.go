package agent

import (
	"fmt"
	"log"

	"github.com/bgokden/miniagent/messages"
	"github.com/bgokden/miniagent/prompt"
)

type Agent struct {
	PromptTree     *prompt.FunctionNode
	MaxLength      int
	MessageHistory *messages.MessageHistory
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
		MaxLength:      4000, // Default MaxLength
		PromptTree:     DefaultBuildTree(),
		MessageHistory: messages.NewMessageHistory(),
	}
	for _, option := range options {
		option(agent)
	}
	agent.PromptTree = BuildTree(agent)
	return agent
}

func (a *Agent) GeneratePrompt(input ...string) (string, error) {
	var userInput string
	if len(input) > 0 {
		userInput = input[0]
	} else {
		userInput = "default input"
	}

	// Call the external GeneratePrompt function with the agent's PromptTree, userInput, and MaxLength
	return prompt.GeneratePrompt(a.PromptTree, userInput, a.MaxLength)
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
	askingDescription := func(input string, maxLength int) (string, error) {
		return fmt.Sprintf("Human: %s\nAI:", input), nil
	}

	root := prompt.NewFunctionNode("root", 0, nil,
		prompt.NewFunctionNode("system", 1, systemDescription),
		prompt.NewFunctionNode("memories", 2, nil,
			prompt.NewFunctionNode("ltm_optional", 1, nil, prompt.NewFunctionNode("ltm", 1, longTermMemory)),
			prompt.NewFunctionNode("stm", 2, shortTermMemory),
		),
		prompt.NewFunctionNode("functions_optional", 3, nil, prompt.NewFunctionNode("functions", 1, functions)),
		prompt.NewFunctionNode("asking", 4, askingDescription),
	)

	return root
}

func BuildTree(agent *Agent) *prompt.FunctionNode {
	systemDescription := func(input string, maxLength int) (string, error) {
		content := "<|system|>You are an AI Assistant.\nThis is a friendly conversation between Human and AI.\nYour primary role is to answer questions and provide assistance.\n" +
			"Output should only include one function as the next step using the format:\n" +
			"Function: name of the function\n" +
			"Input: Function Input as text\n" +
			"Reasoning: Reason to choose the Function and Input\n" +
			"Critism: Self critic of the current action\n"
		return content, nil
	}
	longTermMemory := func(input string, maxLength int) (string, error) {
		return "", nil
	}
	shortTermMemory := func(input string, maxLength int) (string, error) {
		return fmt.Sprintf("Conversation:\n%s\n", agent.MessageHistory.GetAllMessagesAsString()), nil
	}
	functions := func(input string, maxLength int) (string, error) {
		content := "Functions:\n" +
			"- Function: Search\n" +
			"  Input: Search Input\n" +
			"  Description: This search is useful to get reliable quick data.\n" +
			"- Function: Browse\n" +
			"  Input: website url\n" +
			"  Description: This browse is useful when users want to get content of a page.\n" +
			"- Function: CurrentTime\n" +
			"  Description: Get Current Time\n" +
			"  Input: N/A\n" +
			"- Function: Finish\n" +
			"  Input: Result of the task.\n" +
			"  Description: This is useful when the agent decides to finish this task.\n"

		return content, nil
	}
	askingDescription := func(input string, maxLength int) (string, error) {
		return fmt.Sprintf("</s><|user|>%s\n</s><|assistant|>", input), nil
	}

	root := prompt.NewFunctionNode("root", 0, nil,
		prompt.NewFunctionNode("system", 1, systemDescription),
		prompt.NewFunctionNode("memories", 2, nil,
			prompt.NewFunctionNode("ltm_optional", 1, nil, prompt.NewFunctionNode("ltm", 1, longTermMemory)),
			prompt.NewFunctionNode("stm", 2, shortTermMemory),
		),
		prompt.NewFunctionNode("functions_optional", 3, nil, prompt.NewFunctionNode("functions", 1, functions)),
		prompt.NewFunctionNode("asking", 4, askingDescription),
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

	var functionName, functionInput, reasoning string
	var generateResp *GenerateResponse

	for functionName != "Finish" {
		prompt, err := a.GeneratePrompt(userInputInferred)
		if err != nil {
			fmt.Println("Error generating prompt:", err)
			return "", err
		}

		log.Println(prompt)

		generateResp, err = callAPI(prompt)
		if err != nil {
			fmt.Println("Error calling API:", err)
			return "", err
		}

		functionName, functionInput, reasoning, err = parseOutput(generateResp.Response)
		if err != nil {
			fmt.Println("Error parsing output:", err)
			fmt.Println(generateResp.Response)
			a.MessageHistory.AddMessage(messages.FunctionResult, "System", "", "Error parsing output.")
			break
			// continue
		}

		fmt.Printf("Function: %s\nInput: %s\nReasoning: %s\n------\n", functionName, functionInput, reasoning)
		result := ""
		if functionName == "Search" {
			result = Search(functionInput)
			fmt.Printf("Result:\n%s\n", result)
		} else if functionName == "CurrentTime" {
			result = GetCurrentTimeString()
			fmt.Printf("Result:\n%s\n", result)
		} else if functionName == "Browse" {
			result = Browse(functionInput)
			result = summarizeWebPage(userInput, result)
			result = fmt.Sprintf("URL: %s \nSummary: %s \n\n", functionInput, result)
			fmt.Printf("Result:\n%s\n", result)
		} else if functionName == "Finish" {
			fmt.Println(input)
		}
		if len(result) > 0 {
			a.MessageHistory.AddMessage(messages.FunctionResult, "System", functionName, result)
		}
	}

	fmt.Println("Process completed.")
	a.MessageHistory.AddMessage(messages.AIMessage, "AI", "", functionInput)
	return functionInput, nil
}
