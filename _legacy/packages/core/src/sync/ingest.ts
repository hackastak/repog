import { createHash } from 'crypto';
import { loadConfigAsync } from '../config/config.js';
import { getDb } from '../db/index.js';
import { GitHubClient } from '../github/client.js';
import {
  fetchFileTree,
  fetchOwnedRepos,
  fetchReadme,
  fetchStarredRepos,
  type GitHubRepo,
} from '../github/repos.js';
import { splitIntoChunks } from './chunker.js';

export interface IngestOptions {
  includeOwned: boolean;
  includeStarred: boolean;
  fullTree: boolean; // if true, always fetch file tree regardless of README length
  verbose: boolean;
}

export type IngestProgressEvent =
  | { type: 'repo'; repo: string; status: 'new' | 'updated' }
  | { type: 'skip'; repo: string; reason: 'unchanged' }
  | { type: 'error'; repo: string; error: string }
  | { type: 'done'; total: number; skipped: number; errors: number };

function hashPushedAt(pushedAt: string): string {
  return createHash('sha256').update(pushedAt).digest('hex');
}

export async function* ingestRepos(
  options: IngestOptions
): AsyncGenerator<IngestProgressEvent> {
  // 1. Load config
  const config = await loadConfigAsync();
  if (!config.githubPat) {
    throw new Error('GitHub PAT not configured');
  }

  // 2. Create client
  const client = new GitHubClient(config.githubPat);

  // 3. Open DB
  const db = getDb(config.dbPath);

  // 4. Collect repos
  const reposToProcess = new Map<
    string,
    { repo: GitHubRepo; isOwned: boolean; isStarred: boolean }
  >();

  try {
    if (options.includeOwned) {
      for await (const repo of fetchOwnedRepos(client)) {
        reposToProcess.set(repo.full_name, {
          repo,
          isOwned: true,
          isStarred: false,
        });
      }
    }

    if (options.includeStarred) {
      for await (const repo of fetchStarredRepos(client)) {
        const existing = reposToProcess.get(repo.full_name);
        if (existing) {
          existing.isStarred = true;
        } else {
          reposToProcess.set(repo.full_name, {
            repo,
            isOwned: false,
            isStarred: true,
          });
        }
      }
    }
  } catch (error: unknown) {
    // If fetching lists fails, we can't do much
    throw new Error(`Failed to fetch repositories: ${error instanceof Error ? error.message : error}`);
  }

  // 4b. Record sync start in sync_state
  const syncType = [options.includeOwned ? 'owned' : '', options.includeStarred ? 'starred' : ''].filter(Boolean).join('+');
  const syncStateResult = db.prepare(`
    INSERT INTO sync_state (sync_type, status, total_repos, started_at)
    VALUES (?, 'in_progress', ?, ?)
  `).run(syncType, reposToProcess.size, new Date().toISOString());
  const syncStateId = syncStateResult.lastInsertRowid;

  let skippedCount = 0;
  let errorCount = 0;
  let processedCount = 0;

  try {
    // Prepare statements
    const checkStmt = db.prepare(
      'SELECT pushed_at_hash FROM repos WHERE github_id = ?'
    );
    
    // Note: We only insert columns that exist in the schema
    const upsertRepoStmt = db.prepare(`
      INSERT OR REPLACE INTO repos (
        github_id, owner, name, full_name, description, url, 
        primary_language, topics, stars, is_fork, is_private, 
        is_starred, is_owned, readme, file_tree, pushed_at, 
        pushed_at_hash, synced_at
      ) VALUES (
        @github_id, @owner, @name, @full_name, @description, @url,
        @primary_language, @topics, @stars, @is_fork, @is_private,
        @is_starred, @is_owned, @readme, @file_tree, @pushed_at,
        @pushed_at_hash, @synced_at
      )
    `);

    const deleteChunksStmt = db.prepare('DELETE FROM chunks WHERE repo_id = ?');
    
    const insertChunkStmt = db.prepare(`
      INSERT INTO chunks (repo_id, chunk_type, chunk_index, content)
      VALUES (@repo_id, @chunk_type, @chunk_index, @content)
    `);

    // 5. Process repos
    for (const [fullName, { repo, isOwned, isStarred }] of reposToProcess) {
      try {
        // 5a. Compute hash
        const pushedAtHash = hashPushedAt(repo.pushed_at);

        // 5b. Look up existing
        const existing = checkStmt.get(repo.id) as { pushed_at_hash: string } | undefined;
        const isExisting = !!existing;

        // 5c. Check if unchanged
        if (existing && existing.pushed_at_hash === pushedAtHash) {
          skippedCount++;
          yield { type: 'skip', repo: fullName, reason: 'unchanged' };
          continue;
        }

        // 5d. Fetch additional data
        const readme = await fetchReadme(client, repo.owner.login, repo.name);
        
        let fileTree: string | null = null;
        if (options.fullTree || (readme && readme.length >= 100)) {
          fileTree = await fetchFileTree(
            client,
            repo.owner.login,
            repo.name,
            repo.default_branch
          );
        }

        // 5e. Write to DB
        const transaction = db.transaction(() => {
          // Upsert repo
          const syncedAt = new Date().toISOString();
          upsertRepoStmt.run({
            github_id: repo.id,
            owner: repo.owner.login,
            name: repo.name,
            full_name: repo.full_name,
            description: repo.description,
            url: repo.html_url,
            primary_language: repo.language,
            topics: JSON.stringify(repo.topics || []),
            stars: repo.stargazers_count,
            is_fork: repo.fork ? 1 : 0,
            is_private: repo.private ? 1 : 0,
            is_starred: isStarred ? 1 : 0,
            is_owned: isOwned ? 1 : 0,
            readme: readme,
            file_tree: fileTree,
            pushed_at: repo.pushed_at,
            pushed_at_hash: pushedAtHash,
            synced_at: syncedAt,
          });

          const { lastInsertRowid } = db.prepare('SELECT last_insert_rowid() as lastInsertRowid').get() as { lastInsertRowid: number | bigint };
          const repoId = lastInsertRowid;

          // Delete existing chunks
          deleteChunksStmt.run(repoId);

          // Insert metadata chunk
          const metadata = {
            full_name: repo.full_name,
            description: repo.description,
            language: repo.language,
            stars: repo.stargazers_count,
            forks: repo.forks_count,
            topics: repo.topics || [],
            archived: repo.archived,
            fork: repo.fork,
            default_branch: repo.default_branch,
            pushed_at: repo.pushed_at,
            html_url: repo.html_url,
          };

          insertChunkStmt.run({
            repo_id: repoId,
            chunk_type: 'metadata',
            chunk_index: 0,
            content: JSON.stringify(metadata),
          });

          // Insert readme chunks
          if (readme && readme.trim().length > 0) {
            const readmeChunks = splitIntoChunks(readme);
            readmeChunks.forEach((chunk, index) => {
              insertChunkStmt.run({
                repo_id: repoId,
                chunk_type: 'readme',
                chunk_index: index,
                content: chunk,
              });
            });
          }

          // Insert file_tree chunks
          if (fileTree && fileTree.trim().length > 0) {
            const treeChunks = splitIntoChunks(fileTree);
            treeChunks.forEach((chunk, index) => {
              insertChunkStmt.run({
                repo_id: repoId,
                chunk_type: 'file_tree',
                chunk_index: index,
                content: chunk,
              });
            });
          }
        });

        transaction();

        processedCount++;
        // Update sync state
        db.prepare('UPDATE sync_state SET processed_repos = ? WHERE id = ?').run(processedCount, syncStateId);

        yield {
          type: 'repo',
          repo: fullName,
          status: isExisting ? 'updated' : 'new',
        };
      } catch (error: unknown) {
        errorCount++;
        yield { type: 'error', repo: fullName, error: error instanceof Error ? error.message : String(error) };
        // Continue to next repo
      }
    }

    // Mark sync as completed
    db.prepare(`
      UPDATE sync_state 
      SET status = 'completed', completed_at = ? 
      WHERE id = ?
    `).run(new Date().toISOString(), syncStateId);

  } catch (error: unknown) {
    // Mark sync as failed if a fatal error occurs
    if (syncStateId) {
      db.prepare(`
        UPDATE sync_state 
        SET status = 'failed', error = ? 
        WHERE id = ?
      `).run(error instanceof Error ? error.message : String(error), syncStateId);
    }
    throw error;
  }

  yield {
    type: 'done',
    total: processedCount,
    skipped: skippedCount,
    errors: errorCount,
  };
}
