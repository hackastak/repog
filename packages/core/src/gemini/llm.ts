import { GoogleGenerativeAI } from '@google/generative-ai';

/**
 * The LLM model to use for text generation.
 */
const LLM_MODEL = 'gemini-2.0-flash';

/**
 * Maximum output tokens for LLM responses.
 */
const MAX_OUTPUT_TOKENS = 1024;

/**
 * Temperature for LLM responses (lower = more focused).
 */
const TEMPERATURE = 0.3;

/**
 * Result of a successful LLM call.
 */
export interface LLMResult {
  /** The generated text response */
  text: string;
  /** Number of input tokens used */
  inputTokens: number;
  /** Number of output tokens generated */
  outputTokens: number;
  /** Duration of the call in milliseconds */
  durationMs: number;
}

/**
 * Result of a failed LLM call.
 */
export interface LLMError {
  /** Error message describing the failure */
  error: string;
  /** Duration of the call in milliseconds */
  durationMs: number;
}

/**
 * Type guard to check if a result is an LLMError.
 *
 * @param result - The result to check
 * @returns True if the result is an LLMError
 */
export function isLLMError(result: LLMResult | LLMError): result is LLMError {
  return 'error' in result && !('text' in result);
}

/**
 * Call the Gemini LLM with a prompt and optional system instruction.
 *
 * This function is a reusable primitive for text generation with Gemini.
 * It handles all error cases gracefully and never throws.
 *
 * @param apiKey - The Gemini API key
 * @param prompt - The user prompt to send
 * @param systemPrompt - Optional system instruction for the model
 * @returns LLMResult on success, LLMError on failure
 */
/**
 * Callback function invoked for each streamed text chunk.
 */
export type OnChunkCallback = (text: string) => void;

export async function callLLM(
  apiKey: string,
  prompt: string,
  systemPrompt?: string
): Promise<LLMResult | LLMError> {
  const startTime = performance.now();

  try {
    const genAI = new GoogleGenerativeAI(apiKey);

    // Build model configuration
    const modelConfig: {
      model: string;
      generationConfig: {
        maxOutputTokens: number;
        temperature: number;
      };
      systemInstruction?: string;
    } = {
      model: LLM_MODEL,
      generationConfig: {
        maxOutputTokens: MAX_OUTPUT_TOKENS,
        temperature: TEMPERATURE,
      },
    };

    // Add system instruction if provided
    if (systemPrompt) {
      modelConfig.systemInstruction = systemPrompt;
    }

    const model = genAI.getGenerativeModel(modelConfig);

    // Generate content
    const result = await model.generateContent(prompt);
    const response = result.response;

    const durationMs = performance.now() - startTime;

    // Extract text from response
    const text = response.text();

    // Extract token counts from usage metadata
    const usageMetadata = response.usageMetadata;
    const inputTokens = usageMetadata?.promptTokenCount ?? 0;
    const outputTokens = usageMetadata?.candidatesTokenCount ?? 0;

    return {
      text,
      inputTokens,
      outputTokens,
      durationMs,
    };
  } catch (error) {
    const durationMs = performance.now() - startTime;
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';

    return {
      error: errorMessage,
      durationMs,
    };
  }
}

/**
 * Call the Gemini LLM with streaming enabled.
 *
 * This function streams the response token by token, calling the optional
 * onChunk callback for each text chunk as it arrives. The full response
 * is assembled and returned as an LLMResult.
 *
 * @param apiKey - The Gemini API key
 * @param prompt - The user prompt to send
 * @param systemPrompt - Optional system instruction for the model
 * @param onChunk - Optional callback invoked for each streamed text chunk
 * @returns LLMResult on success, LLMError on failure - never throws
 */
export async function streamLLM(
  apiKey: string,
  prompt: string,
  systemPrompt?: string,
  onChunk?: OnChunkCallback
): Promise<LLMResult | LLMError> {
  const startTime = performance.now();

  try {
    const genAI = new GoogleGenerativeAI(apiKey);

    // Build model configuration
    const modelConfig: {
      model: string;
      generationConfig: {
        maxOutputTokens: number;
        temperature: number;
      };
      systemInstruction?: string;
    } = {
      model: LLM_MODEL,
      generationConfig: {
        maxOutputTokens: MAX_OUTPUT_TOKENS,
        temperature: TEMPERATURE,
      },
    };

    // Add system instruction if provided
    if (systemPrompt) {
      modelConfig.systemInstruction = systemPrompt;
    }

    const model = genAI.getGenerativeModel(modelConfig);

    // Generate content with streaming
    const result = await model.generateContentStream(prompt);

    // Collect chunks as they arrive
    const chunks: string[] = [];
    for await (const chunk of result.stream) {
      const chunkText = chunk.text();
      chunks.push(chunkText);
      if (onChunk) {
        onChunk(chunkText);
      }
    }

    // Get the final aggregated response
    const response = await result.response;

    const durationMs = performance.now() - startTime;

    // Assemble the full text from chunks
    const text = chunks.join('');

    // Extract token counts from usage metadata
    const usageMetadata = response.usageMetadata;
    const inputTokens = usageMetadata?.promptTokenCount ?? 0;
    const outputTokens = usageMetadata?.candidatesTokenCount ?? 0;

    return {
      text,
      inputTokens,
      outputTokens,
      durationMs,
    };
  } catch (error) {
    const durationMs = performance.now() - startTime;
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';

    return {
      error: errorMessage,
      durationMs,
    };
  }
}
