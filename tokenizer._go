package main

import (
	"fmt"
	"log"
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

func main() {
	tk, err := getTokenizer()
	if err != nil {
		log.Fatal(err)
	}

	sentence := `The Gophers craft code using [MASK] language.`
	length := getLength(sentence, tk)
	fmt.Printf("Length of tokenized sentence: %v\n", length)
}

func getLength(s string, tk *tokenizer.Tokenizer) int {
	en, err := tk.EncodeSingle(s)
	if err != nil {
		log.Fatal(err)
	}
	return en.Len()
}
