package mcpadapter_test

import (
	"context"
	"fmt"
	"testing"

	mcpadapter "m/orchard/pkg/mcpserver"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLocalhostMCPIntegration(t *testing.T) {
	ctx := context.Background()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mcp-test",
		Version: "1.0.0",
	}, nil)

	// (Adjust the endpoint path if your server mounts it elsewhere, e.g., "/" or "/mcp")
	transport := &mcp.StreamableClientTransport{
		Endpoint: "http://localhost:8082/mcp",
	}

	// 3. Load the tools using your adapter
	nativeTools, session, err := mcpadapter.LoadMCPToolsAsNative(ctx, client, transport)
	if err != nil {
		t.Fatalf("Failed to connect or load tools from localhost:8082: %v", err)
	}
	defer session.Close()

	// 4. Verify discovery output
	fmt.Printf("Successfully connected! Discovered %d tools:\n", len(nativeTools))
	for _, tool := range nativeTools {
		fmt.Printf("  - Tool Name: %s\n    Description: %s\n", tool.Name, tool.Description)
	}

	// Optional: Test executing a discovered tool if you know its name & arguments
	// if len(nativeTools) > 0 {
	//     // dummy message/index for your ExecFunc signature
	//     _, err := nativeTools[0].Execute(ctx, someMessage, 0)
	//     if err != nil {
	//         t.Errorf("Tool execution failed: %v", err)
	//     }
	// }
}
