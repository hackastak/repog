package format

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFormatStarsZero(t *testing.T) {
	result := FormatStars(0)
	if result != "0" {
		t.Errorf("FormatStars(0) = %q, want %q", result, "0")
	}
}

func TestFormatStars999(t *testing.T) {
	result := FormatStars(999)
	if result != "999" {
		t.Errorf("FormatStars(999) = %q, want %q", result, "999")
	}
}

func TestFormatStars1000(t *testing.T) {
	result := FormatStars(1000)
	if result != "1.0k" {
		t.Errorf("FormatStars(1000) = %q, want %q", result, "1.0k")
	}
}

func TestFormatStars1234(t *testing.T) {
	result := FormatStars(1234)
	if result != "1.2k" {
		t.Errorf("FormatStars(1234) = %q, want %q", result, "1.2k")
	}
}

func TestFormatStars10000(t *testing.T) {
	result := FormatStars(10000)
	if result != "10k" {
		t.Errorf("FormatStars(10000) = %q, want %q", result, "10k")
	}
}

func TestWrapTextWrapsAtCorrectBoundary(t *testing.T) {
	text := "This is a long sentence that should wrap at the specified width boundary."
	result := WrapText(text, 30, 0)

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if len(line) > 30 {
			t.Errorf("Line exceeds max width: %q (len=%d)", line, len(line))
		}
	}
}

func TestWrapTextIndentsContinuationLines(t *testing.T) {
	text := "First line content here and more content to force wrapping"
	result := WrapText(text, 25, 4)

	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Skip("Text didn't wrap, skipping indent test")
	}

	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "    ") {
			t.Errorf("Continuation line %d not indented: %q", i, lines[i])
		}
	}
}

func TestSeparator60(t *testing.T) {
	result := Separator(60)
	runeCount := utf8.RuneCountInString(result)
	if runeCount != 60 {
		t.Errorf("Separator(60) rune count = %d, want 60", runeCount)
	}

	for _, r := range result {
		if r != '─' {
			t.Errorf("Separator contains unexpected character: %c", r)
		}
	}
}

func TestSeparatorCapsAt60(t *testing.T) {
	result := Separator(100)
	runeCount := utf8.RuneCountInString(result)
	if runeCount != 60 {
		t.Errorf("Separator(100) rune count = %d, want 60 (capped)", runeCount)
	}
}

