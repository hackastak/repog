import { describe, it, expect } from 'vitest';
import {
  formatRelativeTime,
  wrapText,
  formatStars,
  formatSimilarity,
  truncateText,
  redactSensitive,
} from './format.js';

describe('formatRelativeTime', () => {
  it('returns "just now" for timestamps within the last 60 seconds', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 30 * 1000); // 30 seconds ago
    expect(formatRelativeTime(past.toISOString())).toBe('just now');
  });

  it('returns "X minutes ago" for timestamps 1–59 minutes ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 5 * 60 * 1000); // 5 minutes ago
    expect(formatRelativeTime(past.toISOString())).toBe('5 minutes ago');

    const past2 = new Date(now.getTime() - 59 * 60 * 1000); // 59 minutes ago
    expect(formatRelativeTime(past2.toISOString())).toBe('59 minutes ago');
  });

  it('returns "X hours ago" for timestamps 1–23 hours ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 2 * 60 * 60 * 1000); // 2 hours ago
    expect(formatRelativeTime(past.toISOString())).toBe('2 hours ago');

    const past2 = new Date(now.getTime() - 23 * 60 * 60 * 1000); // 23 hours ago
    expect(formatRelativeTime(past2.toISOString())).toBe('23 hours ago');
  });

  it('returns "X days ago" for timestamps 1+ days ago', () => {
    const now = new Date();
    const past = new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000); // 2 days ago
    expect(formatRelativeTime(past.toISOString())).toBe('2 days ago');
  });

  it('returns "in X minutes" for future timestamps', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 15 * 60 * 1000); // in 15 minutes
    expect(formatRelativeTime(future.toISOString())).toBe('in 15 minutes');
  });

  it('returns "in X hours" for future timestamps more than 60 minutes away', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 2 * 60 * 60 * 1000); // in 2 hours
    expect(formatRelativeTime(future.toISOString())).toBe('in 2 hours');
  });

  it('handles singular units correctly', () => {
    const now = new Date();
    
    // 1 minute ago
    const pastMin = new Date(now.getTime() - 60 * 1000);
    expect(formatRelativeTime(pastMin.toISOString())).toBe('1 minute ago');

    // 1 hour ago
    const pastHour = new Date(now.getTime() - 60 * 60 * 1000);
    expect(formatRelativeTime(pastHour.toISOString())).toBe('1 hour ago');

    // 1 day ago
    const pastDay = new Date(now.getTime() - 24 * 60 * 60 * 1000);
    expect(formatRelativeTime(pastDay.toISOString())).toBe('1 day ago');

    // in 1 minute
    const futureMin = new Date(now.getTime() + 60 * 1000);
    expect(formatRelativeTime(futureMin.toISOString())).toBe('in 1 minute');

    // in 1 hour
    const futureHour = new Date(now.getTime() + 60 * 60 * 1000);
    expect(formatRelativeTime(futureHour.toISOString())).toBe('in 1 hour');
  });

  it('returns "in X days" for future timestamps more than 24 hours away', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 3 * 24 * 60 * 60 * 1000); // in 3 days
    expect(formatRelativeTime(future.toISOString())).toBe('in 3 days');
  });

  it('handles singular day in future', () => {
    const now = new Date();
    const future = new Date(now.getTime() + 24 * 60 * 60 * 1000); // in 1 day
    expect(formatRelativeTime(future.toISOString())).toBe('in 1 day');
  });
});

describe('wrapText', () => {
  it('returns empty string for empty input', () => {
    expect(wrapText('')).toBe('');
  });

  it('returns empty string for whitespace-only input', () => {
    expect(wrapText('   ')).toBe('');
  });

  it('wraps text at word boundaries', () => {
    const text = 'This is a test of word wrapping functionality';
    const result = wrapText(text, { width: 20 });

    const lines = result.split('\n');
    expect(lines.every(line => line.length <= 20)).toBe(true);
  });

  it('uses default width of 80', () => {
    const text = 'A'.repeat(100);
    const result = wrapText(text);

    // Single word longer than width is kept intact
    expect(result).toBe('A'.repeat(100));
  });

  it('applies first line indent correctly', () => {
    const text = 'Short text';
    const result = wrapText(text, { firstLineIndent: '>> ' });

    expect(result).toBe('>> Short text');
  });

  it('applies continuation indent correctly', () => {
    const text = 'This is a longer text that needs to wrap to multiple lines';
    const result = wrapText(text, { width: 25, indent: '  ' });

    const lines = result.split('\n');
    expect(lines[0]).toBe('This is a longer text');
    expect(lines[1].startsWith('  ')).toBe(true);
  });

  it('handles words longer than width', () => {
    const text = 'Short superlongwordthatexceedswidth end';
    const result = wrapText(text, { width: 15 });

    const lines = result.split('\n');
    expect(lines).toContain('superlongwordthatexceedswidth');
  });

  it('normalizes multiple spaces', () => {
    const text = 'Text   with    multiple     spaces';
    const result = wrapText(text);

    expect(result).toBe('Text with multiple spaces');
  });
});

