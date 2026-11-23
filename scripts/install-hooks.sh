#!/bin/sh
# Local first: run the same checks as CI before every push.
set -eu
cd "$(dirname "$0")/.."
hook=.git/hooks/pre-push
cat > "$hook" <<'HOOK'
#!/bin/sh
# Installed by `make hooks`. Skip once with: git push --no-verify (CI still runs).
set -e
make check
make scan
HOOK
chmod +x "$hook"
echo "installed $hook"
