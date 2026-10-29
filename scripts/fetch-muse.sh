#!/usr/bin/env bash
# Download the tested public Muse release into a caller-owned directory.
set -euo pipefail
if [[ $# != 1 ]]; then echo "usage: $0 OUTPUT_DIRECTORY" >&2; exit 2; fi
case "$(uname -m)" in
  aarch64|arm64) artifact=aarch64-linux; sha=3dc39b8fa26f9963cc2038f2966c0932ed5a39f0f9efb39cc0266a261cbbcf76 ;;
  x86_64|amd64) artifact=x86-linux; sha=0809aac7cb58d18f5022b6d05e02aa1974683c0ec430e5f04d25b60ed17de2da ;;
  *) echo 'Unsupported architecture' >&2; exit 2 ;;
esac
mkdir -p "$1"
output="$1/muse"
if [[ -e "$output" ]]; then echo "Refusing to overwrite $output" >&2; exit 2; fi
curl --fail --location --proto '=https' --proto-redir '=https' \
  "https://lookaside.facebook.com/lookaside/muse/download/?channel=muse&version=1.4.14-R7309.1&file=muse-$artifact" -o "$output"
printf '%s  %s\n' "$sha" "$output" | sha256sum --check --status
chmod +x "$output"
printf 'Verified Muse 1.4.14-R7309.1: %s\n' "$output"
