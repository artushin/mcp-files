package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// GitignoreFilter handles .gitignore pattern matching
type GitignoreFilter struct {
	patterns []string
	basePath string
}

// NewGitignoreFilter creates a new gitignore filter
func NewGitignoreFilter(basePath string) *GitignoreFilter {
	filter := &GitignoreFilter{
		patterns: []string{".git", ".git/"}, // Always ignore .git directory
		basePath: basePath,
	}

	// Try to read .gitignore file
	gitignorePath := filepath.Join(basePath, ".gitignore")
	if file, err := os.Open(gitignorePath); err == nil {
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// TODO: Handle negation patterns (!) if needed
			// For now, we'll just add positive patterns
			if !strings.HasPrefix(line, "!") {
				filter.patterns = append(filter.patterns, line)
			}
		}
	}

	return filter
}

// ShouldIgnore checks if a file/directory should be ignored
func (f *GitignoreFilter) ShouldIgnore(path string) bool {
	// Get relative path from base
	relPath, err := filepath.Rel(f.basePath, path)
	if err != nil {
		return false
	}

	// Always ignore .git directory
	if strings.HasPrefix(relPath, ".git") || strings.Contains(relPath, "/.git") {
		return true
	}

	fileName := filepath.Base(path)

	for _, pattern := range f.patterns {
		// Handle directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if matched, _ := filepath.Match(dirPattern, fileName); matched {
				return true
			}
			// Also check if any parent directory matches
			if strings.Contains(relPath, dirPattern+"/") {
				return true
			}
		} else {
			// File or directory pattern
			if matched, _ := filepath.Match(pattern, fileName); matched {
				return true
			}
			// Check full relative path for patterns with /
			if strings.Contains(pattern, "/") {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return true
				}
				// Also check if pattern matches any part of the path
				pathParts := strings.Split(relPath, "/")
				for i := range pathParts {
					subPath := strings.Join(pathParts[i:], "/")
					if matched, _ := filepath.Match(pattern, subPath); matched {
						return true
					}
				}
			}
		}
	}

	return false
}

// handleReadFileStructure handles the read_file_structure tool with filtering
func (s *MCPFileServer) handleReadFileStructure(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Create gitignore filter
	filter := NewGitignoreFilter(s.config.BasePath)

	// Build file tree with filtering
	root, err := s.buildFileTreeWithFilter(s.config.BasePath, 0, filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read file structure: %v", err)), nil
	}

	// Create result as JSON text
	result := map[string]interface{}{
		"base_path": s.config.BasePath,
		"structure": root,
		"note":      "Filtered out .git directory and .gitignore patterns",
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// buildFileTreeWithFilter recursively builds a file tree structure with gitignore filtering
func (s *MCPFileServer) buildFileTreeWithFilter(dirPath string, currentDepth int, filter *GitignoreFilter) (*FileNode, error) {
	// Check if this path should be ignored
	if filter.ShouldIgnore(dirPath) {
		return nil, nil
	}

	stat, err := os.Stat(dirPath)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(s.config.BasePath, dirPath)
	if relPath == "." {
		relPath = ""
	}

	node := &FileNode{
		Name: filepath.Base(dirPath),
		Path: relPath,
	}

	if stat.IsDir() {
		node.Type = "directory"

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			childPath := filepath.Join(dirPath, entry.Name())

			// Skip if should be ignored
			if filter.ShouldIgnore(childPath) {
				continue
			}

			child, err := s.buildFileTreeWithFilter(childPath, currentDepth+1, filter)
			if err != nil {
				continue // Skip entries that cause errors
			}
			if child != nil {
				node.Children = append(node.Children, child)
			}
		}
	} else {
		node.Type = "file"
		size := stat.Size()
		node.Size = &size
	}

	return node, nil
}
