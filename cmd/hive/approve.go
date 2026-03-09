package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lovyou-ai/hive/pkg/authority"
)

// cliApprover prompts the human operator on stdin for authority decisions.
// Returns an Approver function suitable for authority.NewGate.
func cliApprover() authority.Approver {
	reader := bufio.NewReader(os.Stdin)

	return func(req authority.Request) (bool, string) {
		fmt.Println()
		fmt.Println("════════════════════════════════════════════════")
		fmt.Printf("  APPROVAL REQUIRED (%s)\n", req.Level)
		fmt.Println("════════════════════════════════════════════════")
		fmt.Printf("  Action:        %s\n", req.Action)
		fmt.Printf("  Requested by:  %s\n", req.Actor.Value())
		fmt.Printf("  Justification: %s\n", req.Justification)
		fmt.Println("════════════════════════════════════════════════")
		fmt.Print("  Approve? [y/n]: ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Sprintf("input error: %v", err)
		}

		answer := strings.TrimSpace(strings.ToLower(line))
		switch answer {
		case "y", "yes":
			return true, "approved by human operator"
		default:
			fmt.Print("  Reason for denial (optional): ")
			reason, _ := reader.ReadString('\n')
			reason = strings.TrimSpace(reason)
			if reason == "" {
				reason = "denied by human operator"
			}
			return false, reason
		}
	}
}
