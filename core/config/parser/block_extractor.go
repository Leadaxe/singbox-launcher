package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// ExtractParserConfigBlock extracts the @ParserConfig block from config file content using regex
// Returns the JSON content string from the block, or error if block not found
// This is a pure parsing function that doesn't depend on core types
func ExtractParserConfigBlock(data []byte) (string, error) {
	// Find the @ParserConfig block using regex
	// Pattern matches: /** @ParserConfig ... */
	pattern := regexp.MustCompile(`/\*\*\s*@ParserConfig\s*\n([\s\S]*?)\*/`)
	matches := pattern.FindSubmatch(data)

	if len(matches) < 2 {
		return "", fmt.Errorf("@ParserConfig block not found in config.json")
	}

	// Extract the JSON content from the comment block
	jsonContent := strings.TrimSpace(string(matches[1]))
	return jsonContent, nil
}
