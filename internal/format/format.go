// Package format provides text formatting utilities for CLI output.
package format

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// FormatStars formats a star count as "1.2k" for >= 1000, "342" for < 1000.
func FormatStars(count int) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 10000 {
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	return fmt.Sprintf("%dk", count/1000)
}

// WrapText wraps text at maxWidth characters, indenting continuation lines
// with indent spaces.
func WrapText(text string, maxWidth, indent int) string {
	// Normalize whitespace
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}

	words := strings.Split(text, " ")
	var lines []string
	var currentLine strings.Builder
	indentStr := strings.Repeat(" ", indent)

	isFirstLine := true
	for _, word := range words {
		lineLen := currentLine.Len()
		wordLen := len(word)

		// Calculate effective width for this line
		effectiveWidth := maxWidth
		if !isFirstLine {
			effectiveWidth -= indent
		}

		// Check if adding this word would exceed width
		if lineLen > 0 && lineLen+1+wordLen > effectiveWidth {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			isFirstLine = false
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	// Add indentation to continuation lines
	for i := 1; i < len(lines); i++ {
		lines[i] = indentStr + lines[i]
	}

	return strings.Join(lines, "\n")
}

// Separator returns a string of '─' characters at the given width (max 60).
func Separator(width int) string {
	if width > 60 {
		width = 60
	}
	if width < 1 {
		width = 1
	}
	return strings.Repeat("─", width)
}

// TerminalWidth returns the current terminal width, defaulting to 80.
func TerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	return width
}

// FormatSimilarity formats a similarity score as a percentage.
func FormatSimilarity(similarity float64) string {
	return fmt.Sprintf("%.1f%%", similarity*100)
}

// TruncateText truncates text to a maximum length with ellipsis.
func TruncateText(text string, maxLength int) string {
	// Normalize whitespace
	cleaned := strings.Join(strings.Fields(text), " ")
	if len(cleaned) <= maxLength {
		return cleaned
	}
	if maxLength <= 3 {
		return cleaned[:maxLength]
	}
	return cleaned[:maxLength-3] + "..."
}

// FormatRelativeTime formats a relative time string (e.g., "2 hours ago", "in 14 minutes").
func FormatRelativeTime(isoString string) string {
	t, err := time.Parse(time.RFC3339, isoString)
	if err != nil {
		// Try other common formats
		t, err = time.Parse("2006-01-02T15:04:05Z", isoString)
		if err != nil {
			return isoString
		}
	}

	now := time.Now()
	diff := now.Sub(t)

	if diff >= 0 {
		// Past
		seconds := int(diff.Seconds())
		if seconds < 60 {
			return "just now"
		}

		minutes := seconds / 60
		if minutes < 60 {
			if minutes == 1 {
				return "1 minute ago"
			}
			return fmt.Sprintf("%d minutes ago", minutes)
		}

		hours := minutes / 60
		if hours < 24 {
			if hours == 1 {
				return "1 hour ago"
			}
			return fmt.Sprintf("%d hours ago", hours)
		}

		days := hours / 24
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	// Future
	absDiff := -diff
	seconds := int(absDiff.Seconds())

	minutes := (seconds + 59) / 60 // Round up
	if minutes < 60 {
		if minutes == 1 {
			return "in 1 minute"
		}
		return fmt.Sprintf("in %d minutes", minutes)
	}

	hours := minutes / 60
	if hours < 24 {
		if hours == 1 {
			return "in 1 hour"
		}
		return fmt.Sprintf("in %d hours", hours)
	}

	days := hours / 24
	if days == 1 {
		return "in 1 day"
	}
	return fmt.Sprintf("in %d days", days)
}

// RedactSensitive redacts sensitive information from a string.
func RedactSensitive(str string) string {
	// Patterns for sensitive data
	patterns := []string{
		"github_pat_",
		"ghp_",
		"AIza",
	}

	result := str
	for _, pattern := range patterns {
		for {
			idx := strings.Index(result, pattern)
			if idx == -1 {
				break
			}
			// Find the end of the token (whitespace or end of string)
			end := idx + len(pattern)
			for end < len(result) && !isWhitespace(result[end]) {
				end++
			}
			result = result[:idx] + "[REDACTED]" + result[end:]
		}
	}

	return result
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
