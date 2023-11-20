package main

import (
	"fmt"
	"log"

	"github.com/bgokden/miniagent/agent"
	"github.com/joho/godotenv"
)

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

func main() {
	loadEnv()

	err_pull := agent.PullModel()
	if err_pull != nil {
		log.Printf("Error pulling model: %s", err_pull.Error())
	}

	userInput := "Create a list of VCs in the Netherlands."

	anAgent := agent.NewAgent()
	result, err := anAgent.Run(userInput)
	if err != nil {
		log.Printf("Error from agent: %s", err.Error())
	}
	fmt.Println("------------------------------------")
	fmt.Println(result)

}
