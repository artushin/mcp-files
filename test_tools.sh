#!/bin/bash

# Example script to test the MCP server tools using curl
# Make sure the server is running first: go run .

SERVER_URL="http://localhost:8080/mcp"

echo "=== Testing MCP Filesystem Server ==="
echo

# Test 1: Read file structure
echo "1. Testing read_file_structure..."
curl -X POST "$SERVER_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "read_file_structure"
    }
  }' | jq .

echo -e "\n"

# Test 2: Read file contents (adjust path as needed)
echo "2. Testing read_file_contents..."
curl -X POST "$SERVER_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0", 
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "read_file_contents",
      "arguments": {
        "file_path": "main.go"
      }
    }
  }' | jq .

echo -e "\n"

# Test 3: Grep search
echo "3. Testing grep_search..."
curl -X POST "$SERVER_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3, 
    "method": "tools/call",
    "params": {
      "name": "grep_search",
      "arguments": {
        "queries": "[{\"pattern\": \"Mimic PVC endpoints\", \"file_pattern\": \"*.md\"}, {\"pattern\": \"video-foundation\", \"file_pattern\": \"*.md\"}]",
        "context_lines": 2
      }
    }
  }' | jq .

echo -e "\n=== Tests completed ==="