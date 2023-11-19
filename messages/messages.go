package messages

import (
	"fmt"
	"strings"
)

// MessageType is an enumeration of different types of messages.
type MessageType int

const (
	FunctionResult MessageType = iota // Represents a function result
	AIMessage                         // Represents a message from AI
	HumanMessage                      // Represents a message from a human
	ChatMessage                       // Represents a generic chat message
)

// Message represents a single message in the chat history.
type Message struct {
	Type         MessageType // The type of the message
	Content      string      // The content of the message
	Sender       string      // The sender of the message (custom user name)
	FunctionName string      // The name of the function (if applicable)
}

// MessageHistory stores a history of messages.
type MessageHistory struct {
	messages []Message // Slice to store messages
}

// NewMessageHistory creates a new instance of MessageHistory.
func NewMessageHistory() *MessageHistory {
	return &MessageHistory{
		messages: make([]Message, 0),
	}
}

// AddMessage adds a new message to the history.
func (m *MessageHistory) AddMessage(msgType MessageType, sender, functionName, content string) {
	m.messages = append(m.messages, Message{
		Type:         msgType,
		Sender:       sender,
		FunctionName: functionName,
		Content:      content,
	})
}

// GetAllMessagesAsString returns all messages in the history as a single string.
func (m *MessageHistory) GetAllMessagesAsString() string {
	var result strings.Builder
	for _, msg := range m.messages {
		prefix := fmt.Sprintf("%s: ", msg.Sender)
		if msg.FunctionName != "" {
			prefix += fmt.Sprintf("[%s] ", msg.FunctionName)
		}
		result.WriteString(fmt.Sprintf("%s%s\n", prefix, msg.Content))
	}
	return result.String()
}

// func main() {
// 	history := NewMessageHistory()

// 	history.AddMessage(AIMessage, "AI", "", "Hello, how can I help you?")
// 	history.AddMessage(HumanMessage, "John Doe", "", "I need assistance with my project.")
// 	history.AddMessage(FunctionResult, "System", "Analysis", "Project analysis completed successfully.")

// 	allMessages := history.GetAllMessagesAsString()
// 	fmt.Println(allMessages)
// }
