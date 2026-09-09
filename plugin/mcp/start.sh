#!/usr/bin/env bash
# stdio MCP server for zdev, shipped WITH the plugin.
#
# The fleet supervisor's own daemon exposes the server as a subcommand
# (`zdevd mcp`), so there is nothing to clone or build here. This wrapper
# exists to fail LOUDLY with a fixable message rather than letting Claude
# report an opaque connection error when zdev is not installed yet.
#
# The server talks to the running daemon over its unix socket and every tool
# is a thin client call. No daemon means tools return an in-band error, never
# a crash — a dead daemon must not take the MCP connection down with it.
set -euo pipefail

ZDEVD="${ZDEVD_BIN:-$HOME/.local/bin/zdevd}"

if [[ ! -x "$ZDEVD" ]]; then
  echo "zdev-mcp: $ZDEVD not found or not executable" >&2
  echo "zdev-mcp: install it with: make -C <zdev-checkout>/zdevd install" >&2
  exit 1
fi

exec "$ZDEVD" mcp
