#!/usr/bin/env sh
set -eu

mkdir -p dist

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/log-work.exe .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/log-work-linux-amd64 .

chmod +x dist/log-work-linux-amd64

# Install the Linux build into a WSL/Linux user bin directory so `log-work`
# can be called from anywhere. Override with INSTALL_DIR=/some/path ./build.sh.
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"
cp dist/log-work-linux-amd64 "$install_dir/log-work"
chmod +x "$install_dir/log-work"

printf 'Built:\n  dist/log-work.exe\n  dist/log-work-linux-amd64\n'
printf 'Installed Linux executable:\n  %s/log-work\n' "$install_dir"

case ":$PATH:" in
  *":$install_dir:"*)
    printf 'PATH already includes %s\n' "$install_dir"
    ;;
  *)
    printf '\nAdd this to your shell profile if log-work is not found:\n'
    printf '  export PATH="%s:$PATH"\n' "$install_dir"
    ;;
esac
