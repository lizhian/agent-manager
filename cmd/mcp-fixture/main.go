// mcp-fixture is a local, network-free MCP server for native debugging acceptance.
package main

import (
	"context"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
)

type arguments struct {
	Message string `json:"message" jsonschema:"要返回的文本"`
}

func main() {
	server := sdk.NewServer(&sdk.Implementation{Name: "agent-manager-fixture", Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "返回 JSON 参数中的 message；仅用于本地验收。"}, func(ctx context.Context, req *sdk.CallToolRequest, input arguments) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: input.Message + os.Getenv("MCP_FIXTURE_VALUE")}}}, nil, nil
	})
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