func TestFormatSimilarity(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.853, "85.3%"},
		{1.0, "100.0%"},
		{0.0, "0.0%"},
		{0.5, "50.0%"},
	}

	for _, tt := range tests {
		result := FormatSimilarity(tt.input)
		if result != tt.expected {
			t.Errorf("FormatSimilarity(%f) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateText(t *testing.T) {
	text := "This is a long text that should be truncated"

	result := TruncateText(text, 20)
	if len(result) > 20 {
		t.Errorf("TruncateText result too long: %q (len=%d)", result, len(result))
	}

	if !strings.HasSuffix(result, "...") {
		t.Errorf("TruncateText should end with ellipsis: %q", result)
	}
}

func TestTruncateTextNoTruncation(t *testing.T) {
	text := "Short text"
	result := TruncateText(text, 100)
	if result != text {
		t.Errorf("TruncateText should not truncate: got %q, want %q", result, text)
	}
}

func TestRedactSensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Error with github_pat_12345678901234567890 token", "Error with [REDACTED] token"},
		{"Using ghp_abcdefghijklmnopqrstuvwxyz1234567890", "Using [REDACTED]"},
		{"API key AIzaSyD-12345678901234567890123456", "API key [REDACTED]"},
		{"No sensitive data here", "No sensitive data here"},
	}

	for _, tt := range tests {
		result := RedactSensitive(tt.input)
		if result != tt.expected {
			t.Errorf("RedactSensitive(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatStarsNegative(t *testing.T) {
	result := FormatStars(-1)
	if result != "-1" {
		t.Errorf("FormatStars(-1) = %q, want %q", result, "-1")
	}
}

func TestFormatStars50000(t *testing.T) {
	result := FormatStars(50000)
	if result != "50k" {
		t.Errorf("FormatStars(50000) = %q, want %q", result, "50k")
	}
}

func TestFormatStars9999(t *testing.T) {
	result := FormatStars(9999)
	if result != "10.0k" {
		t.Errorf("FormatStars(9999) = %q, want %q", result, "10.0k")
	}
}

func TestWrapTextEmpty(t *testing.T) {
	result := WrapText("", 80, 0)
	if result != "" {
		t.Errorf("WrapText empty string should return empty, got %q", result)
	}
}

func TestWrapTextWhitespaceOnly(t *testing.T) {
	result := WrapText("   \t\n   ", 80, 0)
	if result != "" {
		t.Errorf("WrapText whitespace only should return empty, got %q", result)
	}
}

func TestWrapTextNoWrapNeeded(t *testing.T) {
	text := "Short text"
	result := WrapText(text, 80, 0)
	if result != text {
		t.Errorf("WrapText short text should not wrap, got %q", result)
	}
}

func TestSeparatorMinimum(t *testing.T) {
	result := Separator(0)
	runeCount := utf8.RuneCountInString(result)
	if runeCount != 1 {
		t.Errorf("Separator(0) should return 1 char, got %d", runeCount)
	}
}

func TestSeparatorNegative(t *testing.T) {
	result := Separator(-5)
	runeCount := utf8.RuneCountInString(result)
	if runeCount != 1 {
		t.Errorf("Separator(-5) should return 1 char, got %d", runeCount)
	}
}

func TestFormatRelativeTimeInvalid(t *testing.T) {
	result := FormatRelativeTime("not-a-date")
	if result != "not-a-date" {
		t.Errorf("FormatRelativeTime invalid should return original, got %q", result)
	}
}

func TestTruncateTextVeryShortMax(t *testing.T) {
	result := TruncateText("Hello World", 3)
	if len(result) != 3 {
		t.Errorf("TruncateText with maxLength=3 should return 3 chars, got %d", len(result))
	}
}

func TestTruncateTextExactLength(t *testing.T) {
	text := "Hello"
	result := TruncateText(text, 5)
	if result != text {
		t.Errorf("TruncateText at exact length should not truncate, got %q", result)
	}
}

func TestRedactSensitiveMultipleTokens(t *testing.T) {
	input := "Token1: github_pat_xxx Token2: ghp_yyy"
	result := RedactSensitive(input)
	if strings.Contains(result, "github_pat_") || strings.Contains(result, "ghp_") {
		t.Errorf("RedactSensitive should redact all tokens, got %q", result)
	}
}

func TestTerminalWidth(t *testing.T) {
	width := TerminalWidth()
	// Should return a positive value (either actual width or default 80)
	if width <= 0 {
		t.Errorf("TerminalWidth should return positive value, got %d", width)
	}
}

func TestFormatSimilarityEdgeCases(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.001, "0.1%"},
		{0.999, "99.9%"},
		{-0.5, "-50.0%"},
	}

	for _, tt := range tests {
		result := FormatSimilarity(tt.input)
		if result != tt.expected {
			t.Errorf("FormatSimilarity(%f) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatRelativeTime_JustNow(t *testing.T) {
	// Time 30 seconds ago
	pastTime := time.Now().Add(-30 * time.Second)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "just now" {
		t.Errorf("FormatRelativeTime(-30s) = %q, want %q", result, "just now")
	}
}

func TestFormatRelativeTime_MinutesAgo(t *testing.T) {
	// Time 5 minutes ago
	pastTime := time.Now().Add(-5 * time.Minute)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "5 minutes ago" {
		t.Errorf("FormatRelativeTime(-5m) = %q, want %q", result, "5 minutes ago")
	}
}

func TestFormatRelativeTime_OneMinuteAgo(t *testing.T) {
	// Time 1 minute ago (use 65 seconds to avoid edge case)
	pastTime := time.Now().Add(-65 * time.Second)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "1 minute ago" {
		t.Errorf("FormatRelativeTime(-1m) = %q, want %q", result, "1 minute ago")
	}
}

func TestFormatRelativeTime_HoursAgo(t *testing.T) {
	// Time 3 hours ago
	pastTime := time.Now().Add(-3 * time.Hour)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "3 hours ago" {
		t.Errorf("FormatRelativeTime(-3h) = %q, want %q", result, "3 hours ago")
	}
}

func TestFormatRelativeTime_OneHourAgo(t *testing.T) {
	// Time 1 hour ago (use 65 minutes to avoid edge case)
	pastTime := time.Now().Add(-65 * time.Minute)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "1 hour ago" {
		t.Errorf("FormatRelativeTime(-1h) = %q, want %q", result, "1 hour ago")
	}
}

func TestFormatRelativeTime_DaysAgo(t *testing.T) {
	// Time 2 days ago
	pastTime := time.Now().Add(-2 * 24 * time.Hour)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "2 days ago" {
		t.Errorf("FormatRelativeTime(-2d) = %q, want %q", result, "2 days ago")
	}
}

func TestFormatRelativeTime_OneDayAgo(t *testing.T) {
	// Time 1 day ago (use 25 hours to avoid edge case)
	pastTime := time.Now().Add(-25 * time.Hour)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	if result != "1 day ago" {
		t.Errorf("FormatRelativeTime(-1d) = %q, want %q", result, "1 day ago")
	}
}

func TestFormatRelativeTime_WeeksAgo(t *testing.T) {
	// Time 14 days ago (2 weeks)
	pastTime := time.Now().Add(-14 * 24 * time.Hour)
	result := FormatRelativeTime(pastTime.Format(time.RFC3339))

	// Should be "14 days ago" based on the implementation (no weeks support)
	if !strings.Contains(result, "days ago") {
		t.Errorf("FormatRelativeTime(-14d) = %q, want something containing 'days ago'", result)
	}
}

func TestFormatRelativeTime_FutureTime(t *testing.T) {
	// Time 1 hour in the future
	futureTime := time.Now().Add(1 * time.Hour)
	result := FormatRelativeTime(futureTime.Format(time.RFC3339))

	// Just verify no panic and some output
	if result == "" {
		t.Error("FormatRelativeTime for future time should return non-empty string")
	}

	// Future times should contain "in"
	if !strings.Contains(result, "in") {
		t.Errorf("FormatRelativeTime for future time should contain 'in', got %q", result)
	}
}

func TestTerminalWidth_DefaultsTo80WhenNotATerminal(t *testing.T) {
	// In test environments, os.Stdout is not a TTY, so the fallback path runs
	width := TerminalWidth()

	// Assert: result >= 80 (the fallback value)
	if width < 80 {
		t.Errorf("TerminalWidth should return at least 80 in non-TTY environment, got %d", width)
	}

	// In CI/test environments, it should return exactly 80
	if width != 80 {
		t.Logf("TerminalWidth returned %d (may be running in actual terminal)", width)
	}
}
