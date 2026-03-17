import { GoogleGenerativeAI } from '@google/generative-ai';
import { redactSensitive } from '../utils/format.js';

/**
 * Result of Gemini API key validation.
 */
export interface GeminiAuthResult {
  /** Whether the API key is valid */
  valid: boolean;
  /** Model name that was successfully accessed */
  model?: string;
  /** Error message if validation failed */
  error?: string;
}

/**
 * The model to use for validation.
 * Using gemini-2.5-flash as it's the current stable standard in March 2026.
 */
const VALIDATION_MODEL = 'gemini-2.5-flash';

/**
 * Fallback models to try if the primary validation model is not found.
 * This adds resilience against model deprecation or regional availability issues.
 * gemini-2.0-flash is still available until March 31, 2026.
 */
const FALLBACK_MODELS = ['gemini-2.0-flash', 'gemini-3.1-flash'];

/**
 * Validate a Google Gemini API key.
 *
 * Makes a minimal API call to verify the key is valid and has access
 * to the Gemini API. Does not throw - returns structured result.
 *
 * @param apiKey - The Gemini API key to validate
 * @returns Validation result indicating success or failure
 */
export async function validateGeminiKey(apiKey: string): Promise<GeminiAuthResult> {
  if (!apiKey || apiKey.trim() === '') {
    return {
      valid: false,
      error: 'API key is empty',
    };
  }

  const modelsToTry = [VALIDATION_MODEL, ...FALLBACK_MODELS];
  let lastError: Error | unknown;

  for (const modelName of modelsToTry) {
    try {
      const genAI = new GoogleGenerativeAI(apiKey);
      const model = genAI.getGenerativeModel({ model: modelName });

      // Make a minimal request to validate the key
      // Using countTokens as it's fast and doesn't consume quota
      await model.countTokens('test');

      return {
        valid: true,
        model: modelName,
      };
    } catch (error) {
      lastError = error;
      if (error instanceof Error) {
        const message = error.message.toLowerCase();

        // Check for authentication errors - fail immediately
        if (message.includes('api key') || message.includes('invalid') || message.includes('401')) {
          return {
            valid: false,
            error: 'Invalid API key',
          };
        }

        // Check for permission errors - fail immediately
        if (message.includes('permission') || message.includes('403')) {
          return {
            valid: false,
            error: 'API key lacks required permissions',
          };
        }

        // Check for quota/rate limit errors (key is valid but limited)
        if (message.includes('quota') || message.includes('rate limit') || message.includes('429')) {
          return {
            valid: true,
            model: modelName,
            error: 'API key is valid but rate limited or quota exceeded',
          };
        }

        // If it's a 404 (model not found), continue to the next model
        if (message.includes('not found') || message.includes('404')) {
          continue;
        }

        // For other errors, we might want to fail immediately or continue.
        // Failing safe by returning the error here.
        return {
          valid: false,
          error: `Failed to validate API key: ${redactSensitive(error.message)}`,
        };
      }
    }
  }

  // If we get here, all models failed
  if (lastError instanceof Error) {
    return {
      valid: false,
      error: `Failed to validate API key: ${redactSensitive(lastError.message)}`,
    };
  }

  return {
    valid: false,
    error: 'Unknown error validating API key',
  };
}

/**
 * Get the model name used for validation.
 *
 * @returns The Gemini model name used for API key validation
 */
export function getValidationModel(): string {
  return VALIDATION_MODEL;
}
