#!/bin/bash
# Container entrypoint. Dispatches a single positional subcommand to the
# corresponding Python under /app/.
#
# A George sign-in lasts a few hours and no device trust skips the push
# after it (DESIGN.md §F), so `download` is a one-shot: a live session
# reused, or a sign-in with the push approved on the phone (nothing to type,
# no VNC), then the REST fetch, in one Camoufox lifetime. `login` folds into
# `download` (except `login --check`, a read-only session probe).
# `vnc-login` is the by-hand fallback. `load` builds the silver. `prune` is a
# pure file walk.

set -euo pipefail

# The verb engine, baked into the base image (shared/images/entrypoint-lib.sh).
# shellcheck source=/dev/null
source /opt/entrypoint-lib.sh

verb="${1:-help}"
[[ $# -gt 0 ]] && shift

case "$verb" in
    explore)   run vnc:explore explore.py -- "$@" ;;
    # One-shot: submit the user name, push approved on the phone, REST
    # fetch. Headed Camoufox under Xvfb, no VNC (nothing to type).
    download)  run xvfb login.py --cli-mfa --bronze-dir /data -- "$@" ;;
    # Fallback: the whole sign-in by hand over VNC.
    vnc-login) run vnc:vnc-login login.py --no-cli-mfa --bronze-dir /data -- "$@" ;;
    login)
        # `login --check` is a read-only session probe under Xvfb.
        login_folds xvfb login.py -- "$@"
        cat >&2 <<'EOF'
erstebank_at: 'login' folds into 'download' — nothing to persist (the push fires every sign-in; §F).
  Run 'download' (one-shot login + fetch), or 'login --check' to probe a session.
EOF
        ;;
    load)      run plain load.py --bronze-dir /data -- "$@" ;;
    prune|sh|bash) entrypoint_common "$verb" "$@" ;;
    help|--help|-h)
        cat <<'EOF'
erstebank_at container

Usage:
  <wrapper> <subcommand> [args...]

Subcommands:
  explore     Discovery harness: Camoufox over VNC + HAR/network log/click
              log/DOM snapshots/response bodies under /debug. See
              DESIGN.md §2-§3.
  download    One-shot sign-in + REST fetch (checking accounts and credit
              cards), the push approved in the George app — no VNC,
              nothing to type. Fetches the rosters, each history, and the
              statement PDFs into a UTC-stamped bronze run dir under /data.
              --lookback bounds the window; --no-documents skips PDFs;
              --dry-run reads the rosters only (it still signs in).
  vnc-login   Fallback for download: expose VNC and do the sign-in by
              hand, then fetch.
  login       Folds into download, which signs in only without a live
              session. 'login --check' probes the session read-only
              (exit 0 alive / 1 dead; no push).
  load        Ingest bronze into the SQLite silver (accounts, transactions,
              daily closing balances, statements). --force rebuilds from
              all bronze.
  prune       Delete non-complete dumps from the bronze tree. --dry-run
              prints the plan. Usually run host-side via the wrapper.
  sh|bash     Open an interactive shell.
  help        Show this message.

Run "<wrapper> <subcommand> --help" for subcommand flags.
EOF
        ;;
    *) exec "$verb" "$@" ;;
esac
