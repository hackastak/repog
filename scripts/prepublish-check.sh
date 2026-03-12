#!/usr/bin/env bash
set -e

echo "RepoG Pre-publish Checklist"
echo "==========================="

# 1. Correct Node version
NODE_MAJOR=$(node --version | cut -d. -f1 | tr -d 'v')
if [ "$NODE_MAJOR" -eq 22 ]; then
  echo "✓ Node.js 22"
else
  echo "✗ Wrong Node version: $(node --version). Run: nvm use 22"
  exit 1
fi

# 2. Tests pass
echo "Running test:ci..."
pnpm test:ci
echo "✓ All tests pass"

# 3. Build succeeds
pnpm --filter repog build
echo "✓ Build succeeded"

# 4. Shebang present
head -1 packages/cli/dist/index.js | grep -q "#!/usr/bin/env node" \
  && echo "✓ Shebang present" \
  || (echo "✗ Shebang missing" && exit 1)

# 5. Package name correct
PKG_NAME=$(node -e "console.log(require('./packages/cli/package.json').name)")
if [ "$PKG_NAME" = "repog" ]; then
  echo "✓ Package name: repog"
else
  echo "✗ Wrong package name: $PKG_NAME"
  exit 1
fi

# 6. Version is not 0.0.0
PKG_VERSION=$(node -e "console.log(require('./packages/cli/package.json').version)")
if [ "$PKG_VERSION" = "0.0.0" ]; then
  echo "✗ Version is 0.0.0 — bump the version before publishing"
  exit 1
else
  echo "✓ Version: $PKG_VERSION"
fi

# 7. Dry run pack
echo "Running npm pack dry run..."
cd packages/cli && npm pack --dry-run 2>&1 | tail -20
cd ../..

# 8. npm auth check
npm whoami 2>/dev/null \
  && echo "✓ Logged in to npm as: $(npm whoami)" \
  || echo "⚠  Not logged in to npm. Run: npm login"

echo ""
echo "==========================="
echo "All checks passed. Ready to publish."
echo "To publish: cd packages/cli && npm publish --access public"
