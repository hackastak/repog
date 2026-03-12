/**
 * @repog/core - Core library for RepoG
 *
 * This package provides the core functionality for the RepoG CLI:
 * - Database management with SQLite + sqlite-vec
 * - GitHub API integration
 * - Embeddings generation
 * - Semantic search
 * - RAG pipeline
 */

// Types
export * from './types/index.js';

// Config (keychain credential storage)
export {
  saveConfig,
  loadConfig,
  loadConfigAsync,
  clearConfig,
  isConfigured,
  isConfiguredAsync,
  getDefaultDbPath,
  migrateFromEncryptedConfig,
  checkConfigPermissions,
  type ConfigData,
  type SaveConfigResult,
  type ConfigPermissionResult,
} from './config/config.js';

// Database
export { getDb, closeDb, migrate } from './db/index.js';
export * from './db/schema.js';
export * from './db/init.js';

// GitHub Auth
export {
  validateGitHubToken,
  hasScope,
  getRequiredScopes,
  type PatValidationResult,
  type GitHubAuthResult,
} from './github/auth.js';

// GitHub Rate Limiter
export { RateLimiter, githubRateLimiter } from './github/rateLimiter.js';

// GitHub Client (RateLimitStats is exported from status module)
export { getRateLimitInfo } from './github/client.js';

// GitHub Repos
export type { GitHubRepo, FetchReposOptions } from './github/repos.js';
export {
  fetchOwnedRepos,
  fetchStarredRepos,
  fetchReadme,
  fetchFileTree,
} from './github/repos.js';

// Gemini Auth
export * from './gemini/auth.js';

// Gemini Embeddings
export {
  embedChunks,
  embedQuery,
  getEmbeddingModel,
  getEmbeddingDimensions,
  type EmbeddingResult,
  type EmbeddingError,
  type BatchEmbeddingResult,
} from './gemini/embeddings.js';

// Gemini LLM
export {
  callLLM,
  streamLLM,
  isLLMError,
  type LLMResult,
  type LLMError,
  type OnChunkCallback,
} from './gemini/llm.js';

// Legacy Auth (re-exports for backwards compatibility)
// Note: some functions have same names as config module but different signatures
export {
  getConfig,
  getGitHubToken,
  setGitHubToken,
  getGeminiApiKey,
  setGeminiApiKey,
  clearCredentials,
  // saveConfig and isConfigured are exported from config/config.js
} from './auth/index.js';

// Sync (legacy stubs)
export {
  syncRepos,
  syncRepo,
  getSyncState,
  resumeSync,
} from './sync/index.js';

// Sync / Ingestion
export type { IngestOptions, IngestProgressEvent } from './sync/ingest.js';
export { ingestRepos } from './sync/ingest.js';

// Embed
export {
  embedRepos,
  embedRepo,
  chunkRepo,
  generateEmbedding,
  getPendingEmbedCount,
  deleteEmbeddings,
  runEmbedPipeline,
  hasRepos,
  type EmbedPipelineOptions,
  type EmbedProgress,
} from './embed/index.js';

// Search
export {
  search,
  searchInRepo,
  findSimilarRepos,
  logQuery,
  searchRepos,
  type SearchFilters,
  type VectorSearchResult,
  type SearchQueryResult,
} from './search/index.js';

// RAG
export {
  ask,
  recommend,
  summarize,
  buildContext,
  generateResponse,
  getRepoForContext,
} from './rag/index.js';

// Recommend
export {
  recommendRepos,
  buildRecommendPrompt,
  type RecommendOptions,
  type Recommendation,
  type RecommendResult,
} from './recommend/index.js';

// Ask (Q&A)
export {
  askQuestion,
  buildAskPrompt,
  type AskOptions,
  type SourceAttribution,
  type AskResult,
} from './ask/index.js';

// Summarize
export {
  summarizeRepo,
  buildSummarizePrompt,
  type SummarizeOptions,
  type SummarizeResult,
} from './summarize/index.js';

// Utils
export {
  wrapText,
  formatStars,
  formatSimilarity,
  truncateText,
  formatRelativeTime,
  redactSensitive,
  type WrapTextOptions,
} from './utils/index.js';

// Status
export {
  getStatus,
  NotConfiguredError,
  type RepoStats,
  type SyncStats,
  type EmbedStats,
  type RateLimitStats,
  type DbStats,
  type StatusResult,
} from './status/index.js';

