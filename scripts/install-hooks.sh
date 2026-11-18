#!/bin/sh
# Local first: run the same checks as CI before every push.
set -eu
cd "$(dirname "$0")/.."
hook=.git/hooks/pre-push
cat > "$hook" <<'HOOK'
#!/bin/sh
# Installed by `mise run hooks:install`. Skip once with: git push --no-verify (CI still runs).
set -e
mise run check
mise run scan
HOOK
chmod +x "$hook"
echo "installed $hook"