describe('formatStars', () => {
  it('formats small numbers without separators', () => {
    expect(formatStars(5)).toBe('5');
    expect(formatStars(100)).toBe('100');
  });

  it('formats large numbers with comma separators', () => {
    expect(formatStars(1000)).toBe('1,000');
    expect(formatStars(1234567)).toBe('1,234,567');
  });

  it('handles zero', () => {
    expect(formatStars(0)).toBe('0');
  });
});

describe('formatSimilarity', () => {
  it('formats 0 as 0.0%', () => {
    expect(formatSimilarity(0)).toBe('0.0%');
  });

  it('formats 1 as 100.0%', () => {
    expect(formatSimilarity(1)).toBe('100.0%');
  });

  it('formats decimal values with one decimal place', () => {
    expect(formatSimilarity(0.853)).toBe('85.3%');
    expect(formatSimilarity(0.5)).toBe('50.0%');
    expect(formatSimilarity(0.999)).toBe('99.9%');
  });
});

describe('truncateText', () => {
  it('returns original text if within limit', () => {
    const text = 'Short text';
    expect(truncateText(text, 20)).toBe('Short text');
  });

  it('truncates long text with ellipsis', () => {
    const text = 'This is a much longer text that exceeds the limit';
    const result = truncateText(text, 20);

    expect(result.length).toBe(20);
    expect(result.endsWith('...')).toBe(true);
  });

  it('uses default max length of 200', () => {
    const text = 'A'.repeat(250);
    const result = truncateText(text);

    expect(result.length).toBe(200);
    expect(result.endsWith('...')).toBe(true);
  });

  it('replaces newlines with spaces', () => {
    const text = 'Line 1\nLine 2\nLine 3';
    const result = truncateText(text, 100);

    expect(result).toBe('Line 1 Line 2 Line 3');
  });

  it('normalizes multiple spaces', () => {
    const text = 'Text   with    multiple     spaces';
    const result = truncateText(text, 100);

    expect(result).toBe('Text with multiple spaces');
  });

  it('handles empty string', () => {
    expect(truncateText('', 10)).toBe('');
  });
});

describe('redactSensitive', () => {
  describe('GitHub fine-grained PATs', () => {
    it('redacts fine-grained PATs (github_pat_)', () => {
      const str = 'Error with token github_pat_11ABCDEFGH0123456789_abcdefghijklmnopqrstuvwxyzABCDEFGH';
      const result = redactSensitive(str);

      expect(result).toContain('[REDACTED]');
      expect(result).not.toContain('github_pat_');
    });

    it('redacts multiple fine-grained PATs', () => {
      const str = 'Token1: github_pat_AAAAAAAAAAAAAAAAAAAAAA and Token2: github_pat_BBBBBBBBBBBBBBBBBBBBBB';
      const result = redactSensitive(str);

      expect(result.match(/\[REDACTED\]/g)?.length).toBe(2);
    });
  });

  describe('GitHub classic PATs', () => {
    it('redacts classic PATs (ghp_)', () => {
      const str = 'Using token ghp_1234567890abcdefghijklmnopqrstuvwxyzAB';
      const result = redactSensitive(str);

      expect(result).toContain('[REDACTED]');
      expect(result).not.toContain('ghp_');
    });
  });

  describe('Gemini API keys', () => {
    it('redacts Gemini API keys (AIza)', () => {
      const str = 'API key: AIzaSyC1234567890abcdefghijklmnopqrstuvwx';
      const result = redactSensitive(str);

      expect(result).toContain('[REDACTED]');
      expect(result).not.toContain('AIza');
    });
  });

  describe('mixed content', () => {
    it('redacts multiple different token types', () => {
      // Use realistic token lengths: github_pat_ + 22 chars, AIza + 35 chars
      const str = 'GitHub: github_pat_ABCDEFGHIJKLMNOPQRSTUV and Gemini: AIzaSyC12345678901234567890123456789012';
      const result = redactSensitive(str);

      expect(result.match(/\[REDACTED\]/g)?.length).toBe(2);
      expect(result).not.toContain('github_pat_');
      expect(result).not.toContain('AIza');
    });

    it('preserves non-sensitive content', () => {
      const str = 'Error: connection failed to github.com with status 401';
      const result = redactSensitive(str);

      expect(result).toBe(str);
    });
  });

  describe('edge cases', () => {
    it('handles empty string', () => {
      expect(redactSensitive('')).toBe('');
    });

    it('handles string without sensitive data', () => {
      const str = 'Just a normal error message';
      expect(redactSensitive(str)).toBe(str);
    });

    it('does not redact partial matches', () => {
      const str = 'ghp_short'; // Too short to be a real token
      expect(redactSensitive(str)).toBe(str);
    });
  });
});
