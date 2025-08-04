package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Config holds server configuration
type Config struct {
	Port        string `json:"port"`
	BasePath    string `json:"base_path"`
	MaxFileSize int64  `json:"max_file_size"`
}

// GrepQuery represents a single grep search query
type GrepQuery struct {
	Pattern     string  `json:"pattern"`
	FilePattern *string `json:"file_pattern,omitempty"`
	IgnoreCase  *bool   `json:"ignore_case,omitempty"`
}

// FileNode represents a file or directory in the tree structure
type FileNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" or "directory"
	Size     *int64      `json:"size,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
	Path     string      `json:"path"`
}

// GrepResult represents a grep search result
type GrepResult struct {
	Query   string            `json:"query"`
	Matches []GrepMatchResult `json:"matches"`
	Error   *string           `json:"error,omitempty"`
}

// GrepMatchResult represents a single file match
type GrepMatchResult struct {
	FilePath string     `json:"file_path"`
	Lines    []GrepLine `json:"lines"`
}

// GrepLine represents a line in grep results
type GrepLine struct {
	LineNumber int    `json:"line_number"`
	Content    string `json:"content"`
	IsMatch    bool   `json:"is_match"`
}

// Server represents our MCP server
type MCPFileServer struct {
	config *Config
	server *server.MCPServer
}

// NewMCPFileServer creates a new MCP server instance
func NewMCPFileServer(config *Config) *MCPFileServer {
	// Create MCP server with proper capabilities
	mcpServer := server.NewMCPServer(
		"filesystem-mcp-server",
		"1.0.0",
		server.WithToolCapabilities(true), // Enable tool capabilities
		server.WithRecovery(),             // Add error recovery
		server.WithLogging(),              // Add logging
	)

	return &MCPFileServer{
		config: config,
		server: mcpServer,
	}
}

// RegisterTools registers all available tools with the MCP server
func (s *MCPFileServer) RegisterTools() {
	// 1. Register read_file_structure tool
	fileStructureTool := mcp.NewTool(
		"read_file_structure",
		mcp.WithDescription("Read and return the file structure of the configured filesystem path"),
	)
	s.server.AddTool(fileStructureTool, s.handleReadFileStructure)

	// 2. Register read_file_contents tool
	fileContentsTool := mcp.NewTool(
		"read_file_contents",
		mcp.WithDescription("Read and return the contents of a specific file"),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to the file relative to the configured base path")),
	)
	s.server.AddTool(fileContentsTool, s.handleReadFileContents)

	// 3. Register grep_search tool
	grepTool := mcp.NewTool(
		"grep_search",
		mcp.WithDescription("Search for patterns in files using grep with context lines. Supports up to 20 search queries."),
		mcp.WithString("queries", mcp.Required(), mcp.Description("JSON string containing array of search queries (max 20)")),
		mcp.WithNumber("context_lines", mcp.Description("Number of lines before and after each match (default: 5)")),
	)
	s.server.AddTool(grepTool, s.handleGrepSearch)

	log.Println("Registered 3 filesystem tools: read_file_structure, read_file_contents, grep_search")
}

// Start starts the HTTP MCP server
func (s *MCPFileServer) Start() error {
	// Register all tools
	s.RegisterTools()

	// Create streamable HTTP server for modern MCP transport
	httpServer := server.NewStreamableHTTPServer(s.server)

	log.Printf("Starting MCP File Server on port %s", s.config.Port)
	log.Printf("Configured base path: %s", s.config.BasePath)
	log.Printf("Server endpoint will be: http://localhost%s/mcp", s.config.Port)

	// Start the server
	return httpServer.Start(s.config.Port)
}

// handleReadFileContents handles the read_file_contents tool
func (s *MCPFileServer) handleReadFileContents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filePath, err := request.RequireString("file_path")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Missing required parameter: %v", err)), nil
	}

	// Validate and resolve path
	fullPath, err := s.validateFilePath(filePath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid file path: %v", err)), nil
	}

	// Check file size
	stat, err := os.Stat(fullPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("File not found: %v", err)), nil
	}

	if stat.Size() > s.config.MaxFileSize {
		return mcp.NewToolResultError(fmt.Sprintf("File too large (%.2f MB > %.2f MB)",
			float64(stat.Size())/1024/1024, float64(s.config.MaxFileSize)/1024/1024)), nil
	}

	// Read file contents
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read file: %v", err)), nil
	}

	// Create result as JSON text
	result := map[string]interface{}{
		"file_path":  filePath,
		"size_bytes": stat.Size(),
		"content":    string(content),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// handleGrepSearch handles the grep_search tool
func (s *MCPFileServer) handleGrepSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Parse queries JSON string
	queriesStr, ok := args["queries"].(string)
	if !ok {
		return mcp.NewToolResultError("queries parameter must be a JSON string"), nil
	}

	var queries []GrepQuery
	if err := json.Unmarshal([]byte(queriesStr), &queries); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid queries JSON: %v", err)), nil
	}

	// Validate number of queries
	if len(queries) == 0 {
		return mcp.NewToolResultError("At least one search query is required"), nil
	}
	if len(queries) > 20 {
		return mcp.NewToolResultError("Maximum 20 search queries allowed"), nil
	}

	// Set default context lines
	contextLines := 5
	if val, ok := args["context_lines"]; ok && val != nil {
		if cl, ok := val.(float64); ok {
			contextLines = int(cl)
		}
	}

	// Execute searches
	results := make([]GrepResult, len(queries))
	for i, query := range queries {
		result, err := s.executeGrepQuery(query, contextLines)
		if err != nil {
			errorMsg := err.Error()
			results[i] = GrepResult{
				Query: query.Pattern,
				Error: &errorMsg,
			}
		} else {
			results[i] = *result
		}
	}

	// Create result as JSON text
	result := map[string]interface{}{
		"base_path":     s.config.BasePath,
		"context_lines": contextLines,
		"results":       results,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// Helper methods

// validateFilePath validates and resolves a file path relative to base path
func (s *MCPFileServer) validateFilePath(filePath string) (string, error) {
	// Clean the path
	cleanPath := filepath.Clean(filePath)

	// Prevent directory traversal attacks
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path traversal not allowed")
	}

	// Build full path
	fullPath := filepath.Join(s.config.BasePath, cleanPath)

	// Ensure the resolved path is still within base path
	relPath, err := filepath.Rel(s.config.BasePath, fullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path outside of allowed directory")
	}

	return fullPath, nil
}

// executeGrepQuery executes a single grep query with context
func (s *MCPFileServer) executeGrepQuery(query GrepQuery, contextLines int) (*GrepResult, error) {
	// Build grep command
	args := []string{}

	// Add context lines
	if contextLines > 0 {
		args = append(args, "-C", strconv.Itoa(contextLines))
	}

	// Add line numbers
	args = append(args, "-n")

	// Add ignore case if specified
	if query.IgnoreCase != nil && *query.IgnoreCase {
		args = append(args, "-i")
	}

	// Add recursive search
	args = append(args, "-r")

	// Add pattern
	args = append(args, query.Pattern)

	// Add file pattern or search path
	if query.FilePattern != nil {
		args = append(args, "--include="+*query.FilePattern)
	}
	args = append(args, s.config.BasePath)

	// Execute grep command
	cmd := exec.Command("grep", args...)
	output, err := cmd.Output()

	// Handle case where grep finds no matches (exit code 1)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				// No matches found, return empty result
				return &GrepResult{
					Query:   query.Pattern,
					Matches: []GrepMatchResult{},
				}, nil
			}
		}
		return nil, fmt.Errorf("grep command failed: %v", err)
	}

	// Parse grep output
	matches, err := s.parseGrepOutput(string(output))
	if err != nil {
		return nil, err
	}

	return &GrepResult{
		Query:   query.Pattern,
		Matches: matches,
	}, nil
}

// parseGrepOutput parses grep output with context lines
func (s *MCPFileServer) parseGrepOutput(output string) ([]GrepMatchResult, error) {
	if output == "" {
		return []GrepMatchResult{}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	matches := make(map[string][]GrepLine)

	// Regex to parse grep output: filename:line_number:content or filename:line_number-content
	lineRegex := regexp.MustCompile(`^([^:]+):(\d+)([:|-])(.*)$`)

	for _, line := range lines {
		if line == "--" {
			continue // Skip separator lines
		}

		matchesFound := lineRegex.FindStringSubmatch(line)
		if len(matchesFound) != 5 {
			continue
		}

		filePath := matchesFound[1]
		lineNumStr := matchesFound[2]
		separator := matchesFound[3]
		content := matchesFound[4]

		// Convert absolute path to relative path
		relPath, err := filepath.Rel(s.config.BasePath, filePath)
		if err != nil {
			relPath = filePath
		}

		lineNum, err := strconv.Atoi(lineNumStr)
		if err != nil {
			continue
		}

		isMatch := separator == ":"

		grepLine := GrepLine{
			LineNumber: lineNum,
			Content:    content,
			IsMatch:    isMatch,
		}

		matches[relPath] = append(matches[relPath], grepLine)
	}

	// Convert map to slice
	result := make([]GrepMatchResult, 0, len(matches))
	for filePath, lines := range matches {
		result = append(result, GrepMatchResult{
			FilePath: filePath,
			Lines:    lines,
		})
	}

	return result, nil
}

// validateConfig validates the server configuration
func validateConfig(config *Config) error {
	// Check if base path exists and is readable
	if _, err := os.Stat(config.BasePath); os.IsNotExist(err) {
		return fmt.Errorf("base path does not exist: %s", config.BasePath)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(config.BasePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	config.BasePath = absPath

	// Validate max file size
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = 10 * 1024 * 1024 // Default: 10MB
	}

	return nil
}

// loadConfig loads configuration from command line flags
func loadConfig() (*Config, error) {
	config := &Config{}

	flag.StringVar(&config.Port, "port", ":3001", "Port to listen on (e.g., :3001)")
	flag.StringVar(&config.BasePath, "base-path", ".", "Base filesystem path to serve")
	flag.Int64Var(&config.MaxFileSize, "max-file-size", 10*1024*1024, "Maximum file size in bytes (default: 10MB)")

	flag.Parse()

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

func main() {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create and start server
	mcpServer := NewMCPFileServer(config)

	if err := mcpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
