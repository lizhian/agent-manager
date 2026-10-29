// mcp-fixture is a local, network-free MCP server for native debugging acceptance.
package main

import (
	"context"
	"flag"
	"fmt"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net"
	"net/http"
	"os"
)

type arguments struct {
	Message string `json:"message" jsonschema:"要返回的文本"`
}

func main() {
	listen := flag.String("listen", "", "HTTP/SSE 本地监听地址，如127.0.0.1:0")
	flag.Parse()
	server := sdk.NewServer(&sdk.Implementation{Name: "agent-manager-fixture", Version: "1"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "返回 JSON 参数中的 message；仅用于本地验收。"}, func(ctx context.Context, req *sdk.CallToolRequest, input arguments) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: input.Message + os.Getenv("MCP_FIXTURE_VALUE")}}}, nil, nil
	})
	if *listen != "" {
		listener, err := net.Listen("tcp", *listen)
		if err != nil {
			os.Exit(1)
		}
		mux := http.NewServeMux()
		mux.Handle("/mcp", sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
		mux.Handle("/sse", sdk.NewSSEHandler(func(*http.Request) *sdk.Server { return server }, nil))
		fmt.Printf("HTTP http://%s/mcp\nSSE http://%s/sse\n", listener.Addr(), listener.Addr())
		if err := http.Serve(listener, mux); err != nil {
			os.Exit(1)
		}
		return
	}
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
