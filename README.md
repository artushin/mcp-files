# Filesystem MCP Server

A Model Context Protocol (MCP) server written in Go that provides filesystem access tools for AI assistants.

## Features

This MCP server provides three powerful filesystem tools:

1. **read_file_structure** - Read and return the file structure of a pre-configured path
2. **read_file_contents** - Read the contents of individual files  
3. **grep_search** - Search for content using grep with context lines and support for up to 20 queries

## Quick Start

### Prerequisites

- Go 1.21 or later
- `grep` command available in system PATH
- Access to the filesystem path you want to serve

### Installation

```bash
# Clone or download the source code
# Initialize Go modules and install dependencies
go mod tidy

# Build the server
go build -o mcp-server .
```

### Running the Server

```bash
# Basic usage (serves current directory on port 8080)
./mcp-server

# Custom configuration
./mcp-server -port :9000 -base-path /path/to/your/files -max-file-size 5242880
```

### Command Line Options

- `-port` - Port to listen on (default: `:8080`)
- `-base-path` - Base filesystem path to serve (default: current directory)
- `-max-file-size` - Maximum file size in bytes (default: 10MB)

### Server Endpoint

Once running, the MCP server will be available at:
```
http://localhost:8080/mcp
```

## Available Tools

### 1. read_file_structure

Reads and returns the directory structure of the configured filesystem path.

**Parameters:**
- `max_depth` (optional): Maximum depth to traverse
- `file_pattern` (optional): Glob pattern to filter files (e.g., "*.go", "*.txt")

**Example Response:**
```json
{
  "base_path": "/path/to/files",
  "structure": {
    "name": "project",
    "type": "directory",
    "path": "",
    "children": [
      {
        "name": "main.go",
        "type": "file",
        "size": 1234,
        "path": "main.go"
      }
    ]
  }
}
```

### 2. read_file_contents

Reads and returns the contents of a specific file.

**Parameters:**
- `file_path` (required): Path to the file relative to the configured base path

**Example Response:**
```json
{
  "file_path": "main.go",
  "size_bytes": 1234,
  "content": "package main\n\nimport \"fmt\"\n..."
}
```

### 3. grep_search

Performs grep searches with context lines. Supports up to 20 search queries in a single request.

**Parameters:**
- `queries` (required): JSON string containing array of search query objects
  - `pattern` (required): Search pattern/regex
  - `file_pattern` (optional): File pattern to limit search (e.g., "*.go")
  - `ignore_case` (optional): Case-insensitive search
- `context_lines` (optional): Number of lines before and after each match (default: 5)

**Example Request:**
```json
{
  "queries": "[{\"pattern\": \"func main\", \"file_pattern\": \"*.go\", \"ignore_case\": false}, {\"pattern\": \"TODO\", \"ignore_case\": true}]",
  "context_lines": 3
}
```

**Example Response:**
```json
{
  "base_path": "/path/to/files",
  "context_lines": 3,
  "results": [
    {
      "query": "func main",
      "matches": [
        {
          "file_path": "main.go",
          "lines": [
            {
              "line_number": 5,
              "content": "import \"fmt\"",
              "is_match": false
            },
            {
              "line_number": 7,
              "content": "func main() {",
              "is_match": true
            },
            {
              "line_number": 8,
              "content": "    fmt.Println(\"Hello\")",
              "is_match": false
            }
          ]
        }
      ]
    }
  ]
}
```

## Security Features

- **Path Validation**: Prevents directory traversal attacks (no `../` allowed)
- **Base Path Restriction**: All file access is restricted to the configured base path
- **File Size Limits**: Configurable maximum file size to prevent reading huge files
- **Read-Only Access**: No write, delete, or modify operations supported
- **Query Limits**: Maximum 20 grep queries per request to prevent abuse

## Configuration

The server uses a streamable HTTP transport that supports both direct HTTP responses and SSE streams for real-time communication with MCP clients.

### Example MCP Client Configuration

For Claude Desktop, add this to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "/path/to/mcp-server",
      "args": ["-base-path", "/path/to/your/files"],
      "env": {}
    }
  }
}
```

## Development

This implementation uses the `mark3labs/mcp-go` library, which is the most mature and widely adopted Go MCP library.

### Architecture

- **Config**: Server configuration with validation
- **MCPFileServer**: Main server struct handling MCP protocol
- **Tool Handlers**: Individual implementations for each filesystem tool
- **Security**: Path validation and access control
- **Error Handling**: Comprehensive error handling with user-friendly messages

### Error Handling

All tools provide detailed error messages for common issues:
- File not found
- Permission denied
- Path traversal attempts
- File too large
- Invalid patterns
- Grep command failures

## Performance Considerations

- **File Size Limits**: Prevents memory issues with large files
- **Depth Limits**: Optional depth limiting for large directory trees
- **Pattern Filtering**: Reduces results to relevant files only
- **Efficient Grep**: Uses native `grep` command for fast text searching

## License

This project is licensed under the MIT License.