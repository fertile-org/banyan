package main

import (
	"fmt"
	"log"

	"github.com/fertile-org/banyan/internal/common"
)

func main() {
	log.Printf("Banyan CLI %s starting...", common.Version)
	fmt.Println("Banyan CLI - Docker Compose deployment orchestrator")
}
